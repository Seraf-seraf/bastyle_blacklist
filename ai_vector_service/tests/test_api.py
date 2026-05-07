from io import BytesIO

from fastapi.testclient import TestClient
from PIL import Image

from ai_vector_service.api import Dependencies, UploadLimits, create_app
from ai_vector_service.images import PillowImageDecoder
from ai_vector_service.model import EmbeddingResult


class FakeModel:
    model_name = "fake-model"

    def embed(self, images):
        vectors = [[1.0, 0.0, 0.0] for _ in images]
        return EmbeddingResult(vectors=vectors, dimension=3, model_name=self.model_name)


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
    assert response.json()["detail"] == "image data is not a supported image"


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
    assert response.json()["detail"] == "too many files"


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
    assert response.json()["detail"] == "upload size exceeds limit"


def test_embed_rejects_image_over_pixel_limit():
    client = TestClient(
        create_app(Dependencies(PillowImageDecoder(max_image_pixels=16), FakeModel()))
    )

    response = client.post(
        "/embed",
        files=[("files", ("frame.png", png_bytes(), "image/png"))],
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "image pixel count exceeds limit"
