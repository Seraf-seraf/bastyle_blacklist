from contextlib import asynccontextmanager
import socket

from fastapi import FastAPI
import uvicorn

from ai_vector_service.api import Dependencies, UploadLimits, VectorIndexService, create_app
from ai_vector_service.config import load_settings
from ai_vector_service.db import close_pool, create_pool
from ai_vector_service.images import PillowImageDecoder
from ai_vector_service.index_events import (
    AIVectorEventApplier,
    INDEX_NAME,
    IndexHealthCheck,
    IndexSynchronizer,
    PostgresCheckpointStore,
    RabbitMQIndexConsumer,
    WatermillOutboxEventReader,
)
from ai_vector_service.model import TransformersImageEmbeddingModel
from ai_vector_service.storage import PostgresVectorStore


settings = load_settings()

database_pool = create_pool(settings.database)
vector_store = PostgresVectorStore(database_pool)
checkpoint_store = None
consumer_id = ""
if settings.index_events.enabled:
    checkpoint_store = PostgresCheckpointStore(database_pool)
    consumer_id = settings.index_events.replica_id or socket.gethostname()

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
    index_health=IndexHealthCheck(checkpoint_store, consumer_id, [INDEX_NAME]) if checkpoint_store is not None else None,
)

index_consumer = None
if settings.index_events.enabled:
    synchronizer = IndexSynchronizer(
        consumer_id=consumer_id,
        reader=WatermillOutboxEventReader(database_pool),
        checkpoints=checkpoint_store,
        applier=AIVectorEventApplier(dependencies.vectors),
        batch_size=settings.index_events.catch_up_batch_size,
    )
    index_consumer = RabbitMQIndexConsumer(
        rabbitmq_url=settings.rabbitmq.url,
        exchange=settings.rabbitmq.exchange,
        exchange_type=settings.rabbitmq.exchange_type,
        queue_template=settings.index_events.queue_template,
        routing_keys=settings.index_events.routing_keys,
        prefetch=settings.index_events.prefetch,
        synchronizer=synchronizer,
        consumer_id=consumer_id,
        reconnect_interval=settings.rabbitmq.reconnect_interval,
        catch_up_interval=settings.index_events.catch_up_interval,
    )


@asynccontextmanager
async def lifespan(_: FastAPI):
    consumer_thread = None
    if index_consumer is not None:
        consumer_thread = index_consumer.start_background()
    try:
        yield
    finally:
        if index_consumer is not None:
            index_consumer.stop()
        if consumer_thread is not None:
            consumer_thread.join(timeout=5)
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
