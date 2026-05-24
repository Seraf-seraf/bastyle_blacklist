#!/bin/sh
set -eu

export PGDATA="${PGDATA:-/var/lib/postgresql/data}"
primary_host="${POSTGRES_PRIMARY_HOST:-postgres-primary}"
replication_user="${POSTGRES_REPLICATION_USER:?POSTGRES_REPLICATION_USER is required}"

if [ ! -s "$PGDATA/PG_VERSION" ]; then
  rm -rf "$PGDATA"/*
  until pg_isready -h "$primary_host" -p 5432 -U "$replication_user"; do
    echo "waiting for primary..."
    sleep 2
  done

  pg_basebackup \
    -h "$primary_host" \
    -p 5432 \
    -U "$replication_user" \
    -D "$PGDATA" \
    -Fp \
    -Xs \
    -P \
    -R

  chown -R postgres:postgres "$PGDATA"
  chmod 0700 "$PGDATA"
fi

exec su-exec postgres postgres
