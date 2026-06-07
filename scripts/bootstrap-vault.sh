#!/usr/bin/env bash
set -euo pipefail

: "${VAULT_ADDR:?VAULT_ADDR is required}"
: "${VAULT_TOKEN:?VAULT_TOKEN is required}"
: "${TELEGRAM_TOKEN:?TELEGRAM_TOKEN is required}"
: "${POSTGRES_ADMIN_PASSWORD:?POSTGRES_ADMIN_PASSWORD is required}"
: "${RABBITMQ_ADMIN_PASSWORD:?RABBITMQ_ADMIN_PASSWORD is required}"

BASTYLE_NAMESPACE="${BASTYLE_NAMESPACE:-bastyle}"
BASTYLE_SERVICE_ACCOUNT="${BASTYLE_SERVICE_ACCOUNT:-blacklist}"
POSTGRES_HOST="${POSTGRES_HOST:-bastyle-postgresql-rw}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_DATABASE="${POSTGRES_DATABASE:-bastyle}"
POSTGRES_ADMIN_USER="${POSTGRES_ADMIN_USER:-vault_admin}"
RABBITMQ_HTTP_URL="${RABBITMQ_HTTP_URL:-http://bastyle-rabbitmq:15672}"
RABBITMQ_ADMIN_USER="${RABBITMQ_ADMIN_USER:-vault_admin}"

vault secrets enable -path=kv kv-v2 || true
vault kv put kv/blacklist/telegram token="${TELEGRAM_TOKEN}"

vault secrets enable database || true

vault write database/config/bastyle-postgres \
  plugin_name=postgresql-database-plugin \
  allowed_roles=blacklist \
  connection_url="postgresql://{{username}}:{{password}}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DATABASE}?sslmode=disable" \
  username="${POSTGRES_ADMIN_USER}" \
  password="${POSTGRES_ADMIN_PASSWORD}"

vault write database/roles/blacklist \
  db_name=bastyle-postgres \
  creation_statements="
    CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}';
    GRANT CONNECT ON DATABASE ${POSTGRES_DATABASE} TO \"{{name}}\";
    GRANT USAGE ON SCHEMA public TO \"{{name}}\";
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO \"{{name}}\";
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO \"{{name}}\";
  " \
  default_ttl=6h \
  max_ttl=24h

vault secrets enable rabbitmq || true

vault write rabbitmq/config/connection \
  connection_uri="${RABBITMQ_HTTP_URL}" \
  username="${RABBITMQ_ADMIN_USER}" \
  password="${RABBITMQ_ADMIN_PASSWORD}"

vault write rabbitmq/roles/blacklist \
  vhosts='{"/":{"configure":"","write":".*","read":".*"}}'

vault auth enable kubernetes || true

vault write auth/kubernetes/config \
  kubernetes_host="https://kubernetes.default.svc"

vault policy write blacklist - <<EOPOLICY
path "kv/data/blacklist/telegram" {
  capabilities = ["read"]
}

path "database/creds/blacklist" {
  capabilities = ["read"]
}

path "rabbitmq/creds/blacklist" {
  capabilities = ["read"]
}
EOPOLICY

vault write auth/kubernetes/role/blacklist \
  bound_service_account_names="${BASTYLE_SERVICE_ACCOUNT}" \
  bound_service_account_namespaces="${BASTYLE_NAMESPACE}" \
  policies=blacklist \
  ttl=24h
