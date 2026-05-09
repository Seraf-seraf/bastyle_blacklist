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
from ai_vector_service.storage import SQLiteVectorStore


class FakeModel:
    model_name = "fake-model"

    def embed(self, images):
        vectors = [[1.0, 0.0, 0.0] for _ in images]
        return EmbeddingResult(vectors=vectors, dimension=3, model_name=self.model_name)


class FakeVectorService:
    def __init__(self):
        self.bans = []
        self.searches = []

    def add_ban(self, *, file_unique_id, media_type, frames, dimension):
        self.bans.append(
            {
                "file_unique_id": file_unique_id,
                "media_type": media_type,
                "frames": frames,
                "dimension": dimension,
            }
        )
        return 42

    def search(self, *, frames, dimension, top_k):
        self.searches.append({"frames": frames, "dimension": dimension, "top_k": top_k})
        return [
            FrameSearchResponse(
                frame_index=0,
                hits=[VectorSearchHitResponse(ban_id=42, frame_index=0, score=0.95)],
            )
        ]


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
        data={"file_unique_id": "file-unique-id", "media_type": "photo"},
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

    store = SQLiteVectorStore(tmp_path / "vectors.sqlite")
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


def test_search_returns_hits():
    vector_service = FakeVectorService()
    client = TestClient(
        create_app(Dependencies(PillowImageDecoder(), FakeModel(), vectors=vector_service))
    )

    response = client.post(
        "/search?top_k=3",
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )

    assert response.status_code == 200
    assert response.json() == {
        "model_name": "fake-model",
        "dimension": 3,
        "frames": [
            {
                "frame_index": 0,
                "hits": [{"ban_id": 42, "frame_index": 0, "score": 0.95}],
            }
        ],
    }
    assert vector_service.searches[0]["top_k"] == 3


def test_ban_requires_vector_service():
    client = TestClient(create_app(Dependencies(PillowImageDecoder(), FakeModel())))

    response = client.post(
        "/ban",
        data={"file_unique_id": "file-unique-id", "media_type": "photo"},
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )

    assert response.status_code == 503
    assert response.json()["detail"] == "сервис векторного индекса не настроен"
