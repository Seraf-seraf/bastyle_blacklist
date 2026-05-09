from __future__ import annotations

import argparse
import json
from pathlib import Path
import statistics
import time

import numpy as np

from ai_vector_service.index import FaissHNSWVectorIndex, HNSWConfig
from ai_vector_service.storage import StoredVectorFrame, VectorBan


DEFAULT_SIZES = (10_000, 50_000, 100_000)


def main() -> None:
    parser = argparse.ArgumentParser(description="Бенчмарк поиска Faiss HNSW на реальных или синтетических векторах.")
    parser.add_argument("--vectors", type=Path, help="NPZ, созданный embed_images.py")
    parser.add_argument("--sizes", default="10000,50000,100000")
    parser.add_argument("--queries", type=int, default=1000)
    parser.add_argument("--top-k", type=int, default=5)
    parser.add_argument("--dimension", type=int, default=768, help="используется только без --vectors")
    parser.add_argument("--output", type=Path, default=Path("data/ai-vector-benchmark/hnsw-report.json"))
    parser.add_argument("--add-batch-size", type=int, default=2048)
    parser.add_argument("--m", type=int, default=32)
    parser.add_argument("--ef-construction", type=int, default=80)
    parser.add_argument("--ef-search", type=int, default=64)
    args = parser.parse_args()
    if args.add_batch_size <= 0:
        raise SystemExit("--add-batch-size должен быть положительным")

    sizes = _parse_sizes(args.sizes)
    vectors = _load_vectors(args.vectors, max(sizes), args.dimension)
    config = HNSWConfig(m=args.m, ef_construction=args.ef_construction, ef_search=args.ef_search)
    report = []

    for size in sizes:
        if size > len(vectors):
            print(f"пропуск размера={size}: доступно только {len(vectors)} векторов")
            continue

        subset = _normalize(vectors[:size])
        started = time.perf_counter()
        index = FaissHNSWVectorIndex(dimension=subset.shape[1], config=config)
        for offset in range(0, size, args.add_batch_size):
            index.add_bans(_bans_from_vectors(subset[offset : offset + args.add_batch_size], id_offset=offset))
        build_seconds = time.perf_counter() - started

        query_count = min(args.queries, size)
        query_ids = np.linspace(0, size - 1, num=query_count, dtype=np.int64)
        latencies_ms = []
        recall_hits = 0
        for query_id in query_ids:
            query = subset[int(query_id)]
            started = time.perf_counter()
            hits = index.search(query.tolist(), args.top_k)
            latencies_ms.append((time.perf_counter() - started) * 1000)
            if hits and hits[0].ban_id == int(query_id) + 1:
                recall_hits += 1

        row = {
            "vectors": size,
            "dimension": int(subset.shape[1]),
            "queries": query_count,
            "top_k": args.top_k,
            "build_seconds": build_seconds,
            "search_ms_avg": statistics.fmean(latencies_ms),
            "search_ms_p50": _percentile(latencies_ms, 50),
            "search_ms_p95": _percentile(latencies_ms, 95),
            "self_recall_at_1": recall_hits / query_count,
            "hnsw": {
                "m": args.m,
                "ef_construction": args.ef_construction,
                "ef_search": args.ef_search,
            },
        }
        report.append(row)
        print(json.dumps(row, sort_keys=True))

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"отчет: {args.output}")


def _parse_sizes(raw: str) -> list[int]:
    sizes = [int(part.strip()) for part in raw.split(",") if part.strip()]
    if not sizes or any(size <= 0 for size in sizes):
        raise SystemExit("--sizes должен содержать положительные целые числа")
    return sizes


def _load_vectors(path: Path | None, count: int, dimension: int) -> np.ndarray:
    if path is None:
        rng = np.random.default_rng(20260508)
        return rng.normal(size=(count, dimension)).astype(np.float32)

    payload = np.load(path)
    vectors = np.asarray(payload["vectors"], dtype=np.float32)
    if vectors.ndim != 2:
        raise RuntimeError("массив векторов должен быть двумерным")
    return vectors


def _normalize(vectors: np.ndarray) -> np.ndarray:
    norms = np.linalg.norm(vectors, axis=1, keepdims=True)
    if np.any(norms == 0):
        raise RuntimeError("нулевой вектор не поддерживается")
    return vectors / norms


def _bans_from_vectors(vectors: np.ndarray, *, id_offset: int) -> list[VectorBan]:
    bans = []
    for index, vector in enumerate(vectors):
        ban_id = id_offset + index + 1
        bans.append(
            VectorBan(
                id=ban_id,
                chat_id=10,
                file_unique_id=f"bench-{ban_id}",
                media_type="photo",
                model_name="benchmark",
                model_revision="benchmark",
                vector_dim=int(vectors.shape[1]),
                frames_count=1,
                created_at="2026-05-08T00:00:00Z",
                active=True,
                frames=[
                    StoredVectorFrame(
                        id=ban_id,
                        ban_id=ban_id,
                        frame_index=0,
                        position_millis=0,
                        vector=vector.tolist(),
                    )
                ],
            )
        )
    return bans


def _percentile(values: list[float], percentile: int) -> float:
    ordered = sorted(values)
    index = round((len(ordered) - 1) * percentile / 100)
    return ordered[index]


if __name__ == "__main__":
    main()
