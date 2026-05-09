from __future__ import annotations

import argparse
import json
from pathlib import Path
import time

import numpy as np

from ai_vector_service.config import DEFAULT_MODEL_NAME, DEFAULT_MODEL_REVISION
from ai_vector_service.images import PillowImageDecoder
from ai_vector_service.model import TransformersImageEmbeddingModel


def main() -> None:
    parser = argparse.ArgumentParser(description="Векторизовать benchmark-изображения и сохранить векторы в NPZ.")
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--limit", type=int, default=0, help="0 означает все строки манифеста")
    parser.add_argument("--batch-size", type=int, default=16)
    parser.add_argument("--device", default="cpu")
    parser.add_argument("--model-name", default=DEFAULT_MODEL_NAME)
    parser.add_argument("--model-revision", default=DEFAULT_MODEL_REVISION)
    parser.add_argument("--target-width", type=int, default=320)
    parser.add_argument("--target-height", type=int, default=320)
    parser.add_argument("--max-image-pixels", type=int, default=4096 * 4096)
    args = parser.parse_args()

    if args.batch_size <= 0:
        raise SystemExit("--batch-size должен быть положительным")

    records = _read_manifest(args.manifest, args.limit)
    decoder = PillowImageDecoder(
        max_image_pixels=args.max_image_pixels,
        target_size=(args.target_width, args.target_height),
    )
    model = TransformersImageEmbeddingModel(args.model_name, args.model_revision, args.device)

    ids: list[int] = []
    paths: list[str] = []
    vectors = []
    started = time.perf_counter()
    for offset in range(0, len(records), args.batch_size):
        batch = records[offset : offset + args.batch_size]
        images = [decoder.decode(Path(record["path"]).read_bytes()) for record in batch]
        result = model.embed(images)
        vectors.extend(result.vectors)
        ids.extend(int(record["id"]) for record in batch)
        paths.extend(str(record["path"]) for record in batch)

        embedded = min(offset + len(batch), len(records))
        elapsed = time.perf_counter() - started
        print(f"векторизовано {embedded}/{len(records)} время={elapsed:.1f}s")

    args.output.parent.mkdir(parents=True, exist_ok=True)
    np.savez_compressed(
        args.output,
        ids=np.asarray(ids, dtype=np.int64),
        paths=np.asarray(paths),
        vectors=np.asarray(vectors, dtype=np.float32),
        model_name=np.asarray([args.model_name]),
        model_revision=np.asarray([args.model_revision]),
    )
    print(f"векторы: {args.output}")


def _read_manifest(path: Path, limit: int) -> list[dict]:
    records = []
    with path.open("r", encoding="utf-8") as file:
        for line in file:
            if line.strip():
                records.append(json.loads(line))
            if limit > 0 and len(records) >= limit:
                break

    if not records:
        raise RuntimeError("манифест не содержит изображений")
    return records


if __name__ == "__main__":
    main()
