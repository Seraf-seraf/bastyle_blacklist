from dataclasses import dataclass
from pathlib import Path

import yaml

from ai_vector_service.index import HNSWConfig


DEFAULT_CONFIG_PATH = Path("configs/config.yaml")
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
class Settings:
    host: str
    port: int
    model_name: str
    model_revision: str
    device: str
    db_path: str
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
    video_media = config.get("matching", {}).get("video_media", {})
    service = ai_vector.get("service", {})
    hnsw = ai_vector.get("hnsw", {})

    return Settings(
        host="0.0.0.0",
        port=int(service.get("port", 8080)),
        model_name=str(ai_vector.get("model_name", DEFAULT_MODEL_NAME)),
        model_revision=str(ai_vector.get("model_revision", DEFAULT_MODEL_REVISION)),
        device=str(ai_vector.get("device", "cpu")),
        db_path=str(ai_vector.get("db_path", "bastyle.sqlite")),
        index_path=str(ai_vector.get("index_path", "faiss-image.index")),
        max_files=int(ai_vector.get("max_files", 10)),
        max_upload_bytes=parse_byte_size(video_media.get("max_upload_bytes", "20MiB")),
        max_image_pixels=int(video_media.get("max_image_pixels", 4096 * 4096)),
        target_width=int(video_media.get("target_width", 320)),
        target_height=int(video_media.get("target_height", 320)),
        threshold=float(ai_vector.get("threshold", 0.92)),
        hnsw=HNSWConfig(
            m=int(hnsw.get("m", 32)),
            ef_construction=int(hnsw.get("ef_construction", 80)),
            ef_search=int(hnsw.get("ef_search", 64)),
        ),
    )


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
                raise ValueError("byte size must contain number followed by unit")
            number += char
            continue
        if not char.isspace():
            unit += char

    if not number or not unit:
        raise ValueError("byte size must contain number and unit")

    multiplier = BYTE_UNITS.get(unit.lower())
    if multiplier is None:
        raise ValueError("unsupported byte size unit")

    return int(number) * multiplier
