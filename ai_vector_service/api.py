from dataclasses import dataclass
from typing import Annotated

from fastapi import FastAPI, File, HTTPException, UploadFile
from pydantic import BaseModel, Field

from ai_vector_service.images import ImageDecodeError, ImageDecoder
from ai_vector_service.model import ImageEmbeddingModel


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


@dataclass(frozen=True)
class Dependencies:
    decoder: ImageDecoder
    model: ImageEmbeddingModel


@dataclass(frozen=True)
class UploadLimits:
    max_files: int = 10
    max_upload_bytes: int = 20 * 1024 * 1024


def create_app(dependencies: Dependencies, limits: UploadLimits | None = None) -> FastAPI:
    limits = limits or UploadLimits()
    app = FastAPI(title="Bastyle AI Vector Service")

    @app.get("/health", response_model=HealthResponse)
    def health() -> HealthResponse:
        return HealthResponse(status="ok", model_name=dependencies.model.model_name)

    @app.post("/embed", response_model=EmbedResponse)
    async def embed(
        files: Annotated[list[UploadFile], File(description="One image or ordered frames")],
    ) -> EmbedResponse:
        if len(files) > limits.max_files:
            raise HTTPException(status_code=413, detail="too many files")

        images = []
        upload_bytes = 0
        for file in files:
            remaining_bytes = limits.max_upload_bytes - upload_bytes
            if remaining_bytes <= 0:
                raise HTTPException(status_code=413, detail="upload size exceeds limit")

            data = await file.read(remaining_bytes + 1)
            upload_bytes += len(data)
            if upload_bytes > limits.max_upload_bytes:
                raise HTTPException(status_code=413, detail="upload size exceeds limit")

            try:
                images.append(dependencies.decoder.decode(data))
            except ImageDecodeError as err:
                raise HTTPException(status_code=400, detail=str(err)) from err

        try:
            result = dependencies.model.embed(images)
        except ValueError as err:
            raise HTTPException(status_code=400, detail=str(err)) from err
        except RuntimeError as err:
            raise HTTPException(status_code=500, detail=str(err)) from err

        return EmbedResponse(
            model_name=result.model_name,
            dimension=result.dimension,
            frames=[
                FrameEmbedding(frame_index=index, vector=vector)
                for index, vector in enumerate(result.vectors)
            ],
        )

    return app
