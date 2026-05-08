# Bastyle Blacklist

Версия: `v1.1.1`

`Bastyle Blacklist` - Telegram-бот для автоматической модерации медиа в
групповых чатах. Бот помогает администраторам один раз заблокировать нежеланный
контент и дальше автоматически удаляет повторные отправки этого же или похожего
медиа.

## Назначение

Задача бота - уменьшить ручную модерацию в Telegram-группах, где
пользователи повторно отправляют запрещенные картинки, GIF-анимации или
стикеры.

Основной сценарий:

1. Администратор отвечает на нежелательное медиа командой `/ban`.
2. Бот проверяет права администратора.
3. Бот сохраняет fingerprint контента в blacklist.
4. Бот удаляет исходное сообщение.
5. При повторной отправке совпадающего контента бот удаляет сообщение
   автоматически.

## Возможности `v1.1.0`

- exact-match по Telegram `file_unique_id`;
- perceptual hash matching для фото и статичных стикеров;
- video-like matching для Telegram animations и video stickers;
- AI vector matching для визуально похожих изображений и кадров;
- извлечение кадров через FFmpeg для GIF/video-like контента;
- SQLite-хранилище fingerprints;
- проверка прав администратора перед `/ban`;
- автоматическое удаление заблокированных сообщений;
- настройка лимитов для animation/video sticker обработки;
- отключаемый `ai_vector` matcher через YAML config.

## Ограничения бота

Вне зоны ответственности:

- Telegram видео и документы;
- видеоматериалы со значительными изменениями: кадрированием (crop), оверлеями, водяными знаками, изменением скорости или агрессивным монтажом.

## Требования

- Go `1.25.4` или совместимая версия;
- Telegram bot token от `@BotFather`;
- FFmpeg для video-like matching и AI vector matching кадров;
- Python service dependencies из `ai_vector_service/requirements.txt`, если включен
  `matching.ai_vector.enabled`;
- доступ на запись к SQLite-файлу;
- бот добавлен в группу администратором;
- у бота есть право удалять сообщения.

Проверить FFmpeg:

```bash
ffmpeg -version
```

## Настройка В Telegram

### 1. Создать Бота

Создайте бота через `@BotFather` и получите token. Token указывается в
`configs/config.yaml`:

```yaml
telegram:
  token: "your-telegram-bot-token"
```

### 2. Включить Inline Mode

В `@BotFather` включите inline mode для бота.

Ожидаемое состояние: inline mode включен, placeholder может быть `Search...`.

![Inline mode](docs/assets/botfather-inline-mode.png)

### 3. Добавить Команды

В `@BotFather` настройте команды бота.

Ожидаемый список команд:

![Bot commands](docs/assets/botfather-commands.png)

Команды в runtime:

- `/hello` - проверочная команда, отвечает `Hello, World!`;
- `/ban` - блокирует медиа из сообщения, на которое сделан reply.

### 4. Добавить Бота В Группу

Добавьте бота в группу как администратора.

Минимально необходимое право:

- `Delete messages`.

У бота должно быть право удалять сообщения. Остальные права можно не выдавать,
если они не нужны для вашей группы.

![Admin rights](docs/assets/telegram-admin-rights.png)

## Конфигурация

Создайте рабочий config:

```bash
cp configs/config.example.yaml configs/config.yaml
```

Пример полной конфигурации:

```yaml
telegram:
  token: "your-telegram-bot-token"
  update_timeout_seconds: 5
  http_client:
    enabled: false
    proxy_url: ""

workers: 5
jobs_buffer: 100

health:
  enabled: true
  host: "127.0.0.1"
  port: 8081

media_config:
  max_animation_duration: 10s
  max_video_sticker_duration: 3s
  max_animation_size: 20MiB
  max_video_sticker_size: 256KiB
  max_frames: 10
  target_width: 320
  target_height: 320
  max_upload_bytes: 20MiB
  max_image_pixels: 16777216
  ffmpeg_binary: ffmpeg
  ffmpeg_timeout: 10s

matching:
  exact:
    buffer: 500

  image_hash:
    db_path: "bastyle.sqlite"
    threshold: 12
    buffer: 500

  video_like:
    db_path: "bastyle.sqlite"
    threshold: 12
    buffer: 500
    min_matched_frames: 2
    min_matched_ratio: 0.4

  ai_vector:
    enabled: false
    model_name: "nomic-ai/nomic-embed-vision-v1.5"
    model_revision: "e3a725bce72db07ca4adb1d83da08903f3ee02f8"
    device: "cpu"
    db_path: "bastyle.sqlite"
    index_path: "faiss-image.index"
    max_files: 10
    threshold: 0.92
    top_k: 5
    min_matched_frames: 2
    min_matched_ratio: 0.4
    request_timeout: 10s
    service:
      host: "bastyle-ai-vector"
      port: 8080
    hnsw:
      m: 32
      ef_construction: 80
      ef_search: 64
```

### Основные Параметры

`telegram.token` - token Telegram-бота.
`telegram.http_client.enabled` - включает инициализацию Telegram Bot API через
настроенный HTTP client.
`telegram.http_client.proxy_url` - proxy URL для HTTP client. Если значение
пустое, используется proxy из окружения.

`workers` - количество worker'ов для обработки сообщений.
`jobs_buffer` - размер очереди сообщений.
`health.enabled` - включает HTTP health endpoint.
`health.host` и `health.port` - host/port для health endpoint.

`matching.exact.buffer` - стартовый размер черного списка по `file_unique_id` в памяти приложения.
`matching.image_hash.db_path` - SQLite-файл для хэшей картинок.
`matching.image_hash.threshold` - максимальная Hamming distance для похожих изображений.
`media_config.*` - общие лимиты и FFmpeg-настройки для кадров, которые
используют matchers `video_like` и `ai_vector`.
`matching.video_like.db_path` - SQLite-файл для video-like отпечатков.
`matching.video_like.min_matched_frames` и `min_matched_ratio` - правило
совпадения кадров для video-like hash matcher-а.
`matching.ai_vector.enabled` - включает или отключает AI vector matcher.
`matching.ai_vector.db_path` - SQLite-файл для AI-vector ban'ов.
`matching.ai_vector.index_path` - файл Faiss HNSW индекса.
`matching.ai_vector.threshold` - минимальный cosine similarity score.
`matching.ai_vector.top_k` - сколько ближайших векторов запрашивать у AI service.
`matching.ai_vector.min_matched_frames` и `min_matched_ratio` - правило
совпадения кадров для AI-vector matcher-а.
`matching.ai_vector.service.host` и `matching.ai_vector.service.port` - host/port
AI vector service.
`matching.ai_vector.hnsw.m`, `ef_construction`, `ef_search` - параметры HNSW
индекса Faiss.

## Запуск

Локально:

```bash
go run ./cmd/main.go -config configs/config.yaml
```

После успешного запуска бот пишет в лог:

```text
Authorized as <bot_username>
```

## Makefile И CI

Основные команды:

```bash
make test
make build
make ci
make up
make down
```

`make ci` запускает `go vet`, тесты и сборку бинарника в
`build/bin/bastyle-blacklist`.

В репозитории добавлен GitHub Actions workflow `.github/workflows/ci.yml`.
Он запускает `make ci` и отдельно проверяет сборку Docker-образа.

## Docker

Сборка:

```bash
docker build -f build/Dockerfile -t bastyle-blacklist:1.1.1 .
```

Запуск:

```bash
docker run --rm \
  --memory 512m \
  --memory-swap 512m \
  -v "$PWD/configs/config.yaml:/etc/bastyle/config.yaml:ro" \
  -v bastyle-data:/var/lib/bastyle \
  bastyle-blacklist:1.1.1
```

Для Docker удобно указывать SQLite-файл внутри `/var/lib/bastyle`:

```yaml
matching:
  image_hash:
    db_path: "/var/lib/bastyle/bastyle.sqlite"
  video_like:
    db_path: "/var/lib/bastyle/bastyle.sqlite"
  ai_vector:
    db_path: "/var/lib/bastyle/bastyle.sqlite"
    index_path: "/var/lib/bastyle/faiss-image.index"
```

## Docker Compose

Запуск через Compose:

```bash
cp configs/config.example.yaml configs/config.yaml
make up
```

Остановка:

```bash
make down
```

Compose монтирует config в `/etc/bastyle/config.yaml` для Go-бота, в
`/app/configs/config.yaml` для AI-сервиса и общий volume `/var/lib/bastyle` для
SQLite, Faiss index и runtime-данных. Контейнер Go-бота ограничен `512m`
памяти, контейнер `bastyle-ai-vector` - `2g`.

## Systemd

Сборка и установка бинарника:

```bash
make build
sudo install -o root -g root -m 0755 build/bin/bastyle-blacklist /usr/local/bin/bastyle-blacklist
```

Подготовка пользователя, config и директории данных:

```bash
id -u bastyle_bot >/dev/null 2>&1 || sudo useradd --system --home-dir /var/lib/bastyle --create-home --shell /usr/sbin/nologin bastyle_bot
sudo install -d -o bastyle_bot -g bastyle_bot -m 0750 /var/lib/bastyle
sudo install -d -o root -g root -m 0755 /etc/bastyle
sudo install -o root -g root -m 0640 configs/config.example.yaml /etc/bastyle/config.yaml
```

В `/etc/bastyle/config.yaml` укажите Telegram token и production-пути к SQLite:

```yaml
matching:
  image_hash:
    db_path: "/var/lib/bastyle/bastyle.sqlite"
  video_like:
    db_path: "/var/lib/bastyle/bastyle.sqlite"
  ai_vector:
    db_path: "/var/lib/bastyle/bastyle.sqlite"
    index_path: "/var/lib/bastyle/faiss-image.index"
```

Установка unit-файла:

```bash
sudo install -o root -g root -m 0644 build/bastyle-blacklist.service /etc/systemd/system/bastyle-blacklist.service
sudo systemctl daemon-reload
sudo systemctl enable --now bastyle-blacklist
```

Проверка:

```bash
systemctl status bastyle-blacklist
journalctl -u bastyle-blacklist -f
```

Unit запускает бот от пользователя `bastyle_bot`, хранит рабочие данные в
`/var/lib/bastyle`, читает config из `/etc/bastyle/config.yaml` и ограничивает
процесс `512M` памяти.

## Использование

Заблокировать медиа:

1. Найдите сообщение с нежелательным медиа.
2. Ответьте на него командой `/ban`.
3. Бот удалит сообщение и сохранит fingerprint.

Проверить работу:

1. Отправьте то же медиа повторно.
2. Бот должен удалить сообщение автоматически.

Если `/ban` отправил не администратор, бот не добавит контент в blacklist.

## Проверка Перед Релизом

Тесты:

```bash
go test ./...
```

Benchmark поиска по video-like index:

```bash
go test ./internal/adapters/matching/videolike \
  -bench BenchmarkVideoLikeLinearIndexSearch \
  -run '^$' \
  -benchtime=3s \
  -count=1
```

Benchmark FFmpeg extraction:

```bash
go test ./internal/adapters/media \
  -bench BenchmarkFFmpegFrameExtractorSmallAnimation \
  -run '^$' \
  -benchtime=3s \
  -count=1
```

AI-vector quality и HNSW benchmark запускаются отдельно, потому что они могут
качать десятки гигабайт изображений и долго считать embeddings:

```bash
make ai-data-10k
make ai-data-50k
make ai-data-100k
```

Эти команды берут уникальные изображения из COCO 2017 train через официальный
`coco_url` из `annotations_trainval2017.zip` и пишут manifest в
`data/ai-vector-benchmark/coco2017/manifest-<N>.jsonl`.

Статичный quality-report по похожим и непохожим изображениям:

```bash
make ai-static-quality MANIFEST=data/ai-vector-benchmark/coco2017/manifest-10000.jsonl
```

Отчёт содержит false positive / false negative для набора thresholds и summary
score-ов. Похожие изображения строятся из забаненных оригиналов через небольшой
crop + resize + JPEG recompress, непохожие берутся из следующей части manifest.

HNSW benchmark на 10k/50k/100k:

```bash
python3 -m ai_vector_service.benchmarks.embed_images \
  --manifest data/ai-vector-benchmark/coco2017/manifest-100000.jsonl \
  --output data/ai-vector-benchmark/coco100k-vectors.npz \
  --batch-size 16

make ai-hnsw-bench VECTORS=data/ai-vector-benchmark/coco100k-vectors.npz
```

Без `VECTORS` benchmark запускается на synthetic normalized vectors, что удобно
для проверки роста latency без долгого прогона модели.

## Что Изменилось В `v1.1.0`

- добавлена поддержка блокировок gif и анимированных стикеров;
- добавлены tests и benchmarks для video-like слоя.
