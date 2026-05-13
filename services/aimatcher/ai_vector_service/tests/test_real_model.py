from io import BytesIO
from pathlib import Path

from PIL import Image, ImageDraw

from ai_vector_service.config import load_settings
from ai_vector_service.model import TransformersImageEmbeddingModel


EXAMPLE_CONFIG_PATH = Path("infra/config/config.example.yaml")


def _base_image() -> Image.Image:
    image = Image.new("RGB", (160, 160), "white")
    draw = ImageDraw.Draw(image)
    draw.rectangle((24, 24, 136, 136), outline="black", width=5)
    draw.ellipse((54, 54, 106, 106), fill="royalblue")
    draw.line((32, 128, 128, 32), fill="crimson", width=4)
    return image


def _changed_image() -> Image.Image:
    image = _base_image()
    draw = ImageDraw.Draw(image)
    draw.rectangle((118, 118, 148, 148), fill="gold")
    return image


def _different_image() -> Image.Image:
    image = Image.new("RGB", (160, 160), "darkgreen")
    draw = ImageDraw.Draw(image)
    for offset in range(0, 160, 16):
        draw.line((0, offset, offset, 0), fill="white", width=2)
    draw.polygon((80, 18, 142, 142, 18, 142), fill="orange")
    return image


def _roundtrip(image: Image.Image) -> Image.Image:
    buffer = BytesIO()
    image.save(buffer, format="PNG")
    return Image.open(BytesIO(buffer.getvalue())).convert("RGB")


def _cosine(left: list[float], right: list[float]) -> float:
    return sum(a * b for a, b in zip(left, right))


def test_real_model_scores_against_threshold():
    settings = load_settings(EXAMPLE_CONFIG_PATH)
    model = TransformersImageEmbeddingModel(
        settings.model_name,
        settings.model_revision,
        settings.device,
    )

    result = model.embed(
        [
            _roundtrip(_base_image()),
            _roundtrip(_base_image()),
            _roundtrip(_changed_image()),
            _roundtrip(_different_image()),
        ]
    )

    same_score = _cosine(result.vectors[0], result.vectors[1])
    changed_score = _cosine(result.vectors[0], result.vectors[2])
    different_score = _cosine(result.vectors[0], result.vectors[3])

    assert same_score >= settings.threshold
    assert changed_score >= settings.threshold
    assert different_score < settings.threshold
