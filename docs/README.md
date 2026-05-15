# Архитектура Bastyle Blacklist

Документ фиксирует целевую архитектуру масштабирования после перехода с
локального SQLite на PostgreSQL.

## Решение

PostgreSQL primary становится единым source of truth для blacklist, matcher
artifacts, AI-vector данных, outbox и состояния индексов. Локальные индексы в
репликах остаются только производными cache/projection и могут быть полностью
пересобраны из PostgreSQL.

Per-replica SQLite не входит в целевую схему. SQLite-файлы текущей версии
считаются временным runtime-хранилищем до миграции данных и кода на PostgreSQL.

## Целевая схема

```text
Telegram / Actor
  -> API Gateway / Nginx
  -> Bastyle bot replica N
      -> go-bot
          -> PostgreSQL primary
              -> media_ban
              -> matcher artifacts
              -> ai_vector data
              -> outbox_events
              -> index_checkpoints
          -> local exact/imagehash/videolike indexes
          -> RabbitMQ publisher/consumer
      -> ai-matcher FastAPI
          -> PostgreSQL primary
          -> local FAISS HNSW index

PostgreSQL primary
  -> PostgreSQL standby replicas

RabbitMQ
  -> index update signals for all bot/ai-matcher replicas
```

## Source of Truth и производные данные

Source of truth:

- PostgreSQL primary.

Производные данные:

- локальные in-memory индексы Go: exact, imagehash, videolike;
- локальный FAISS HNSW index в ai-matcher;
- сообщения RabbitMQ;
- PostgreSQL standby replicas.

RabbitMQ используется как быстрый канал событий для обновления локальных
индексов, async side effects, audit и будущего unban workflow. Долгосрочное
хранение blacklist и событий остается в PostgreSQL.

## PostgreSQL Replication

PostgreSQL replication в этой архитектуре не является multi-master.

- Все write-операции приложения идут в PostgreSQL primary.
- Standby replicas остаются read-only до failover.
- Standby можно использовать для read-only запросов, бэкапов, аналитики и
  snapshot.
- Moderation-critical reads на первом этапе идут в primary, чтобы не зависеть
  от replication lag.
- После failover приложение переключает write-трафик на новый primary.

## Поток /ban

1. Actor отправляет `/ban`.
2. API Gateway направляет update на одну из реплик Bastyle bot.
3. Реплика проверяет права администратора и извлекает признаки медиа.
4. Реплика открывает транзакцию в PostgreSQL primary.
5. В одной транзакции сохраняются `media_ban`, matcher artifacts, AI-vector
   данные и запись `outbox_events` с событием `media.ban.created.v1`.
6. После commit outbox publisher публикует событие в RabbitMQ.
7. Все реплики получают сигнал и догоняют свои локальные индексы по
   `outbox_events` из PostgreSQL.
8. Если реплика пропустила событие или индекс стал сомнительным, индекс
   помечается stale и пересобирается из PostgreSQL.

## Поток проверки медиа

1. API Gateway направляет update на одну из реплик.
2. Реплика проверяет медиа по локальным индексам.
3. Если индекс готов и не отстал, совпадение обрабатывается без запроса в
   PostgreSQL.
4. Если локальный индекс сомнителен, реплика пересобирает его из PostgreSQL.
   На время деградации реплика может читать из PostgreSQL напрямую или стать
   not ready, чтобы gateway не слал на нее трафик.
5. При совпадении бот удаляет сообщение.

## Правило восстановления индексов

Локальный индекс не считается источником истины. Если индекс поврежден, отстал
или его checkpoint нельзя доверять, он пересобирается из PostgreSQL active bans.
После успешного rebuild checkpoint выставляется на актуальный
`max(outbox_events.id)` для использованного snapshot.
