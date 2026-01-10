#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_DIR="${ROOT_DIR}/_governator"
DB_DIR="${ROOT_DIR}/.governator"
CONFIG_FILE="${DB_DIR}/config.json"
CONFIG_TEMPLATE="${STATE_DIR}/templates/config.json"
LEGACY_LAST_UPDATE_FILE="${DB_DIR}/last_update_at"

if [[ ! -f "${LEGACY_LAST_UPDATE_FILE}" ]]; then
  exit 0
fi

mkdir -p "${DB_DIR}"

if [[ ! -f "${CONFIG_FILE}" ]]; then
  if [[ -f "${CONFIG_TEMPLATE}" ]]; then
    cp "${CONFIG_TEMPLATE}" "${CONFIG_FILE}"
  else
    jq -S -n '{last_update_at: "never"}' > "${CONFIG_FILE}"
  fi
fi

legacy_value="$(tr -d '[:space:]' < "${LEGACY_LAST_UPDATE_FILE}")"
if [[ -z "${legacy_value}" ]]; then
  legacy_value="never"
fi

tmp_file="$(mktemp "${DB_DIR}/config.XXXXXX")"
if jq -e . "${CONFIG_FILE}" > /dev/null 2>&1; then
  jq -S --arg value "${legacy_value}" \
    'setpath(["last_update_at"]; $value)' \
    "${CONFIG_FILE}" > "${tmp_file}"
else
  jq -S -n --arg value "${legacy_value}" \
    'setpath(["last_update_at"]; $value)' \
    > "${tmp_file}"
fi
mv "${tmp_file}" "${CONFIG_FILE}"
rm -f "${LEGACY_LAST_UPDATE_FILE}"
