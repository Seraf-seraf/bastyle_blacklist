from dataclasses import dataclass
from typing import Protocol, Sequence

from PIL import Image


# Nomic vision model uses custom Transformers code; safetensors only covers weights.
TRUST_REMOTE_CODE = True


@dataclass(frozen=True)
class EmbeddingResult:
    vectors: list[list[float]]
    dimension: int
    model_name: str


class ImageEmbeddingModel(Protocol):
    @property
    def model_name(self) -> str:
        pass

    def embed(self, images: Sequence[Image.Image]) -> EmbeddingResult:
        pass


class TransformersImageEmbeddingModel:
    def __init__(
        self,
        model_name: str,
        model_revision: str,
        device: str,
    ) -> None:
        import torch
        import torch.nn.functional as functional
        from transformers import AutoImageProcessor, AutoModel

        self._torch = torch
        self._functional = functional
        self._model_name = model_name
        self._model_revision = model_revision
        self._device = torch.device(device)
        self._processor = AutoImageProcessor.from_pretrained(
            model_name,
            revision=model_revision,
            trust_remote_code=TRUST_REMOTE_CODE,
        )
        self._model = AutoModel.from_pretrained(
            model_name,
            revision=model_revision,
            use_safetensors=True,
            trust_remote_code=TRUST_REMOTE_CODE,
        )
        self._model.to(self._device)
        self._model.eval()

    @property
    def model_name(self) -> str:
        return self._model_name

    def embed(self, images: Sequence[Image.Image]) -> EmbeddingResult:
        if not images:
            raise ValueError("требуется хотя бы одно изображение")

        inputs = self._processor(list(images), return_tensors="pt")
        inputs = {key: value.to(self._device) for key, value in inputs.items()}

        with self._torch.no_grad():
            output = self._model(**inputs)

        embeddings = output.last_hidden_state[:, 0]
        embeddings = self._functional.normalize(embeddings, p=2, dim=1)
        vectors = embeddings.detach().cpu().numpy().astype("float32")

        return EmbeddingResult(
            vectors=vectors.tolist(),
            dimension=int(vectors.shape[1]),
            model_name=self._model_name,
        )
