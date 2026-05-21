from dataclasses import dataclass
from pathlib import Path

import yaml

from ai_vector_service.index import HNSWConfig


DEFAULT_CONFIG_PATH = Path("infra/config/config.yaml")
DEFAULT_MODEL_NAME = "nomic-ai/nomic-embed-vision-v1.5"
DEFAULT_MODEL_REVISION = "e3a725bce72db07ca4adb1d83da08903f3ee02f8"
BYTE_UNITS = {
    "b": 1,
    "kb": 1000,
    "mb": 1000**2,
    "gb": 1000**3,
    "kib": 1024,
    "mib": 1024**2,
    "gib": 1024**3,
}


@dataclass(frozen=True)
class DatabaseSettings:
    dsn: str
    min_size: int
    max_size: int
    connect_timeout: float


@dataclass(frozen=True)
class RabbitMQSettings:
    url: str
    exchange: str
    exchange_type: str
    reconnect_interval: float


@dataclass(frozen=True)
class IndexEventsConsumerSettings:
    enabled: bool
    replica_id: str
    queue_template: str
    routing_keys: list[str]
    prefetch: int
    catch_up_interval: float
    catch_up_batch_size: int


@dataclass(frozen=True)
class Settings:
    host: str
    port: int
    model_name: str
    model_revision: str
    device: str
    database: DatabaseSettings
    rabbitmq: RabbitMQSettings
    index_events: IndexEventsConsumerSettings
    index_path: str
    max_files: int
    max_upload_bytes: int
    max_image_pixels: int
    target_width: int
    target_height: int
    threshold: float
    hnsw: HNSWConfig


def load_settings(config_path: Path = DEFAULT_CONFIG_PATH) -> Settings:
    with config_path.open("r", encoding="utf-8") as file:
        config = yaml.safe_load(file) or {}

    ai_vector = config.get("matching", {}).get("ai_vector", {})
    database = config.get("database", {})
    rabbitmq = config.get("rabbitmq", {})
    index_events = config.get("consumers", {}).get("index_events", {})
    media_config = config.get("media_config", {})
    service = ai_vector.get("service", {})
    hnsw = ai_vector.get("hnsw", {})

    return Settings(
        host="0.0.0.0",
        port=int(service.get("port", 8080)),
        model_name=str(ai_vector.get("model_name", DEFAULT_MODEL_NAME)),
        model_revision=str(ai_vector.get("model_revision", DEFAULT_MODEL_REVISION)),
        device=str(ai_vector.get("device", "cpu")),
        database=DatabaseSettings(
            dsn=str(database.get("dsn", "postgres://bastyle:bastyle@bastyle-postgresql:5432/bastyle?sslmode=disable")),
            min_size=int(database.get("min_conns", 1)),
            max_size=int(database.get("max_conns", 10)),
            connect_timeout=parse_duration_seconds(database.get("connect_timeout", "5s")),
        ),
        rabbitmq=RabbitMQSettings(
            url=str(rabbitmq.get("url", "amqp://guest:guest@bastyle-rabbitmq:5672/")),
            exchange=str(rabbitmq.get("exchange", "bastyle.events")),
            exchange_type=str(rabbitmq.get("exchange_type", "topic")),
            reconnect_interval=parse_duration_seconds(rabbitmq.get("reconnect_interval", "5s")),
        ),
        index_events=IndexEventsConsumerSettings(
            enabled=bool(index_events.get("enabled", False)),
            replica_id=str(index_events.get("replica_id", "")),
            queue_template=str(index_events.get("queue_template", "bastyle.replica.%s.events")),
            routing_keys=[str(key) for key in index_events.get("routing_keys", ["media.ban.#", "index.#"])],
            prefetch=int(index_events.get("prefetch", 10)),
            catch_up_interval=parse_duration_seconds(index_events.get("catch_up_interval", "5s")),
            catch_up_batch_size=int(index_events.get("catch_up_batch_size", 100)),
        ),
        index_path=str(ai_vector.get("index_path", "faiss-image.index")),
        max_files=int(ai_vector.get("max_files", 10)),
        max_upload_bytes=parse_byte_size(media_config.get("max_upload_bytes", "20MiB")),
        max_image_pixels=int(media_config.get("max_image_pixels", 4096 * 4096)),
        target_width=int(media_config.get("target_width", 320)),
        target_height=int(media_config.get("target_height", 320)),
        threshold=float(ai_vector.get("threshold", 0.92)),
        hnsw=HNSWConfig(
            m=int(hnsw.get("m", 32)),
            ef_construction=int(hnsw.get("ef_construction", 80)),
            ef_search=int(hnsw.get("ef_search", 64)),
        ),
    )


def parse_duration_seconds(value: object) -> float:
    if isinstance(value, (int, float)):
        return float(value)

    raw = str(value).strip()
    if raw.endswith("ms"):
        return float(raw[:-2]) / 1000
    if raw.endswith("s"):
        return float(raw[:-1])
    if raw.endswith("m"):
        return float(raw[:-1]) * 60
    if raw.endswith("h"):
        return float(raw[:-1]) * 60 * 60

    return float(raw)


def parse_byte_size(value: object) -> int:
    if isinstance(value, int):
        return value

    raw = str(value).strip()
    if raw.isdigit():
        return int(raw)

    number = ""
    unit = ""
    for char in raw:
        if char.isdigit():
            if unit:
                raise ValueError("размер в байтах должен содержать число и единицу измерения")
            number += char
            continue
        if not char.isspace():
            unit += char

    if not number or not unit:
        raise ValueError("размер в байтах должен содержать число и единицу измерения")

    multiplier = BYTE_UNITS.get(unit.lower())
    if multiplier is None:
        raise ValueError("неподдерживаемая единица размера")

    return int(number) * multiplier
