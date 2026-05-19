from contextlib import asynccontextmanager

from fastapi import FastAPI
import uvicorn

from ai_vector_service.api import Dependencies, UploadLimits, VectorIndexService, create_app
from ai_vector_service.config import load_settings
from ai_vector_service.db import close_pool, create_pool
from ai_vector_service.images import PillowImageDecoder
from ai_vector_service.model import TransformersImageEmbeddingModel
from ai_vector_service.storage import PostgresVectorStore


settings = load_settings()

database_pool = create_pool(settings.database)
vector_store = PostgresVectorStore(database_pool)
dependencies = Dependencies(
    decoder=PillowImageDecoder(
        max_image_pixels=settings.max_image_pixels,
        target_size=(settings.target_width, settings.target_height),
    ),
    model=TransformersImageEmbeddingModel(
        settings.model_name,
        settings.model_revision,
        settings.device,
    ),
    vectors=VectorIndexService(
        store=vector_store,
        model_name=settings.model_name,
        model_revision=settings.model_revision,
        index_path=settings.index_path,
        hnsw_config=settings.hnsw,
    ),
    database=database_pool,
)


@asynccontextmanager
async def lifespan(_: FastAPI):
    try:
        yield
    finally:
        close_pool(database_pool)


app = create_app(
    dependencies,
    UploadLimits(
        max_files=settings.max_files,
        max_upload_bytes=settings.max_upload_bytes,
    ),
    lifespan=lifespan,
)


def main() -> None:
    uvicorn.run(app, host=settings.host, port=settings.port, timeout_graceful_shutdown=30)


if __name__ == "__main__":
    main()
