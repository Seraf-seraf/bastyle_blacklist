from __future__ import annotations

import argparse
from io import BytesIO
import json
from pathlib import Path
import time

from PIL import Image

from ai_vector_service.config import DEFAULT_MODEL_NAME, DEFAULT_MODEL_REVISION
from ai_vector_service.images import PillowImageDecoder
from ai_vector_service.index import FaissHNSWVectorIndex, HNSWConfig
from ai_vector_service.model import TransformersImageEmbeddingModel
from ai_vector_service.storage import StoredVectorFrame, VectorBan


def main() -> None:
    parser = argparse.ArgumentParser(description="Измерить FP/FN статичных изображений для кандидатных порогов AI-vector.")
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--ban-count", type=int, default=1000)
    parser.add_argument("--negative-count", type=int, default=1000)
    parser.add_argument("--thresholds", default="0.88,0.90,0.92,0.94,0.96")
    parser.add_argument("--batch-size", type=int, default=16)
    parser.add_argument("--top-k", type=int, default=5)
    parser.add_argument("--device", default="cpu")
    parser.add_argument("--model-name", default=DEFAULT_MODEL_NAME)
    parser.add_argument("--model-revision", default=DEFAULT_MODEL_REVISION)
    parser.add_argument("--output", type=Path, default=Path("data/ai-vector-benchmark/static-quality-report.json"))
    args = parser.parse_args()

    if args.ban_count <= 0 or args.negative_count <= 0:
        raise SystemExit("--ban-count и --negative-count должны быть положительными")

    thresholds = [float(part.strip()) for part in args.thresholds.split(",") if part.strip()]
    records = _read_manifest(args.manifest, args.ban_count + args.negative_count)
    ban_records = records[: args.ban_count]
    negative_records = records[args.ban_count : args.ban_count + args.negative_count]
    if len(negative_records) < args.negative_count:
        raise RuntimeError("манифест содержит недостаточно изображений для негативных примеров")

    decoder = PillowImageDecoder()
    model = TransformersImageEmbeddingModel(args.model_name, args.model_revision, args.device)

    started = time.perf_counter()
    ban_vectors = _embed_paths(model, decoder, [Path(record["path"]) for record in ban_records], args.batch_size)
    print(f"векторизовано банов={len(ban_vectors)} время={time.perf_counter() - started:.1f}s")

    variant_images = [_variant_image(Path(record["path"])) for record in ban_records]
    positive_vectors = _embed_images(model, variant_images, args.batch_size)
    print(f"векторизовано позитивных={len(positive_vectors)} время={time.perf_counter() - started:.1f}s")

    negative_vectors = _embed_paths(model, decoder, [Path(record["path"]) for record in negative_records], args.batch_size)
    print(f"векторизовано негативных={len(negative_vectors)} время={time.perf_counter() - started:.1f}s")

    index = FaissHNSWVectorIndex.build(
        dimension=len(ban_vectors[0]),
        bans=_bans_from_vectors(ban_vectors),
        config=HNSWConfig(),
    )

    positive_scores = []
    for offset, vector in enumerate(positive_vectors):
        hits = index.search(vector, args.top_k)
        expected_ban_id = offset + 1
        best_same = max((hit.score for hit in hits if hit.ban_id == expected_ban_id), default=-1.0)
        positive_scores.append(best_same)

    negative_scores = []
    for vector in negative_vectors:
        hits = index.search(vector, args.top_k)
        negative_scores.append(max((hit.score for hit in hits), default=-1.0))

    report = {
        "ban_count": len(ban_vectors),
        "negative_count": len(negative_vectors),
        "model_name": args.model_name,
        "model_revision": args.model_revision,
        "thresholds": [
            {
                "threshold": threshold,
                "false_negative_rate": sum(score < threshold for score in positive_scores) / len(positive_scores),
                "false_positive_rate": sum(score >= threshold for score in negative_scores) / len(negative_scores),
            }
            for threshold in thresholds
        ],
        "positive_scores": _score_summary(positive_scores),
        "negative_scores": _score_summary(negative_scores),
    }

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(report["thresholds"], indent=2, sort_keys=True))
    print(f"отчет: {args.output}")


def _read_manifest(path: Path, limit: int) -> list[dict]:
    records = []
    with path.open("r", encoding="utf-8") as file:
        for line in file:
            if line.strip():
                records.append(json.loads(line))
            if len(records) >= limit:
                break
    return records


def _embed_paths(model, decoder: PillowImageDecoder, paths: list[Path], batch_size: int) -> list[list[float]]:
    images = [decoder.decode(path.read_bytes()) for path in paths]
    return _embed_images(model, images, batch_size)


def _embed_images(model, images: list[Image.Image], batch_size: int) -> list[list[float]]:
    vectors: list[list[float]] = []
    for offset in range(0, len(images), batch_size):
        result = model.embed(images[offset : offset + batch_size])
        vectors.extend(result.vectors)
    return vectors


def _variant_image(path: Path) -> Image.Image:
    with Image.open(path) as image:
        rgb = image.convert("RGB")
        width, height = rgb.size
        crop_x = max(1, width // 40)
        crop_y = max(1, height // 40)
        cropped = rgb.crop((crop_x, crop_y, width - crop_x, height - crop_y))
        resized = cropped.resize((width, height), Image.Resampling.BICUBIC)
        buffer = BytesIO()
        resized.save(buffer, format="JPEG", quality=65, optimize=True)
        buffer.seek(0)
        with Image.open(buffer) as encoded:
            return encoded.convert("RGB")


def _bans_from_vectors(vectors: list[list[float]]) -> list[VectorBan]:
    bans = []
    for index, vector in enumerate(vectors):
        ban_id = index + 1
        bans.append(
            VectorBan(
                id=ban_id,
                chat_id=10,
                file_unique_id=f"quality-{ban_id}",
                media_type="photo",
                model_name="quality",
                model_revision="quality",
                vector_dim=len(vector),
                frames_count=1,
                active=True,
                created_at="2026-05-08T00:00:00Z",
                frames=[
                    StoredVectorFrame(
                        id=ban_id,
                        ban_id=ban_id,
                        frame_index=0,
                        position_millis=0,
                        vector=vector,
                    )
                ],
            )
        )
    return bans


def _score_summary(scores: list[float]) -> dict:
    ordered = sorted(scores)
    return {
        "min": ordered[0],
        "p50": _percentile(ordered, 50),
        "p95": _percentile(ordered, 95),
        "max": ordered[-1],
    }


def _percentile(ordered: list[float], percentile: int) -> float:
    index = round((len(ordered) - 1) * percentile / 100)
    return ordered[index]


if __name__ == "__main__":
    main()
