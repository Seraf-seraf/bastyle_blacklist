# PostgreSQL Replication

Документ описывает локальный стенд ручной PostgreSQL physical streaming
replication для проверки этапа масштабирования.

Стенд нужен разработчику, чтобы быстро проверить:

- primary принимает записи;
- standby получает WAL в async-режиме;
- данные с primary появляются на standby;
- WAL archive включен;
- standby можно вручную promoted и после promote он принимает записи.

Это не production-манифест. Production-настройки секретов, backup storage,
DNS/endpoint switching и мониторинг должны оформляться отдельно для конкретной
инфраструктуры.

## Архитектурное Решение

Выбран вариант: ручная PostgreSQL physical streaming replication.

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

Поднять стенд:

```bash
make pgrp-up
```

Команда создаст `infra/postgresql/replication/.env` из `.env.example`, если
локального файла еще нет.

Проверить replication:

```bash
make pgrp-check
```

Проверка выполняет:

- чтение `pg_stat_replication` на primary;
- проверку `pg_is_in_recovery()` на standby;
- `pg_switch_wal()` и проверку WAL archive;
- запись контрольной строки в primary;
- чтение контрольной строки со standby.

Проверить ручной failover:

```bash
make pgrp-failover
```

Проверка останавливает primary, выполняет `pg_ctl promote` на standby и
проверяет, что promoted standby больше не в recovery и принимает запись.

Остановить стенд и удалить volumes:

```bash
make pgrp-down
```

## Ожидаемый Результат

`make pgrp-check` должен показать:

- `state = streaming`;
- `sync_state = async`;
- `lag_bytes` не растет постоянно;
- `pg_is_in_recovery = true` на standby;
- количество файлов в WAL archive больше нуля после `pg_switch_wal()`;
- контрольная строка доступна на standby.

`make pgrp-failover` должен показать:

- `pg_is_in_recovery = false` после promote;
- запись в promoted standby проходит успешно.

После failover-теста старый primary нельзя возвращать в кластер как primary.
Для нового цикла проверки нужно выполнить `make pgrp-down`, затем
`make pgrp-up`.
