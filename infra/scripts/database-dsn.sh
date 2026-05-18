#!/usr/bin/env bash
set -euo pipefail

config_path="${1:-infra/config/config.yaml}"

if ! command -v yq >/dev/null 2>&1; then
	echo "required command not found: yq" >&2
	exit 1
fi

if [[ ! -f "$config_path" ]]; then
	echo "config file not found: $config_path" >&2
	exit 1
fi

dsn="$(yq -r '.database.dsn // ""' "$config_path")"

if [[ -z "$dsn" ]]; then
	echo "database.dsn not found in config: $config_path" >&2
	exit 1
fi

printf '%s\n' "$dsn"
