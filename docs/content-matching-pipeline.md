# Content Matching Pipeline

## Цель

Текущий бот блокирует повторную отправку медиа по `FileUniqueID`. Это хороший первый фильтр, но он не ловит измененные копии: пересжатие, обрезку, кроп, изменение цвета, рамки, водяные знаки, монтаж GIF/video.

Цель следующей версии: построить многоступенчатую проверку, где дешевые и точные методы выполняются первыми, а более дорогие и неточные методы подключаются только когда это оправдано.

## Краткая рекомендация

Для текущего Go-бота разумный порядок внедрения:

1. Оставить `FileUniqueID` как быстрый exact-match слой.
2. Добавить скачивание файла из Telegram только для медиа, которое нужно банить или проверять глубже.
3. Для фото и статичных стикеров добавить perceptual hash: сначала `pHash`/`dHash`.
4. Для GIF/video добавить извлечение кадров через FFmpeg и хранить набор perceptual hashes по sampled frames.
5. Для видео не полагаться на первый, средний и последний кадр. Использовать равномерную выборку кадров и порог по количеству совпавших кадров.
6. После накопления тестового датасета добавить AI embeddings как отдельный matcher, а не замену hash-слоям.
7. Для поиска по векторам использовать LanceDB или FAISS, если blacklist вырастет за пределы линейного скана.

## Основной принцип архитектуры

Сервис модерации не должен знать, каким способом определяется совпадение контента. Он должен работать с абстракцией:

```go
type ContentMatcher interface {
	Block(content domain.Content) error
	IsBlocked(content domain.Content) (MatchResult, error)
}
```

Конкретные реализации могут быть независимыми:

```text
ExactMatcher          -> Telegram file_unique_id
ImageHashMatcher      -> pHash/dHash/PDQ
VideoFrameMatcher     -> sampled frames + image hashes
VideoTMKMatcher       -> TMK/vPDQ style video fingerprint
EmbeddingMatcher      -> CLIP/MobileCLIP/SigLIP embeddings
CompositeMatcher      -> запускает matchers по порядку
```

`moderation.Service` вызывает только `ContentMatcher`. Telegram, FFmpeg, image decoding, ML runtime и vector database остаются в adapters/infrastructure.

## Предлагаемый pipeline

```text
Telegram message
  -> extract metadata
  -> exact match by file_unique_id
  -> if unknown: download media
  -> normalize media
  -> build content fingerprints
  -> run matchers from cheap to expensive
  -> decide: delete / allow / uncertain
  -> persist fingerprints when /ban is used
```

## Этап 1. Telegram exact match

### Технология

Telegram Bot API у медиа-объектов содержит `file_id` и `file_unique_id`. `file_unique_id` стабилен во времени и между ботами, но не используется для скачивания файла. Для скачивания используется `file_id` через `getFile`, после чего файл доступен по `https://api.telegram.org/file/bot<token>/<file_path>`. В обычном Bot API максимальный размер скачиваемого файла ограничен 20 MB; локальный Bot API server снимает часть ограничений.

Источники:

- Telegram Bot API, `file_unique_id`, `File`, `getFile`: https://core.telegram.org/bots/api
- Go Telegram Bot API library: https://github.com/go-telegram-bot-api/telegram-bot-api

### Плюсы

- Очень быстро.
- Не требует скачивания и декодирования файла.
- Почти нет false positive.
- Хорошо подходит как первый фильтр.

### Минусы

- Не ловит измененные копии.
- Не ловит пересжатие, обрезку, кроп, изменение контейнера.
- Не является content fingerprint в строгом смысле: это идентификатор файла в Telegram.

### Решение

Оставить как слой `ExactMatcher`. Он должен отвечать первым.

```text
if file_unique_id in blacklist:
    delete
else:
    continue deeper checks
```

## Этап 2. Download and media normalization

### Технология

Для глубоких проверок нужно скачать файл и привести его к стабильному виду:

- изображения: decode -> orientation fix -> resize -> grayscale/RGB normalization;
- GIF/video: decode -> sample frames -> normalize frames;
- stickers: учитывать разные форматы Telegram, включая animated/video stickers.

Для видео и GIF в Go практично использовать FFmpeg:

- `ffmpeg` CLI напрямую через `os/exec`;
- `github.com/u2takey/ffmpeg-go` как wrapper над FFmpeg CLI;
- `github.com/csnewman/ffmpeg-go` как bindings к FFmpeg libraries, если нужен более плотный контроль.

Источники:

- FFmpeg: https://ffmpeg.org/
- `u2takey/ffmpeg-go`: https://github.com/u2takey/ffmpeg-go
- `csnewman/ffmpeg-go`: https://github.com/csnewman/ffmpeg-go

### Плюсы

- FFmpeg покрывает большинство реальных форматов.
- Можно одинаково обрабатывать GIF, MP4, WebM и Telegram animations.
- Можно выбрать sampling strategy без изменения бизнес-логики.

### Минусы

- Скачивание и декодирование дороже exact match.
- FFmpeg добавляет системную зависимость.
- Нужно ограничивать размер, duration, CPU time и количество кадров.
- Ошибки декодирования должны быть нормальной частью pipeline, а не panic.

### Решение

Сделать отдельный порт:

```go
type MediaDownloader interface {
	Download(ctx context.Context, fileID string) (domain.MediaFile, error)
}

type FrameExtractor interface {
	ExtractFrames(ctx context.Context, media domain.MediaFile, plan SamplingPlan) ([]image.Image, error)
}
```

## Этап 3. Image perceptual hashes

### Технология

Perceptual hash преобразует изображение в короткий отпечаток, где похожие изображения имеют близкие hashes. Обычно сравнение идет через Hamming distance.

Варианты:

- `aHash`: быстрый average hash, слабее к изменениям.
- `dHash`: быстрый difference hash, часто хороший baseline.
- `pHash`: DCT-based hash, обычно устойчивее к ресайзу и пересжатию.
- `PDQ`: perceptual hash от Meta, разработан для moderation-like сценариев и масштабной сверки.

Go-библиотеки:

- `github.com/corona10/goimagehash`: AverageHash, DifferenceHash, PerceptionHash.
- `github.com/kotylevskiy/go-phash`: dependency-light pHash.

Meta PDQ:

- Reference implementation: https://github.com/facebook/ThreatExchange/tree/main/pdq
- PDQ README указывает стартовые thresholds: distance `<=31`, quality discard `<=49`; эти значения нужно калибровать на своих данных.

Источники:

- `goimagehash`: https://github.com/corona10/goimagehash
- `go-phash`: https://pkg.go.dev/github.com/kotylevskiy/go-phash
- Meta PDQ: https://github.com/facebook/ThreatExchange/tree/main/pdq

### Плюсы

- Быстро и просто.
- Хорошо ловит ресайз, легкое пересжатие, небольшие цветовые изменения.
- Хеши маленькие, легко хранить.
- Подходит для первого "похожего" слоя после exact match.

### Минусы

- Плохо переносит сильный кроп, рамки, вставки, поворот, мемные коллажи.
- Возможны false positive на простых/однотонных изображениях.
- Threshold нельзя брать вслепую: его нужно подобрать на реальных банах и обычных сообщениях.

### Решение

Для первой версии:

```text
Photo/static image -> normalize -> pHash and/or dHash -> compare by Hamming distance
```

Для более устойчивой версии:

```text
Photo/static image -> normalize -> global hash + region hashes + quality score
```

## Этап 4. GIF and video frame fingerprints

### Проблема

Для GIF/video один hash всего файла плохо работает. Если видео обрезали, изменили начало/конец, добавили интро, поменяли FPS или контейнер, exact hash и `FileUniqueID` не помогут.

Первый/средний/последний кадр тоже слабая стратегия:

- при trim все три точки могут измениться;
- у GIF часто первый кадр неинформативен;
- у видео начало и конец часто содержат заставки, переходы или черные кадры.

### Технология

Лучше хранить множество отпечатков:

```text
video/gif
  -> sample N frames
  -> normalize each frame
  -> hash each frame
  -> store ordered or unordered fingerprint set
```

Sampling strategies:

- fixed interval: 1 frame/sec;
- fixed budget: 10-30 кадров на весь ролик;
- scene-change sampling: брать кадры при заметном изменении сцены;
- hybrid: keyframes + равномерные кадры.

Matching strategies:

- unordered set overlap: сколько кадров нового видео похоже на кадры blacklist;
- ordered subsequence: совпавшие кадры идут в похожем порядке;
- sliding window: полезно для trimmed clips;
- weighted score: учитывать качество кадра, duration, position.

### Плюсы

- Хорошо переживает trim начала/конца.
- Ловит повторные GIF/video даже после пересжатия.
- Можно начать без ML.
- Простая отладка: можно показать совпавшие кадры.

### Минусы

- Дороже CPU и IO.
- Нужно хранить много hashes на один banned item.
- Сильный кроп, overlay, watermark и speed change могут ломать совпадение.
- Требуется аккуратный threshold, иначе появятся false positive на похожих сценах.

### Решение

Для MVP:

```text
Extract 12-20 frames per video/GIF
Hash each frame with pHash or PDQ
Match if >= 3 frames are within threshold
For very short GIF: require lower absolute count but higher ratio
```

Пример:

```text
banned: [h1 h2 h3 h4 h5 h6 h7 h8 h9]
new:          [x1 x2 x3 x4]

x1 ~ h3
x2 ~ h4
x3 ~ h5
=> likely match, even if beginning/end were trimmed
```

## Этап 5. Video-specific perceptual hashing

### Технология

Meta ThreatExchange содержит TMK, Temporal Match Kernel, алгоритм video similarity detection. В README проекта описано, что TMK использует frame features и двухуровневую scoring-модель. Там же указано, что TMK хорошо подходит для почти идентичных полных видео и как pre-filter, но слабее для clips/trimmed fragments.

Также в ThreatExchange есть vPDQ и PDQF-related tooling.

Источники:

- Meta TMK: https://github.com/facebook/ThreatExchange/tree/main/tmk
- Meta PDQ: https://github.com/facebook/ThreatExchange/tree/main/pdq
- Paper/test drive: https://arxiv.org/abs/1912.07745

### Плюсы

- Специализировано для video similarity.
- Лучше формализовано, чем самодельный набор кадров.
- Есть reference implementation и scoring model.

### Минусы

- Интеграция тяжелее, чем pHash over frames.
- Не Go-native.
- Требует FFmpeg.
- По документации TMK слабее именно для clips, а проблема trimmed video для бота важна.

### Решение

Не начинать с TMK. Рассматривать как второй video matcher после MVP на sampled frames. Для текущей задачи trimmed clips вероятнее проще и полезнее сначала сделать sliding-window frame-hash matcher.

## Этап 6. Local feature matching: ORB/SIFT/OpenCV

### Технология

Локальные признаки ищут характерные точки изображения и сравнивают дескрипторы. ORB доступен в OpenCV и рассчитан как быстрый feature detector/descriptor. В Go можно использовать GoCV.

Источники:

- OpenCV ORB tutorial: https://docs.opencv.org/3.4/d1/d89/tutorial_py_orb.html
- GoCV: https://gocv.io/

### Плюсы

- Лучше переносит кроп и частичные совпадения.
- Может помочь, когда global pHash ломается из-за рамки, crop или overlay.
- Объяснимый результат: можно визуализировать matched keypoints.

### Минусы

- Сильно сложнее и тяжелее perceptual hash.
- Плохо работает на однотонных, размытых, мемных или низкодетальных кадрах.
- OpenCV добавляет тяжелую системную зависимость.
- Порог matching нужно настраивать отдельно.

### Решение

Не делать первым этапом. Использовать как optional fallback для спорных совпадений или для banned items, где pHash дает много false negative.

## Этап 7. AI embeddings

### Технология

Модель кодирует изображение или кадр в vector embedding. Затем новый embedding сравнивается с blacklist embeddings через cosine similarity или dot product.

Кандидаты:

- CLIP/OpenCLIP-like image encoder;
- MobileCLIP для более легкого inference;
- SigLIP-like image encoder;
- EfficientNet/MobileNet as image embedding backbone;
- ONNX Runtime для запуска модели из Go.

Источники:

- OpenAI CLIP: https://github.com/openai/CLIP
- Apple MobileCLIP: https://github.com/apple/ml-mobileclip
- ONNX Runtime: https://onnxruntime.ai/docs/
- Go ONNX Runtime wrapper: https://github.com/yalue/onnxruntime_go

### Плюсы

- Лучше переносит crop, color changes, watermark, redraw, meme edits.
- Может находить семантически похожий контент, а не только near-duplicate.
- Хорошо расширяется на разные типы контента: images, sampled video frames.

### Минусы

- Дороже CPU/RAM.
- Появляются false positive: "похожий смысл" не всегда значит тот же banned content.
- Нужен vector index при росте базы.
- Нужна калибровка threshold и ручная проверка спорных случаев.
- Модель и preprocessing должны быть версионированы, иначе embeddings станут несовместимы.

### Решение

Добавлять после hash-based MVP. Не использовать embeddings как единственный критерий удаления. Лучше:

```text
exact match -> delete
strong perceptual match -> delete
embedding match above high threshold -> delete or mark as suspicious
embedding match in gray zone -> optional admin review/log
```

Для video:

```text
sample frames -> embeddings per frame -> aggregate top-k similarities
```

Не усреднять все кадры бездумно: короткий banned fragment может раствориться в длинном видео.

## Этап 8. Storage and search

### Exact and perceptual hashes

Для маленького blacklist достаточно in-memory map и линейного сравнения Hamming distance. При росте:

- хранить hashes в SQLite/PostgreSQL;
- добавлять bucket/index по префиксам hash;
- использовать BK-tree или VP-tree для Hamming distance;
- отдельно хранить metadata: media type, source chat, created_at, model/hash version.

### Vector embeddings

Для embeddings нужен vector search:

- LanceDB: embedded vector DB, есть индексы IVF/HNSW-style через IVF_HNSW variants.
- FAISS: библиотека Meta для similarity search/clustering dense vectors, сильна на больших объемах и GPU, но Go-интеграция обычно сложнее.

Источники:

- LanceDB vector indexes: https://docs.lancedb.com/indexing/vector-index
- LanceDB tables: https://docs.lancedb.com/tables
- FAISS: https://github.com/facebookresearch/faiss

### Плюсы LanceDB

- Удобнее как embedded database.
- Хорошо ложится на локальное приложение.
- Есть документация по vector indexes.

### Минусы LanceDB

- Дополнительная база и миграции.
- Нужно проверить зрелость Go SDK под вашу задачу.

### Плюсы FAISS

- Очень сильная библиотека для dense vector similarity.
- Подходит для больших индексов.
- Есть GPU-возможности.

### Минусы FAISS

- Основной стек C++/Python.
- В Go-проекте интеграция тяжелее.
- Это библиотека индекса, а не полноценное хранилище бизнес-метаданных.

## Proposed domain model

Текущий `domain.Message` содержит только metadata. Для усиленного matching нужен отдельный `domain.Content`, чтобы не смешивать Telegram message и проверяемый контент.

```go
package domain

type MediaKind string

const (
	MediaPhoto     MediaKind = "photo"
	MediaAnimation MediaKind = "animation"
	MediaSticker   MediaKind = "sticker"
	MediaVideo     MediaKind = "video"
)

type Content struct {
	FileID       string
	FileUniqueID string
	Kind         MediaKind
	MimeType     string
	SizeBytes    int64
	DurationSec  int
	Width        int
	Height       int
}
```

`FileID` нужен для скачивания, `FileUniqueID` нужен для exact match. `Kind`, `MimeType`, `DurationSec`, `Width`, `Height` нужны для выбора pipeline и ограничения ресурсов.

## Proposed interfaces

```go
type ContentMatcher interface {
	Block(ctx context.Context, content domain.Content) error
	IsBlocked(ctx context.Context, content domain.Content) (MatchResult, error)
}

type MatchResult struct {
	Blocked    bool
	Confidence float64
	Reason     string
	Matcher    string
}

type Fingerprinter interface {
	Fingerprint(ctx context.Context, media domain.MediaFile) (domain.FingerprintSet, error)
}

type FingerprintStore interface {
	Save(ctx context.Context, item domain.BlockedContent) error
	FindCandidates(ctx context.Context, fp domain.FingerprintSet) ([]domain.Candidate, error)
}
```

Composite matcher:

```text
CompositeMatcher
  -> ExactMatcher
  -> ImageHashMatcher
  -> VideoFrameMatcher
  -> EmbeddingMatcher
```

Каждый matcher должен возвращать не только `true/false`, но и причину. Это важно для логов, настройки thresholds и анализа false positives.

## Decision table

| Подход | Что ловит | Что ломает | Стоимость | Риск false positive | Когда внедрять |
|---|---|---|---:|---:|---|
| `file_unique_id` | тот же Telegram file | любое изменение файла | очень низкая | очень низкий | уже сейчас |
| cryptographic hash bytes | тот же скачанный файл | пересжатие, metadata, trim | низкая | очень низкий | optional, после download |
| dHash/aHash | похожие изображения | crop, сильные edits | низкая | средний | MVP image |
| pHash | resize/compression/color shifts | crop, overlay, collage | низкая-средняя | средний | MVP image |
| PDQ | moderation-like image matching | сложная интеграция, crop limits | средняя | ниже при quality threshold | после pHash MVP |
| sampled frame hashes | GIF/video trim, re-encode | heavy crop, speed change, overlays | средняя | средний | MVP video |
| TMK/vPDQ | near-duplicate videos | clips/trimmed weaker for TMK | средняя-высокая | средний | после MVP video |
| ORB/OpenCV | partial/cropped visual match | low-detail frames | высокая | средний-высокий | fallback |
| AI embeddings | semantic/edited similarity | threshold ambiguity | высокая | высокий без review | после dataset |

## Resource limits

Обязательные ограничения перед production:

- max download size;
- max video duration;
- max frames per item;
- max processing timeout;
- max concurrent fingerprint jobs;
- temp file cleanup;
- fingerprint versioning;
- structured logs with matcher name and score.

Для Telegram default Bot API учитывать лимит скачивания файлов 20 MB. Если нужно больше, рассмотреть Local Bot API Server.

## Threshold strategy

Thresholds нельзя выбирать только по README библиотек. Нужен локальный calibration set:

```text
positive set:
  original banned media
  re-encoded copies
  trimmed videos
  cropped images
  watermarked variants

negative set:
  обычные сообщения из чата
  похожие, но допустимые мемы
  однотонные/низкодетальные изображения
```

Метрики:

- false positive rate важнее, чем максимальный recall, потому что бот удаляет сообщения;
- для auto-delete использовать высокий threshold;
- для uncertain zone логировать или отправлять на admin review;
- хранить score и matcher в логах.

## Recommended implementation plan

### Step 1. Расширить domain

Добавить `domain.Content` и извлекать из Telegram:

- `FileID`;
- `FileUniqueID`;
- `Kind`;
- `MimeType`;
- `SizeBytes`;
- `DurationSec`;
- dimensions.

### Step 2. Заменить blacklist store на matcher abstraction

Текущий `BlacklistStore` хранит только строку. Нужно перейти к:

```text
ModerationService -> ContentMatcher
```

Сначала `ExactMatcher` может внутри использовать текущий map.

### Step 3. Добавить MediaDownloader

Telegram adapter скачивает файл только когда matcher требует глубокую проверку.

### Step 4. Добавить image pHash

Начать с `goimagehash`:

- быстро подключается;
- Go-native;
- достаточно для MVP.

### Step 5. Добавить video sampled frame matcher

Через FFmpeg:

```text
ffmpeg -i input.mp4 -vf fps=1,scale=320:-1 frames/%04d.jpg
```

В коде лучше спрятать это за `FrameExtractor`.

### Step 6. Собрать calibration set

Без этого нельзя уверенно включать auto-delete для fuzzy matching.

### Step 7. Добавить embeddings

Только после того, как есть:

- реальные positive/negative примеры;
- логи scores;
- понимание false positive.

## Final target architecture

```text
cmd/main.go
  -> telegram adapter
  -> moderation.Service

moderation.Service
  -> ports.ContentMatcher
  -> ports.MessageActions
  -> ports.AdminChecker

adapters/telegram
  -> Telegram message mapping
  -> file download
  -> send/delete actions
  -> admin check

adapters/matching
  -> exact matcher
  -> perceptual image matcher
  -> video frame matcher
  -> embedding matcher

adapters/media
  -> image decoder
  -> ffmpeg frame extractor
  -> normalizer

adapters/storage
  -> memory/sqlite/postgres/lancedb
```

Главная граница: `moderation.Service` решает, что делать с сообщением, но не знает, как скачать файл, как декодировать видео, какой hash считается и какая ML-модель используется.

## Sources

- Telegram Bot API: https://core.telegram.org/bots/api
- go-telegram-bot-api: https://github.com/go-telegram-bot-api/telegram-bot-api
- FFmpeg: https://ffmpeg.org/
- u2takey/ffmpeg-go: https://github.com/u2takey/ffmpeg-go
- csnewman/ffmpeg-go: https://github.com/csnewman/ffmpeg-go
- goimagehash: https://github.com/corona10/goimagehash
- go-phash: https://pkg.go.dev/github.com/kotylevskiy/go-phash
- Meta ThreatExchange PDQ: https://github.com/facebook/ThreatExchange/tree/main/pdq
- Meta ThreatExchange TMK: https://github.com/facebook/ThreatExchange/tree/main/tmk
- PDQ and TMK paper: https://arxiv.org/abs/1912.07745
- OpenCV ORB: https://docs.opencv.org/3.4/d1/d89/tutorial_py_orb.html
- GoCV: https://gocv.io/
- OpenAI CLIP: https://github.com/openai/CLIP
- Apple MobileCLIP: https://github.com/apple/ml-mobileclip
- ONNX Runtime: https://onnxruntime.ai/docs/
- yalue/onnxruntime_go: https://github.com/yalue/onnxruntime_go
- LanceDB vector indexes: https://docs.lancedb.com/indexing/vector-index
- LanceDB tables: https://docs.lancedb.com/tables
- FAISS: https://github.com/facebookresearch/faiss
