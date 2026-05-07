from dataclasses import dataclass
from pathlib import Path

import yaml


DEFAULT_CONFIG_PATH = Path("configs/config.yaml")
DEFAULT_MODEL_NAME = "nomic-ai/nomic-embed-vision-v1.5"
DEFAULT_MODEL_REVISION = "e3a725bce72db07ca4adb1d83da08903f3ee02f8"


@dataclass(frozen=True)
class Settings:
    host: str
    port: int
    model_name: str
    model_revision: str
    device: str
    max_files: int
    max_upload_bytes: int
    max_image_pixels: int
    target_width: int
    target_height: int
    threshold: float


def load_settings(config_path: Path = DEFAULT_CONFIG_PATH) -> Settings:
    with config_path.open("r", encoding="utf-8") as file:
        config = yaml.safe_load(file) or {}

    ai_vector = config.get("matching", {}).get("ai_vector", {})
    service = ai_vector.get("service", {})

    return Settings(
        host=str(service.get("host", "0.0.0.0")),
        port=int(service.get("port", 8080)),
        model_name=str(ai_vector.get("model_name", DEFAULT_MODEL_NAME)),
        model_revision=str(ai_vector.get("model_revision", DEFAULT_MODEL_REVISION)),
        device=str(ai_vector.get("device", "cpu")),
        max_files=int(ai_vector.get("max_files", 10)),
        max_upload_bytes=int(ai_vector.get("max_upload_bytes", 20 * 1024 * 1024)),
        max_image_pixels=int(ai_vector.get("max_image_pixels", 4096 * 4096)),
        target_width=int(ai_vector.get("target_width", 320)),
        target_height=int(ai_vector.get("target_height", 320)),
        threshold=float(ai_vector.get("threshold", 0.92)),
    )
