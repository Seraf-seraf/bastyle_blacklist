from __future__ import annotations

import argparse
import concurrent.futures
import hashlib
import json
from pathlib import Path
import time
from urllib.error import URLError
from urllib.request import urlopen
from zipfile import ZipFile


ANNOTATIONS_URL = "http://images.cocodataset.org/annotations/annotations_trainval2017.zip"
TRAIN_IMAGES_KEY = "annotations/instances_train2017.json"
DEFAULT_OUTPUT_DIR = Path("data/ai-vector-benchmark/coco2017")


def main() -> None:
    parser = argparse.ArgumentParser(description="Скачать уникальные изображения COCO 2017 train для AI-vector бенчмарков.")
    parser.add_argument("--limit", type=int, default=100_000, help="количество уникальных изображений для скачивания")
    parser.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT_DIR)
    parser.add_argument("--workers", type=int, default=16)
    parser.add_argument("--timeout", type=float, default=30.0)
    parser.add_argument("--retries", type=int, default=3)
    args = parser.parse_args()

    if args.limit <= 0:
        raise SystemExit("--limit должен быть положительным")
    if args.workers <= 0:
        raise SystemExit("--workers должен быть положительным")

    args.output_dir.mkdir(parents=True, exist_ok=True)
    annotations_zip = args.output_dir / "annotations_trainval2017.zip"
    if not annotations_zip.exists():
        print(f"загрузка аннотаций: {ANNOTATIONS_URL}")
        _download_file(ANNOTATIONS_URL, annotations_zip, args.timeout)

    images = _load_train_images(annotations_zip, args.limit)
    images_dir = args.output_dir / "images"
    images_dir.mkdir(parents=True, exist_ok=True)
    manifest_path = args.output_dir / f"manifest-{args.limit}.jsonl"

    existing = _load_existing_manifest(manifest_path)
    todo = [image for image in images if image["id"] not in existing]
    print(f"цель={len(images)} существующих={len(existing)} осталось={len(todo)}")

    with manifest_path.open("a", encoding="utf-8") as manifest:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as executor:
            futures = [
                executor.submit(_download_image, image, images_dir, args.timeout, args.retries)
                for image in todo
            ]
            completed = len(existing)
            for future in concurrent.futures.as_completed(futures):
                record = future.result()
                if record is None:
                    continue
                manifest.write(json.dumps(record, sort_keys=True) + "\n")
                manifest.flush()
                completed += 1
                if completed % 1000 == 0:
                    print(f"загружено {completed}/{len(images)}")

    print(f"манифест: {manifest_path}")


def _load_train_images(annotations_zip: Path, limit: int) -> list[dict]:
    with ZipFile(annotations_zip) as archive:
        with archive.open(TRAIN_IMAGES_KEY) as file:
            payload = json.load(file)

    images = payload["images"]
    images.sort(key=lambda item: item["id"])
    selected = []
    seen: set[int] = set()
    for image in images:
        image_id = int(image["id"])
        if image_id in seen:
            continue
        seen.add(image_id)
        selected.append(
            {
                "id": image_id,
                "file_name": image["file_name"],
                "url": image["coco_url"],
                "width": image["width"],
                "height": image["height"],
            }
        )
        if len(selected) >= limit:
            break

    if len(selected) < limit:
        raise RuntimeError(f"COCO train2017 содержит только {len(selected)} уникальных изображений")
    return selected


def _load_existing_manifest(path: Path) -> set[int]:
    if not path.exists():
        return set()

    existing = set()
    with path.open("r", encoding="utf-8") as file:
        for line in file:
            if line.strip():
                existing.add(int(json.loads(line)["id"]))
    return existing


def _download_image(image: dict, images_dir: Path, timeout: float, retries: int) -> dict | None:
    path = images_dir / image["file_name"]
    if not path.exists():
        for attempt in range(1, retries + 1):
            try:
                _download_file(image["url"], path, timeout)
                break
            except (OSError, URLError) as err:
                if path.exists():
                    path.unlink()
                if attempt == retries:
                    print(f"ошибка {image['url']}: {err}")
                    return None
                time.sleep(min(2**attempt, 10))

    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    return {
        **image,
        "path": str(path),
        "sha256": digest,
        "bytes": path.stat().st_size,
    }


def _download_file(url: str, path: Path, timeout: float) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp_path = path.with_suffix(path.suffix + ".tmp")
    with urlopen(url, timeout=timeout) as response:
        with tmp_path.open("wb") as file:
            while True:
                chunk = response.read(1024 * 1024)
                if not chunk:
                    break
                file.write(chunk)
    tmp_path.replace(path)


if __name__ == "__main__":
    main()
