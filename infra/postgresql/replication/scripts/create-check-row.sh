#!/bin/sh
set -eu

compose_file="infra/postgresql/replication/docker-compose.yaml"
env_file="infra/postgresql/replication/.env"

set -a
. "$env_file"
set +a

docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-primary \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "
    CREATE TABLE IF NOT EXISTS ha_replication_check (
      id BIGSERIAL PRIMARY KEY,
      marker TEXT NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );

    INSERT INTO ha_replication_check(marker)
    VALUES ('replication-check-' || extract(epoch from now())::text);
  "

sleep 2

docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-standby \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "
    SELECT count(*) AS replicated_rows, max(marker) AS last_marker
    FROM ha_replication_check;
  "
