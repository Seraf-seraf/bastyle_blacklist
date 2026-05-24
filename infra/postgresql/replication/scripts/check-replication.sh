#!/bin/sh
set -eu

compose_file="infra/postgresql/replication/docker-compose.yaml"
env_file="infra/postgresql/replication/.env"

set -a
. "$env_file"
set +a

docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-primary \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "
    SELECT
      application_name,
      state,
      sync_state,
      pg_wal_lsn_diff(sent_lsn, replay_lsn) AS lag_bytes
    FROM pg_stat_replication;
  "

docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-standby \
  psql -U "$POSTGRES_REPLICATION_USER" -d postgres -v ON_ERROR_STOP=1 -c "
    SELECT
      pg_is_in_recovery() AS is_standby,
      pg_last_wal_receive_lsn(),
      pg_last_wal_replay_lsn(),
      now() - pg_last_xact_replay_timestamp() AS replay_delay;
  "

docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-primary \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "SELECT pg_switch_wal();"

sleep 3

docker compose --env-file "$env_file" -f "$compose_file" exec -T postgres-primary \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "
    SELECT count(*) AS archived_wal_files
    FROM pg_ls_dir('/var/lib/postgresql/data/wal_archive');
  "
