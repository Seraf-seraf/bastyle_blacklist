# Runbook наблюдаемости

Документ описывает первые проверки Blacklist при деградации
БД, исходящего журнала и локальных индексов.

## PostgreSQL недоступен

Симптомы:

- `/health` Go-бота или AI matcher возвращает `503`.
- `bastyle_db_errors_total` или `bastyle_ai_db_errors_total` растет.
- `up{job="postgres-exporter"}` равен `0`.

Проверки:

1. Проверить pod/service PostgreSQL и exporter.
2. Проверить наличие Secret `blacklist-postgres-runtime` и ключа
   `BASTYLE_DATABASE_DSN`. Значение DSN в логи не выводить.
3. Проверить `pg_isready` из внутренней сети кластера.
4. Проверить лимиты соединений: `bastyle_db_pool_acquired`,
   `bastyle_db_pool_idle`, `pg_stat_database_numbackends`.

Действия:

- Если primary недоступен, остановить rollout приложения и выполнить runbook
  failover PostgreSQL.
- Если исчерпан pool, уменьшить нагрузку или поднять лимиты после проверки
  `max_connections`.

## Migration failed

Симптомы:

- Kubernetes Job миграций завершился с ошибкой.
- Новые pod приложения не проходят readiness из-за отсутствующих таблиц/полей.

Проверки:

1. Посмотреть логи migration job.
2. Проверить текущую версию миграции через `make db-status` в dev-окружении или
   через `goose status` с тем же DSN.
3. Проверить, что job подключается к primary PostgreSQL, а не к standby.

Действия:

- Не запускать повторно приложение с новой схемой, пока миграция не исправлена.
- Исправить миграцию отдельным изменением; не менять уже примененную миграцию в
  production без отдельного rollback/fix-forward решения.

## Replication lag растет

Симптомы:

- Метрики postgres exporter по replication lag растут.
- Standby отстает, но primary продолжает принимать writes.

Проверки:

1. Проверить `pg_stat_replication` на primary.
2. Проверить сеть между primary и standby.
3. Проверить disk usage и checkpoint/write pressure.

Действия:

- Не переключать moderation-critical reads на standby.
- Если lag продолжает расти, временно отключить read-only нагрузку со standby.

## Outbox backlog растет

Симптомы:

- `bastyle_outbox_unpublished_total` растет.
- `bastyle_outbox_unpublished_max_age_seconds` растет.

Проверки:

1. Проверить доступность RabbitMQ.
2. Проверить логи outbox publisher.
3. Проверить, что consumer group `outbox-publisher` двигает offsets.

Действия:

- Восстановить RabbitMQ или publisher.
- Не чистить `watermill_outbox_events` вручную: это источник догоняющей
  синхронизации индексов.

## Индекс stale

Симптомы:

- `bastyle_index_stale` или `bastyle_ai_index_stale` равен `1`.
- Readiness возвращает `503`.

Проверки:

1. Проверить `index_checkpoints.stale_reason`.
2. Проверить событие outbox по `event_id` из логов.
3. Проверить доступность PostgreSQL и RabbitMQ.

Действия:

- Перезапустить реплику после устранения причины: bootstrap catch-up должен
  догнать индекс из PostgreSQL.
- Если ошибка связана с поврежденным локальным FAISS-файлом, удалить только
  производный файл индекса на pod/PVC и дать сервису пересобрать его из БД.
