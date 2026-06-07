# Bastyle Blacklist

Версия: `v2.1.0`

`Bastyle Blacklist` - Telegram-бот для автоматической модерации медиа в
групповых чатах. Администратор один раз блокирует нежелательный контент, после
чего бот автоматически удаляет повторные отправки этого же или похожего медиа.

## Возможности

- exact-сопоставление по Telegram `file_unique_id`;
- perceptual hash matching для фото и статичных стикеров;
- video-like matching для GIF-анимаций и video stickers;
- AI-vector matching для визуально похожих изображений и анимаций;
- проверка прав администратора перед выполнением `/ban`;
- автоматическое удаление заблокированных сообщений;
- восстановление производных индексов из PostgreSQL.

Основной сценарий:

1. Администратор отвечает на нежелательное медиа командой `/ban`.
2. Бот проверяет права администратора.
3. Fingerprint контента сохраняется в blacklist.
4. Исходное сообщение удаляется.
5. Последующие совпадающие сообщения удаляются автоматически.

## Структура

```text
charts/blacklist/ Helm chart приложения
config/           пример конфигурации
docker/           Dockerfiles bot, aimatcher и migrations
scripts/          установка приложения и bootstrap его Vault policy
services/         исходный код bot и aimatcher
docs/             документация приложения
```

Проект состоит из Go Telegram-бота и Python/FastAPI сервиса AI matching.
PostgreSQL используется как источник истины, RabbitMQ передает сигналы
обновления индексов, а локальные индексы являются восстанавливаемыми
проекциями данных.

## Проверки

```bash
make test
make ci
make helm-lint
make helm-template
```

Миграции локально запускаются только против явно доступной базы:

```bash
make migrate DB_DSN='postgres://user:password@host:5432/bastyle?sslmode=require'
```

## Образы

```bash
make docker-build-bot
make docker-build-aimatcher
make docker-build-migrations
```

## Kubernetes

Для установки необходим Kubernetes-кластер со следующими компонентами:

- PostgreSQL и RabbitMQ;
- ingress-nginx и cert-manager;
- Vault и Vault Secrets Operator.

После подготовки зависимостей настройте Vault policy и установите Helm chart:

```bash
set -a
source .env
set +a

make bootstrap-vault
make install-app
```

Параметры подключения задаются в `charts/blacklist/values.yaml`:

- `dependencies.postgresql`;
- `dependencies.rabbitmq`;
- `vault`;
- `bot.runtimeSecrets`;
- `ingress`.

По умолчанию используются Kubernetes services `bastyle-postgresql-rw` и
`bastyle-rabbitmq` в namespace релиза.
