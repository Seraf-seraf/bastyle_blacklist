from dataclasses import dataclass
import threading
from typing import Annotated, Any

from fastapi import FastAPI, File, Form, HTTPException, Query, UploadFile
from pydantic import BaseModel, Field

from ai_vector_service.images import ImageDecodeError, ImageDecoder
from ai_vector_service.index import FaissHNSWVectorIndex, HNSWConfig
from ai_vector_service.model import ImageEmbeddingModel
from ai_vector_service.storage import InsertBanResult, PostgresVectorStore, VectorFrame


class FrameEmbedding(BaseModel):
    frame_index: int = Field(ge=0)
    vector: list[float]


class EmbedResponse(BaseModel):
    model_name: str
    dimension: int = Field(gt=0)
    frames: list[FrameEmbedding]


class HealthResponse(BaseModel):
    status: str
    model_name: str


class BanResponse(BaseModel):
    ban_id: int = Field(gt=0)


class VectorSearchHitResponse(BaseModel):
    ban_id: int = Field(gt=0)
    chat_id: int
    frame_index: int = Field(ge=0)
    score: float


class FrameSearchResponse(BaseModel):
    frame_index: int = Field(ge=0)
    hits: list[VectorSearchHitResponse]


class SearchResponse(BaseModel):
    model_name: str
    dimension: int = Field(gt=0)
    frames: list[FrameSearchResponse]


@dataclass(frozen=True)
class Dependencies:
    decoder: ImageDecoder
    model: ImageEmbeddingModel
    vectors: "VectorIndexService | None" = None
    database: object | None = None
    index_health: object | None = None


@dataclass(frozen=True)
class UploadLimits:
    max_files: int = 10
    max_upload_bytes: int = 20 * 1024 * 1024


class VectorIndexService:
    def __init__(
        self,
        *,
        store: PostgresVectorStore,
        model_name: str,
        model_revision: str,
        index_path: str,
        hnsw_config: HNSWConfig | None = None,
    ) -> None:
        self._store = store
        self._model_name = model_name
        self._model_revision = model_revision
        self._index_path = index_path
        self._hnsw_config = hnsw_config
        self._index: FaissHNSWVectorIndex | None = None
        self._dimension: int | None = None
        self._lock = threading.RLock()

    def add_ban(
        self,
        *,
        chat_id: int,
        file_unique_id: str,
        media_type: str,
        frames: list[FrameEmbedding],
        dimension: int,
    ) -> int:
        if not frames:
            raise ValueError("требуется хотя бы один вектор кадра")

        with self._lock:
            index = self._index_for_dimension(dimension)
            index.validate_vectors([frame.vector for frame in frames])

            insert_result: InsertBanResult | None = None
            try:
                insert_result = self._insert_ban(
                    file_unique_id=file_unique_id,
                    chat_id=chat_id,
                    media_type=media_type,
                    model_name=self._model_name,
                    model_revision=self._model_revision,
                    vector_dim=dimension,
                    frames=[
                        VectorFrame(
                            frame_index=frame.frame_index,
                            position_millis=frame.frame_index * 1000,
                            vector=frame.vector,
                        )
                        for frame in frames
                    ],
                )
                ban_id = insert_result.ban_id
                if not insert_result.created:
                    return ban_id

                ban = next(
                    ban
                    for ban in self._store.load_active_bans(
                        chat_id=chat_id,
                        model_name=self._model_name,
                        model_revision=self._model_revision,
                        vector_dim=dimension,
                    )
                    if ban.id == ban_id
                )
                index.add_ban(ban)
                index.save(
                    store=self._store,
                    path=self._index_path,
                    model_name=self._model_name,
                    model_revision=self._model_revision,
                )
            except Exception:
                if insert_result is not None and insert_result.created:
                    self._store.deactivate_ban(insert_result.ban_id)
                    self._index = None
                    self._dimension = None
                raise

            return ban_id

    def search(self, *, chat_id: int, frames: list[FrameEmbedding], dimension: int, top_k: int) -> list[FrameSearchResponse]:
        if top_k <= 0:
            raise ValueError("top_k должен быть положительным")

        with self._lock:
            index = self._index_for_dimension(dimension)
            return [
                FrameSearchResponse(
                    frame_index=frame.frame_index,
                    hits=[
                        VectorSearchHitResponse(
                            ban_id=hit.ban_id,
                            chat_id=hit.chat_id,
                            frame_index=hit.frame_index,
                            score=hit.score,
                        )
                        for hit in index.search(frame.vector, top_k, chat_id=chat_id)
                    ],
                )
                for frame in frames
            ]

    def apply_existing_ban(self, *, ban_uid: str) -> str:
        if not ban_uid:
            raise ValueError("ban_uid обязателен")

        with self._lock:
            load_by_uid = getattr(self._store, "load_active_ban_by_uid", None)
            if load_by_uid is None:
                raise RuntimeError("PostgreSQL-хранилище не умеет загружать AI-vector ban по UID")

            ban = load_by_uid(
                ban_uid=ban_uid,
                model_name=self._model_name,
                model_revision=self._model_revision,
            )
            if ban is None:
                return "skipped"

            index = self._index_for_dimension(ban.vector_dim)
            index.add_ban(ban)
            index.save(
                store=self._store,
                path=self._index_path,
                model_name=self._model_name,
                model_revision=self._model_revision,
            )
            return "applied"

    def _index_for_dimension(self, dimension: int) -> FaissHNSWVectorIndex:
        if self._index is not None:
            if self._dimension != dimension:
                raise ValueError("размерность вектора не совпадает")

            return self._index

        self._index = FaissHNSWVectorIndex.load_or_rebuild(
            store=self._store,
            model_name=self._model_name,
            model_revision=self._model_revision,
            vector_dim=dimension,
            index_path=self._index_path,
            config=self._hnsw_config,
        )
        self._dimension = dimension
        return self._index

    def _insert_ban(self, **kwargs) -> InsertBanResult:
        insert_with_result = getattr(self._store, "insert_ban_result", None)
        if insert_with_result is not None:
            return insert_with_result(**kwargs)

        return InsertBanResult(ban_id=self._store.insert_ban(**kwargs), created=True)


def create_app(
    dependencies: Dependencies,
    limits: UploadLimits | None = None,
    lifespan: Any | None = None,
) -> FastAPI:
    limits = limits or UploadLimits()
    app = FastAPI(title="Bastyle AI Vector Service", lifespan=lifespan)

    @app.get("/health", response_model=HealthResponse)
    def health() -> HealthResponse:
        if dependencies.database is None:
            raise HTTPException(status_code=503, detail="PostgreSQL pool не настроен")

        try:
            dependencies.database.ping()
        except Exception as err:
            raise HTTPException(status_code=503, detail="PostgreSQL недоступен") from err
        if dependencies.index_health is not None:
            try:
                dependencies.index_health.check()
            except Exception as err:
                raise HTTPException(status_code=503, detail="Индексы требуют пересинхронизации") from err

        return HealthResponse(status="ok", model_name=dependencies.model.model_name)

    @app.post("/embed", response_model=EmbedResponse)
    async def embed(
        files: Annotated[list[UploadFile], File(description="Одно изображение или упорядоченные кадры")],
    ) -> EmbedResponse:
        result = await _embed_files(dependencies, limits, files)

        return EmbedResponse(
            model_name=result.model_name,
            dimension=result.dimension,
            frames=[
                FrameEmbedding(frame_index=index, vector=vector)
                for index, vector in enumerate(result.vectors)
            ],
        )

    @app.post("/ban", response_model=BanResponse)
    async def ban(
        chat_id: Annotated[int, Form()],
        file_unique_id: Annotated[str, Form(min_length=1)],
        media_type: Annotated[str, Form(min_length=1)],
        files: Annotated[list[UploadFile], File(description="Одно изображение или упорядоченные кадры")],
    ) -> BanResponse:
        vector_service = _require_vector_service(dependencies)
        result = await _embed_files(dependencies, limits, files)

        try:
            ban_id = vector_service.add_ban(
                file_unique_id=file_unique_id,
                chat_id=chat_id,
                media_type=media_type,
                frames=[
                    FrameEmbedding(frame_index=index, vector=vector)
                    for index, vector in enumerate(result.vectors)
                ],
                dimension=result.dimension,
            )
        except ValueError as err:
            raise HTTPException(status_code=400, detail=str(err)) from err
        except RuntimeError as err:
            raise HTTPException(status_code=500, detail=str(err)) from err

        return BanResponse(ban_id=ban_id)

    @app.post("/search", response_model=SearchResponse)
    async def search(
        files: Annotated[list[UploadFile], File(description="Одно изображение или упорядоченные кадры")],
        chat_id: Annotated[int, Query()],
        top_k: Annotated[int, Query(gt=0)] = 5,
    ) -> SearchResponse:
        vector_service = _require_vector_service(dependencies)
        result = await _embed_files(dependencies, limits, files)

        try:
            frames = vector_service.search(
                chat_id=chat_id,
                frames=[
                    FrameEmbedding(frame_index=index, vector=vector)
                    for index, vector in enumerate(result.vectors)
                ],
                dimension=result.dimension,
                top_k=top_k,
            )
        except ValueError as err:
            raise HTTPException(status_code=400, detail=str(err)) from err
        except RuntimeError as err:
            raise HTTPException(status_code=500, detail=str(err)) from err

        return SearchResponse(
            model_name=result.model_name,
            dimension=result.dimension,
            frames=frames,
        )

    return app


def _require_vector_service(dependencies: Dependencies) -> VectorIndexService:
    if dependencies.vectors is None:
        raise HTTPException(status_code=503, detail="сервис векторного индекса не настроен")

    return dependencies.vectors


async def _embed_files(dependencies: Dependencies, limits: UploadLimits, files: list[UploadFile]):
    if len(files) > limits.max_files:
        raise HTTPException(status_code=413, detail="слишком много файлов")

    images = []
    upload_bytes = 0
    for file in files:
        remaining_bytes = limits.max_upload_bytes - upload_bytes
        if remaining_bytes <= 0:
            raise HTTPException(status_code=413, detail="размер загрузки превышает лимит")

        data = await file.read(remaining_bytes + 1)
        upload_bytes += len(data)
        if upload_bytes > limits.max_upload_bytes:
            raise HTTPException(status_code=413, detail="размер загрузки превышает лимит")

        try:
            images.append(dependencies.decoder.decode(data))
        except ImageDecodeError as err:
            raise HTTPException(status_code=400, detail=str(err)) from err

    try:
        return dependencies.model.embed(images)
    except ValueError as err:
        raise HTTPException(status_code=400, detail=str(err)) from err
    except RuntimeError as err:
        raise HTTPException(status_code=500, detail=str(err)) from err
