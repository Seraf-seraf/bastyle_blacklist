# PostgreSQL Replication

Локальный контур ручной PostgreSQL physical streaming replication проверяет:

- primary принимает записи;
- standby получает WAL в async-режиме;
- данные с primary появляются на standby;
- WAL archive включен;
- standby можно вручную promoted и после promote он принимает записи.

Это не Kubernetes-манифест.

Правила для приложения:

- все write-операции идут только в primary;
- moderation-critical reads остаются на primary;
- standby используется для read-only admin/reporting-сценариев, backup,
  диагностики и failover;
- replication mode для MVP - async;
- multi-primary не используется.

## Файлы

```text
infra/postgresql/replication/
  docker-compose.yaml
  .env.example
  primary/
    postgresql.conf
    pg_hba.conf
    init-replication-user.sh
  standby/
    standby-entrypoint.sh
  scripts/
    check-replication.sh
    create-check-row.sh
    promote-standby.sh
```

`docker-compose.yaml` описывает только контейнеры и volume mounts. PostgreSQL
настройки лежат в `primary/postgresql.conf` и `primary/pg_hba.conf`.

Секреты и порты задаются в локальном `.env`. Файл `.env` не коммитится; для
старта используется `.env.example`.

## Команды

Подготовить окружение:

```bash
test -f infra/postgresql/replication/.env || cp infra/postgresql/replication/.env.example infra/postgresql/replication/.env
```

Поднять PostgreSQL primary/standby:

```bash
docker compose \
  --env-file infra/postgresql/replication/.env \
  -f infra/postgresql/replication/docker-compose.yaml \
  --project-directory infra/postgresql/replication \
  up -d
```

Проверить replication:

```bash
infra/postgresql/replication/scripts/check-replication.sh
infra/postgresql/replication/scripts/create-check-row.sh
```

Проверка выполняет:

- чтение `pg_stat_replication` на primary;
- проверку `pg_is_in_recovery()` на standby;
- `pg_switch_wal()` и проверку WAL archive;
- запись контрольной строки в primary;
- чтение контрольной строки со standby.

Проверить ручной failover:

```bash
infra/postgresql/replication/scripts/promote-standby.sh
```

Проверка останавливает primary, выполняет `pg_ctl promote` на standby и
проверяет, что promoted standby больше не в recovery и принимает запись.

Остановить контур и удалить volumes:

```bash
docker compose \
  --env-file infra/postgresql/replication/.env \
  -f infra/postgresql/replication/docker-compose.yaml \
  --project-directory infra/postgresql/replication \
  down -v --remove-orphans
```

## Результат

Проверка replication должна показать:

- `state = streaming`;
- `sync_state = async`;
- `lag_bytes` не растет постоянно;
- `pg_is_in_recovery = true` на standby;
- количество файлов в WAL archive больше нуля после `pg_switch_wal()`;
- контрольная строка доступна на standby.

Проверка failover должна показать:

- `pg_is_in_recovery = false` после promote;
- запись в promoted standby проходит успешно.

После failover-теста старый primary нельзя возвращать в кластер как primary.
Для нового цикла проверки нужно удалить volumes и поднять контур заново.
