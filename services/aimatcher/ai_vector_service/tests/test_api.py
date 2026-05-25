from io import BytesIO

from fastapi.testclient import TestClient
from PIL import Image

from ai_vector_service.api import (
    Dependencies,
    FrameSearchResponse,
    FrameEmbedding,
    UploadLimits,
    VectorSearchHitResponse,
    VectorIndexService,
    create_app,
)
from ai_vector_service.images import PillowImageDecoder
from ai_vector_service.model import EmbeddingResult
from ai_vector_service.storage import InsertBanResult, StoredVectorFrame, VectorBan


class FakeModel:
    model_name = "fake-model"

    def embed(self, images):
        vectors = [[1.0, 0.0, 0.0] for _ in images]
        return EmbeddingResult(vectors=vectors, dimension=3, model_name=self.model_name)


class FakeVectorService:
    def __init__(self):
        self.bans = []
        self.searches = []

    def add_ban(self, *, chat_id, file_unique_id, media_type, frames, dimension):
        self.bans.append(
            {
                "chat_id": chat_id,
                "file_unique_id": file_unique_id,
                "media_type": media_type,
                "frames": frames,
                "dimension": dimension,
            }
        )
        return 42

    def search(self, *, chat_id, frames, dimension, top_k):
        self.searches.append({"chat_id": chat_id, "frames": frames, "dimension": dimension, "top_k": top_k})
        return [
            FrameSearchResponse(
                frame_index=0,
                hits=[VectorSearchHitResponse(ban_id=42, chat_id=chat_id, frame_index=0, score=0.95)],
            )
        ]


class FakeDatabase:
    def __init__(self, err=None):
        self.err = err
        self.pings = 0

    def ping(self):
        self.pings += 1
        if self.err is not None:
            raise self.err


class FakeIndexHealth:
    def __init__(self, err=None):
        self.err = err
        self.checks = 0

    def check(self):
        self.checks += 1
        if self.err is not None:
            raise self.err


def png_bytes() -> bytes:
    buffer = BytesIO()
    Image.new("RGB", (8, 8), "white").save(buffer, format="PNG")
    return buffer.getvalue()


def test_embed_returns_vectors_for_ordered_frames():
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel())))

    response = client.post(
        "/embed",
        files=[
            ("files", ("frame-0.png", png_bytes(), "image/png")),
            ("files", ("frame-1.png", png_bytes(), "image/png")),
        ],
    )

    assert response.status_code == 200
    assert response.json() == {
        "model_name": "fake-model",
        "dimension": 3,
        "frames": [
            {"frame_index": 0, "vector": [1.0, 0.0, 0.0]},
            {"frame_index": 1, "vector": [1.0, 0.0, 0.0]},
        ],
    }


def test_health_returns_ok_when_database_is_available():
    database = FakeDatabase()
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel(), database=database)))

    response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"status": "ok", "model_name": "fake-model"}
    assert database.pings == 1


def test_health_returns_unavailable_when_database_is_missing():
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel())))

    response = client.get("/health")

    assert response.status_code == 503
    assert response.json()["detail"] == "PostgreSQL pool не настроен"


def test_health_returns_unavailable_when_database_ping_fails():
    database = FakeDatabase(RuntimeError("нет соединения"))
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel(), database=database)))

    response = client.get("/health")

    assert response.status_code == 503
    assert response.json()["detail"] == "PostgreSQL недоступен"


def test_health_returns_unavailable_when_index_checkpoint_is_stale():
    database = FakeDatabase()
    index_health = FakeIndexHealth(RuntimeError("stale"))
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel(), database=database, index_health=index_health)))

    response = client.get("/health")

    assert response.status_code == 503
    assert response.json()["detail"] == "Индексы требуют пересинхронизации"
    assert index_health.checks == 1


def test_metrics_returns_prometheus_text():
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel())))

    response = client.get("/metrics")

    assert response.status_code == 200
    assert "text/plain" in response.headers["content-type"]
    assert "bastyle_ai_db_errors_total" in response.text
    assert "bastyle_ai_db_query_duration_seconds" in response.text
    assert "bastyle_ai_index_stale" in response.text
    assert "bastyle_ai_http_requests_total" in response.text


def test_metrics_records_http_requests_by_route_status_and_latency():
    database = FakeDatabase()
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel(), database=database)))

    health_response = client.get("/health")
    embed_response = client.post(
        "/embed",
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )
    missing_response = client.get("/missing")
    metrics_response = client.get("/metrics")

    assert health_response.status_code == 200
    assert embed_response.status_code == 200
    assert missing_response.status_code == 404
    assert metrics_response.status_code == 200
    assert 'bastyle_ai_http_requests_total{method="GET",route="/health",status="200"}' in metrics_response.text
    assert 'bastyle_ai_http_requests_total{method="POST",route="/embed",status="200"}' in metrics_response.text
    assert 'bastyle_ai_http_requests_total{method="GET",route="unmatched",status="404"}' in metrics_response.text
    assert 'bastyle_ai_http_requests_total{method="GET",route="/metrics",status="200"}' not in metrics_response.text
    assert 'bastyle_ai_http_request_duration_seconds_bucket{le="0.005",method="GET",route="/health",status="200"}' in metrics_response.text


def test_embed_rejects_invalid_image():
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel())))

    response = client.post(
        "/embed",
        files=[("files", ("broken.png", b"not an image", "image/png"))],
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "данные не являются поддерживаемым изображением"


def test_embed_rejects_too_many_files():
    client = TestClient(
        create_app(
            Dependencies(PillowImageDecoder(), FakeModel()),
            UploadLimits(max_files=1, max_upload_bytes=1024 * 1024),
        )
    )

    response = client.post(
        "/embed",
        files=[
            ("files", ("frame-0.png", png_bytes(), "image/png")),
            ("files", ("frame-1.png", png_bytes(), "image/png")),
        ],
    )

    assert response.status_code == 413
    assert response.json()["detail"] == "слишком много файлов"


def test_embed_rejects_upload_over_size_limit():
    client = TestClient(
        create_app(
            Dependencies(PillowImageDecoder(), FakeModel()),
            UploadLimits(max_files=10, max_upload_bytes=8),
        )
    )

    response = client.post(
        "/embed",
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )

    assert response.status_code == 413
    assert response.json()["detail"] == "размер загрузки превышает лимит"


def test_embed_rejects_image_over_pixel_limit():
    client = TestClient(
        create_app(Dependencies(PillowImageDecoder(max_image_pixels=16), FakeModel()))
    )

    response = client.post(
        "/embed",
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "количество пикселей изображения превышает лимит"


def test_ban_saves_vectors():
    vector_service = FakeVectorService()
    client = TestClient(
        create_app(Dependencies(PillowImageDecoder(), FakeModel(), vectors=vector_service))
    )

    response = client.post(
        "/ban",
        data={"chat_id": "10", "file_unique_id": "file-unique-id", "media_type": "photo"},
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )

    assert response.status_code == 200
    assert response.json() == {"ban_id": 42}
    assert vector_service.bans[0]["file_unique_id"] == "file-unique-id"
    assert vector_service.bans[0]["media_type"] == "photo"
    assert vector_service.bans[0]["dimension"] == 3
    assert vector_service.bans[0]["frames"][0].vector == [1.0, 0.0, 0.0]


def test_vector_index_service_deactivates_ban_when_index_update_fails(tmp_path):
    class FailingIndex:
        def validate_vectors(self, vectors):
            return None

        def add_ban(self, ban):
            raise RuntimeError("обновление индекса завершилось ошибкой")

    store = MemoryVectorStore()
    service = VectorIndexService(
        store=store,
        model_name="fake-model",
        model_revision="fake-revision",
        index_path=str(tmp_path / "faiss.index"),
    )
    service._index = FailingIndex()
    service._dimension = 3

    try:
        try:
            service.add_ban(
                chat_id=10,
                file_unique_id="file-unique-id",
                media_type="photo",
                frames=[FrameEmbedding(frame_index=0, vector=[1.0, 0.0, 0.0])],
                dimension=3,
            )
        except RuntimeError:
            pass
        else:
            raise AssertionError("ожидался RuntimeError")

        active_bans = store.load_active_bans(
            model_name="fake-model",
            model_revision="fake-revision",
            vector_dim=3,
        )
        assert active_bans == []
        assert service._index is None
        assert service._dimension is None
    finally:
        store.close()


def test_vector_index_service_keeps_existing_ban_active_when_duplicate_index_update_fails(tmp_path):
    class FailingIndex:
        def validate_vectors(self, vectors):
            return None

        def add_ban(self, ban):
            raise RuntimeError("повторное обновление индекса завершилось ошибкой")

        def save(self, **kwargs):
            return None

    store = MemoryVectorStore()
    first_ban_id = store.insert_ban(
        chat_id=10,
        file_unique_id="file-unique-id",
        media_type="photo",
        model_name="fake-model",
        model_revision="fake-revision",
        vector_dim=3,
        frames=[MemoryVectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
    )
    store.return_existing = True
    service = VectorIndexService(
        store=store,
        model_name="fake-model",
        model_revision="fake-revision",
        index_path=str(tmp_path / "faiss.index"),
    )
    service._index = FailingIndex()
    service._dimension = 3

    try:
        ban_id = service.add_ban(
            chat_id=10,
            file_unique_id="file-unique-id",
            media_type="photo",
            frames=[FrameEmbedding(frame_index=0, vector=[1.0, 0.0, 0.0])],
            dimension=3,
        )

        assert ban_id == first_ban_id
        active_bans = store.load_active_bans(
            model_name="fake-model",
            model_revision="fake-revision",
            vector_dim=3,
        )
        assert [ban.id for ban in active_bans] == [first_ban_id]
    finally:
        store.close()


def test_vector_index_service_applies_existing_ban_by_uid(tmp_path):
    class FakeIndex:
        def __init__(self):
            self.ban_ids = []

        def add_ban(self, ban):
            self.ban_ids.append(ban.id)

        def save(self, **kwargs):
            return None

    store = MemoryVectorStore()
    ban_uid = "ban-uid-1"
    ban_id = store.insert_ban(
        chat_id=10,
        file_unique_id="file-unique-id",
        media_type="photo",
        model_name="fake-model",
        model_revision="fake-revision",
        vector_dim=3,
        frames=[MemoryVectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
        ban_uid=ban_uid,
    )
    service = VectorIndexService(
        store=store,
        model_name="fake-model",
        model_revision="fake-revision",
        index_path=str(tmp_path / "faiss.index"),
    )
    service._index = FakeIndex()
    service._dimension = 3

    status = service.apply_existing_ban(ban_uid=ban_uid)

    assert status == "applied"
    assert service._index.ban_ids == [ban_id]


def test_vector_index_service_skips_repeated_existing_ban_event(tmp_path):
    store = MemoryVectorStore()
    ban_uid = "ban-uid-1"
    store.insert_ban(
        chat_id=10,
        file_unique_id="file-unique-id",
        media_type="photo",
        model_name="fake-model",
        model_revision="fake-revision",
        vector_dim=3,
        frames=[MemoryVectorFrame(frame_index=0, position_millis=0, vector=[1.0, 0.0, 0.0])],
        ban_uid=ban_uid,
    )
    service = VectorIndexService(
        store=store,
        model_name="fake-model",
        model_revision="fake-revision",
        index_path=str(tmp_path / "faiss.index"),
    )

    first_status = service.apply_existing_ban(ban_uid=ban_uid)
    second_status = service.apply_existing_ban(ban_uid=ban_uid)

    assert first_status == "already_applied"
    assert second_status == "already_applied"
    assert service._index.vectors_count == 1


def test_search_returns_hits():
    vector_service = FakeVectorService()
    client = TestClient(
        create_app(Dependencies(PillowImageDecoder(), FakeModel(), vectors=vector_service))
    )

    response = client.post(
        "/search?top_k=3&chat_id=10",
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )

    assert response.status_code == 200
    assert response.json() == {
        "model_name": "fake-model",
        "dimension": 3,
        "frames": [
            {
                "frame_index": 0,
                "hits": [{"ban_id": 42, "chat_id": 10, "frame_index": 0, "score": 0.95}],
            }
        ],
    }
    assert vector_service.searches[0]["top_k"] == 3


def test_ban_requires_vector_service():
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel())))

    response = client.post(
        "/ban",
        data={"chat_id": "10", "file_unique_id": "file-unique-id", "media_type": "photo"},
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )

    assert response.status_code == 503
    assert response.json()["detail"] == "сервис векторного индекса не настроен"


class MemoryVectorStore:
    def __init__(self):
        self.bans = []
        self.next_ban_id = 0
        self.next_frame_id = 0
        self.return_existing = False
        self.ban_uids = {}

    def close(self):
        return None

    def insert_ban(self, *, chat_id, file_unique_id, media_type, model_name, model_revision, vector_dim, frames, ban_uid=None):
        return self.insert_ban_result(
            chat_id=chat_id,
            file_unique_id=file_unique_id,
            media_type=media_type,
            model_name=model_name,
            model_revision=model_revision,
            vector_dim=vector_dim,
            frames=frames,
            ban_uid=ban_uid,
        ).ban_id

    def insert_ban_result(self, *, chat_id, file_unique_id, media_type, model_name, model_revision, vector_dim, frames, ban_uid=None):
        if self.return_existing:
            for ban in self.bans:
                if (
                    ban.active
                    and ban.chat_id == chat_id
                    and ban.file_unique_id == file_unique_id
                    and ban.model_name == model_name
                    and ban.model_revision == model_revision
                    and ban.vector_dim == vector_dim
                ):
                    return InsertBanResult(ban_id=ban.id, created=False)

        self.next_ban_id += 1
        stored_frames = []
        for frame in frames:
            self.next_frame_id += 1
            stored_frames.append(
                StoredVectorFrame(
                    id=self.next_frame_id,
                    ban_id=self.next_ban_id,
                    frame_index=frame.frame_index,
                    position_millis=frame.position_millis,
                    vector=frame.vector,
                )
            )
        self.bans.append(
            VectorBan(
                id=self.next_ban_id,
                chat_id=chat_id,
                file_unique_id=file_unique_id,
                media_type=media_type,
                model_name=model_name,
                model_revision=model_revision,
                vector_dim=vector_dim,
                frames_count=len(frames),
                active=True,
                created_at="2026-05-19T00:00:00Z",
                frames=stored_frames,
            )
        )
        self.ban_uids[self.next_ban_id] = ban_uid
        return InsertBanResult(ban_id=self.next_ban_id, created=True)

    def load_active_bans(self, *, chat_id=None, model_name=None, model_revision=None, vector_dim=None):
        bans = [ban for ban in self.bans if ban.active]
        if chat_id is not None:
            bans = [ban for ban in bans if ban.chat_id == chat_id]
        if model_name is not None:
            bans = [ban for ban in bans if ban.model_name == model_name]
        if model_revision is not None:
            bans = [ban for ban in bans if ban.model_revision == model_revision]
        if vector_dim is not None:
            bans = [ban for ban in bans if ban.vector_dim == vector_dim]
        return bans

    def load_active_ban_by_uid(self, *, ban_uid, model_name, model_revision):
        for ban in self.load_active_bans(model_name=model_name, model_revision=model_revision):
            if self.ban_uids.get(ban.id) == ban_uid:
                return ban
        return None

    def deactivate_ban(self, ban_id):
        from dataclasses import replace

        for index, ban in enumerate(self.bans):
            if ban.id == ban_id and ban.active:
                self.bans[index] = replace(ban, active=False)
                return True
        return False

    def save_index_state(self, state):
        return None

    def load_index_state(self, **kwargs):
        return None


class MemoryVectorFrame:
    def __init__(self, frame_index, position_millis, vector):
        self.frame_index = frame_index
        self.position_millis = position_millis
        self.vector = vector
