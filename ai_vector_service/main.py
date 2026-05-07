import uvicorn

from ai_vector_service.api import Dependencies, UploadLimits, create_app
from ai_vector_service.config import load_settings
from ai_vector_service.images import PillowImageDecoder
from ai_vector_service.model import TransformersImageEmbeddingModel


settings = load_settings()
app = create_app(
    Dependencies(
        decoder=PillowImageDecoder(
            max_image_pixels=settings.max_image_pixels,
            target_size=(settings.target_width, settings.target_height),
        ),
        model=TransformersImageEmbeddingModel(
            settings.model_name,
            settings.model_revision,
            settings.device,
        ),
    ),
    UploadLimits(
        max_files=settings.max_files,
        max_upload_bytes=settings.max_upload_bytes,
    ),
)


def main() -> None:
    uvicorn.run(app, host=settings.host, port=settings.port)


if __name__ == "__main__":
    main()
