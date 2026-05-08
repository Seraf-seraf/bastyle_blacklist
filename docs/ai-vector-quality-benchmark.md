# AI Vector Quality And Benchmark

## Данные

Основной reproducible-набор для 10k/50k/100k проверок - COCO 2017 train.
Скрипт `ai_vector_service.benchmarks.download_coco` скачивает официальный
`annotations_trainval2017.zip`, берёт уникальные записи `images` из
`instances_train2017.json` и скачивает сами файлы по `coco_url`.

Команды:

```bash
make ai-data-10k
make ai-data-50k
make ai-data-100k
```

Результат:

```text
data/ai-vector-benchmark/coco2017/images/
data/ai-vector-benchmark/coco2017/manifest-10000.jsonl
data/ai-vector-benchmark/coco2017/manifest-50000.jsonl
data/ai-vector-benchmark/coco2017/manifest-100000.jsonl
```

## Static Quality

Проверка подбирает threshold на двух сценариях:

- positive: оригинал попадает в ban, запросом идёт изменённая версия этой же
  картинки через crop + resize + JPEG recompress;
- negative: запросом идут другие уникальные COCO-картинки, которых не было в
  ban-наборе.

Запуск:

```bash
make ai-static-quality MANIFEST=data/ai-vector-benchmark/coco2017/manifest-10000.jsonl
```

Выходной файл:

```text
data/ai-vector-benchmark/static-quality-report.json
```

В отчёте:

- `false_negative_rate` - доля positive-запросов, где свой ban не набрал
  threshold;
- `false_positive_rate` - доля negative-запросов, где любой ban набрал
  threshold;
- `positive_scores` и `negative_scores` - summary ближайших score-ов.

## HNSW Benchmark

Для benchmark на реальных embeddings сначала надо посчитать vectors:

```bash
python3 -m ai_vector_service.benchmarks.embed_images \
  --manifest data/ai-vector-benchmark/coco2017/manifest-100000.jsonl \
  --output data/ai-vector-benchmark/coco100k-vectors.npz \
  --batch-size 16
```

Затем:

```bash
make ai-hnsw-bench VECTORS=data/ai-vector-benchmark/coco100k-vectors.npz
```

Выходной файл:

```text
data/ai-vector-benchmark/hnsw-report.json
```

Без `VECTORS` benchmark использует synthetic normalized vectors:

```bash
make ai-hnsw-bench
```

Это не заменяет quality-тесты на реальных картинках, но быстро показывает
latency scaling индекса на 10k/50k/100k.
