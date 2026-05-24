1#!/bin/sh
set -eu

compose_file="infra/postgresql/replication/docker-compose.yaml"
env_file="infra/postgresql/replication/.env"

set -a
. "$env_file"
set +a

docker compose --env-file "$env_file" -f "$compose_file" stop postgres-primary
docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-standby \
  su-exec postgres pg_ctl -D /var/lib/postgresql/data promote
sleep 2
docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-standby \
  psql -U "$POSTGRES_REPLICATION_USER" -d postgres -v ON_ERROR_STOP=1 -c "
    SELECT pg_is_in_recovery() AS is_standby_after_promote;
  "

docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-standby \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "
    CREATE TABLE IF NOT EXISTS ha_failover_check (
      id BIGSERIAL PRIMARY KEY,
      marker TEXT NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );

    INSERT INTO ha_failover_check(marker)
    VALUES ('promoted-standby-write-' || extract(epoch from now())::text);

    SELECT count(*) AS promoted_primary_writes
    FROM ha_failover_check;
  "
