# Bastyle Blacklist

Версия: `v1.2.1`

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

## Возможности `v1.2.1`

- exact-совпадение по Telegram `file_unique_id`;
- perceptual hash matching для фото и статичных стикеров;
- video-like matching для Telegram animations и video stickers;
- AI-vector matching для визуально похожих изображений, статичных стикеров,
  GIF-анимаций, Telegram animations и video stickers;
- проверка прав администратора перед `/ban`;
- автоматическое удаление заблокированных сообщений;
- настройка лимитов для animation/video sticker обработки.

## Ограничения бота

Вне зоны ответственности:

- Telegram видео и документы;
- видеоматериалы со значительными изменениями: кадрированием (crop), оверлеями, водяными знаками, изменением скорости или агрессивным монтажом.

## Архитектура

Текущий runtime `v1.2.1` использует Docker Compose, Go Telegram bot, отдельный
Python/FastAPI AI matcher, PostgreSQL как основное хранилище и RabbitMQ как
канал сигналов для обновления производных индексов. Полная архитектура
масштабирования зафиксирована в [docs/README.md](docs/README.md).

PostgreSQL является источником истины для ban-записей, артефактов
сопоставителей, ИИ-векторов, исходящего журнала Watermill и контрольных точек.
Локальные exact/imagehash/videolike/FAISS индексы являются производными
проекциями и могут быть восстановлены из PostgreSQL.

```text
Telegram group
  -> Telegram Bot API
  -> Go bot
      -> moderation service
          -> exact matcher: Telegram file_unique_id
          -> image_hash matcher: perceptual hash for static media
          -> video_like matcher: FFmpeg frames + frame fingerprints
          -> ai_vector matcher: HTTP client
          -> PostgreSQL primary
              -> media_ban and matcher artifacts
              -> Watermill outbox and index checkpoints
          -> RabbitMQ publisher/consumer
              -> Python FastAPI AI vector service
                  -> PostgreSQL primary
                  -> Pillow decoder
                  -> Transformers image embedding model
                  -> Faiss HNSW derived index
      -> Telegram delete message action
```

Основной процесс написан на Go и отвечает за Telegram-интеграцию, проверку прав
администратора, пайплайн модерации и удаление сообщений. AI-vector matching
вынесен в отдельный долгоживущий Python/FastAPI сервис, чтобы модель загружалась
один раз, а Go-бот обращался к ней по HTTP. PostgreSQL хранит активные
ban-записи и векторы, FAISS HNSW используется как производный индекс для
быстрого поиска.

SQLite на каждую реплику не входит в рабочую схему: все операции записи
приложения идут в основной узел PostgreSQL, резервные реплики PostgreSQL
остаются только для чтения до переключения основного узла, а любой сомнительный
локальный индекс пересобирается из PostgreSQL.

## Структура проекта

```text
services/
  bot/        Go Telegram bot service
  aimatcher/  Python FastAPI AI matching service
infra/
  config/     application configs
  docker/     service Dockerfiles
  helm/       Kubernetes Helm charts and platform values
  scripts/    Kubernetes/Vault install scripts
  bin/        local build artifacts
  systemd/    systemd unit files
```

## Требования

- Go `1.25.4` или совместимая версия;
- Telegram bot token от `@BotFather`;
- FFmpeg для video-like matching и AI vector matching кадров;
- Python service dependencies из `services/aimatcher/ai_vector_service/requirements.txt`, если включен
  `matching.ai_vector.enabled`;
- доступ к PostgreSQL;
- доступ к RabbitMQ, если включены публикация исходящих событий или потребители
  индексных событий;
- бот добавлен в группу администратором;
- у бота есть право удалять сообщения.

Проверить FFmpeg:

```bash
ffmpeg -version
```

## Настройка в Telegram

### 1. Создать бота

Создайте бота через `@BotFather` и получите token. Token указывается в
`infra/config/config.yaml`:

```yaml
telegram:
  token: "your-telegram-bot-token"
```

### 2. Включить inline mode

В `@BotFather` включите inline mode для бота.

Ожидаемое состояние: inline mode включен, placeholder может быть `Search...`.

![Inline mode](docs/assets/botfather-inline-mode.png)

### 3. Добавить команды

В `@BotFather` настройте команды бота.

Ожидаемый список команд:

![Bot commands](docs/assets/botfather-commands.png)

Команды в runtime:

- `/hello` - проверочная команда, отвечает `Hello, World!`;
- `/ban` - блокирует медиа из сообщения, на которое сделан reply.

### 4. Добавить бота в группу

Добавьте бота в группу как администратора.

Минимально необходимое право:

- `Delete messages`.

У бота должно быть право удалять сообщения. Остальные права можно не выдавать,
если они не нужны для вашей группы.

![Admin rights](docs/assets/telegram-admin-rights.png)

## Конфигурация

Создайте рабочий config:

```bash
cp infra/config/config.example.yaml infra/config/config.yaml
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

database:
  dsn: "postgres://bastyle:bastyle@bastyle-postgresql:5432/bastyle?sslmode=disable"
  max_conns: 10
  min_conns: 1
  max_conn_lifetime: 1h
  max_conn_idle_time: 15m
  health_check_period: 30s
  connect_timeout: 5s
  statement_timeout: 10s
  migration:
    enabled: true

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
    threshold: 12
    buffer: 500

  video_like:
    threshold: 12
    buffer: 500
    min_matched_frames: 2
    min_matched_ratio: 0.4

  ai_vector:
    enabled: false
    model_name: "nomic-ai/nomic-embed-vision-v1.5"
    model_revision: "e3a725bce72db07ca4adb1d83da08903f3ee02f8"
    device: "cpu"
    index_path: "faiss-image.index"
    max_files: 10
    threshold: 0.92
    top_k: 5
    min_matched_frames: 2
    min_matched_ratio: 0.4
    request_timeout: 10s
    service:
      host: "bastyle-aimatcher"
      port: 8080
    hnsw:
      m: 32
      ef_construction: 80
      ef_search: 64
```

### Основные параметры

`telegram.token` - token Telegram-бота.
`telegram.http_client.enabled` - включает инициализацию Telegram Bot API через
настроенный HTTP client.
`telegram.http_client.proxy_url` - proxy URL для HTTP client. Если значение
пустое, используется proxy из окружения.

`workers` - количество worker'ов для обработки сообщений.
`jobs_buffer` - размер очереди сообщений.
`health.enabled` - включает HTTP health endpoint.
`health.host` и `health.port` - host/port для health endpoint.
`database.dsn` - PostgreSQL DSN для Go-бота и AI-сервиса.
`database.max_conns` и `min_conns` - лимиты пула соединений PostgreSQL.
`database.max_conn_lifetime`, `max_conn_idle_time`, `health_check_period`,
`connect_timeout` и `statement_timeout` - таймауты PostgreSQL client/pool.
`database.migration.enabled` - включает запуск миграций там, где это явно
поддержано инфраструктурой.

`matching.exact.buffer` - стартовый размер черного списка по `file_unique_id` в памяти приложения.
`matching.image_hash.threshold` - максимальная Hamming distance для похожих изображений.
`media_config.*` - общие лимиты и FFmpeg-настройки для кадров, которые
используют матчеры `video_like` и `ai_vector`.
`matching.video_like.min_matched_frames` и `min_matched_ratio` - правило
долю совпадения кадров для video-like hash матчера.
`matching.ai_vector.enabled` - включает или отключает AI vector матчер.
`matching.ai_vector.index_path` - файл Faiss HNSW индекса.
`matching.ai_vector.threshold` - минимальный cosine similarity score.
`matching.ai_vector.top_k` - сколько ближайших векторов запрашивать у AI service.
`matching.ai_vector.min_matched_frames` и `min_matched_ratio` - правило
долю совпадения кадров для AI-vector матчера.
`matching.ai_vector.service.host` и `matching.ai_vector.service.port` - host/port
AI vector service.
`matching.ai_vector.hnsw.m`, `ef_construction`, `ef_search` - параметры HNSW
индекса Faiss.

## Запуск

Локально:

```bash
go -C services/bot run ./cmd/main.go -config ../../infra/config/config.yaml
```

После успешного запуска бот пишет в лог:

```text
Авторизован как <bot_username>
```

## Docker

Сборка:

```bash
docker build -f infra/docker/Dockerfile.bot -t bastyle-blacklist:1.2.0 .
```

Запуск:

```bash
docker run --rm \
  --memory 512m \
  --memory-swap 512m \
  -v "$PWD/infra/config/config.yaml:/etc/bastyle/config.yaml:ro" \
  -v bastyle-data:/var/lib/bastyle \
  bastyle-blacklist:1.2.0
```

Для Docker укажите PostgreSQL DSN в config. Локальный volume `/var/lib/bastyle`
остается для производных runtime-данных, например FAISS index:

```yaml
database:
  dsn: "postgres://bastyle:bastyle@bastyle-postgresql:5432/bastyle?sslmode=disable"

matching:
  ai_vector:
    index_path: "/var/lib/bastyle/faiss-image.index"
```

## Docker Compose

Запуск через Compose:

```bash
cp infra/config/config.example.yaml infra/config/config.yaml
make up
```

Остановка:

```bash
make down
```

Compose монтирует config в `/etc/bastyle/config.yaml` для Go-бота, в
`/app/infra/config/config.yaml` для AI-сервиса и общий volume `/var/lib/bastyle` для
Faiss index и runtime-данных. Контейнер Go-бота ограничен `512m`
памяти, контейнер `bastyle-aimatcher` - `2g`.

`bastyle-aimatcher` - внутренний сервис. Его HTTP endpoint должен быть доступен
только Go-боту внутри приватной сети Compose/Kubernetes и не должен
публиковаться наружу через public ports, ingress или gateway.

## Kubernetes

Используется один Helm values-файл: `infra/helm/bastyle/values.yaml`.

Перед установкой проверьте значения:

- `bot.image.repository` и `bot.image.tag`;
- `aimatcher.image.repository` и `aimatcher.image.tag`;
- `vault.*`;
- `bot.runtimeSecrets.*`;
- `postgresql.*`;
- `rabbitmq.*`.

```bash
make install-platform
```

Эта команда устанавливает HashiCorp Vault и Vault Secrets Operator. Helm chart
приложения использует ресурсы оператора `VaultAuth`, `VaultDynamicSecret` и
`VaultStaticSecret`, поэтому устанавливать приложение до появления CRD нельзя.
Если запустить `helm upgrade --install bastyle infra/helm/bastyle/` напрямую,
Helm завершится ошибкой вида `no matches for kind "VaultAuth" in version
"secrets.hashicorp.com/v1beta1"`.

Проверьте, что CRD оператора зарегистрированы:

```bash
kubectl get crd vaultauths.secrets.hashicorp.com \
  vaultdynamicsecrets.secrets.hashicorp.com \
  vaultstaticsecrets.secrets.hashicorp.com
```

Проверьте, что Vault Secrets Operator запущен:

```bash
kubectl rollout status deployment/vault-secrets-operator-controller-manager -n vault
```

Инициализируйте и распечатайте Vault:

```bash
kubectl exec -n vault vault-0 -- vault operator init
kubectl exec -n vault vault-0 -- vault operator unseal
```

Для HA Vault выполните `unseal` для каждого Vault pod.

Откройте доступ к Vault API:

```bash
kubectl port-forward -n vault svc/vault 8200:8200
```

В другом терминале задайте переменные:

```bash
export VAULT_ADDR="http://127.0.0.1:8200"
export VAULT_TOKEN="<vault-token>"
export TELEGRAM_TOKEN="<telegram-token>"
export POSTGRES_ADMIN_PASSWORD="<postgres-admin-password>"
export RABBITMQ_ADMIN_PASSWORD="<rabbitmq-admin-password>"
```

Настройте Vault и установите приложение:

```bash
make bootstrap-vault
make install-app
```

Проверка:

```bash
kubectl get vaultauth,vaultdynamicsecret,vaultstaticsecret -n bastyle
kubectl get secret bastyle-postgres-runtime bastyle-rabbitmq-runtime bastyle-telegram-runtime -n bastyle
kubectl rollout status deployment/bastyle-bot -n bastyle
kubectl port-forward -n bastyle deploy/bastyle-bot 18081:8081
curl -fsS http://127.0.0.1:18081/health
```

Секреты в Kubernetes создает Vault Secrets Operator. Не создавайте runtime
Secret вручную.

## Systemd

Сборка и установка бинарника:

```bash
make build
sudo install -o root -g root -m 0755 infra/bin/bastyle-blacklist /usr/local/bin/bastyle-blacklist
```

Подготовка пользователя, config и директории данных:

```bash
id -u bastyle_bot >/dev/null 2>&1 || sudo useradd --system --home-dir /var/lib/bastyle --create-home --shell /usr/sbin/nologin bastyle_bot
sudo install -d -o bastyle_bot -g bastyle_bot -m 0750 /var/lib/bastyle
sudo install -d -o root -g root -m 0755 /etc/bastyle
sudo install -o root -g root -m 0640 infra/config/config.example.yaml /etc/bastyle/config.yaml
```

В `/etc/bastyle/config.yaml` укажите Telegram token, PostgreSQL DSN и путь к
производному FAISS index:

```yaml
database:
  dsn: "postgres://bastyle:bastyle@postgres.example:5432/bastyle?sslmode=require"

matching:
  ai_vector:
    index_path: "/var/lib/bastyle/faiss-image.index"
```

Установка unit-файла:

```bash
sudo install -o root -g root -m 0644 infra/systemd/bastyle-blacklist.service /etc/systemd/system/bastyle-blacklist.service
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

## Что изменилось в `v1.2.1`

- (fix) exact matcher сохраняет состояние и восстановливает его после перезагрузки приложения
