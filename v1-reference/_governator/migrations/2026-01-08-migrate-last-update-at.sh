#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_DIR="${ROOT_DIR}/_governator"
LOCAL_STATE_DIR="${STATE_DIR}/_local_state"
DURABLE_STATE_DIR="${STATE_DIR}/_durable_state"
CONFIG_FILE="${DURABLE_STATE_DIR}/config.json"
CONFIG_TEMPLATE="${STATE_DIR}/templates/config.json"
LAST_UPDATE_FILE="${LOCAL_STATE_DIR}/last_update_at"

if [[ ! -f "${LAST_UPDATE_FILE}" ]]; then
  exit 0
fi

mkdir -p "${LOCAL_STATE_DIR}"
mkdir -p "${DURABLE_STATE_DIR}"

if [[ ! -f "${CONFIG_FILE}" ]]; then
  if [[ -f "${CONFIG_TEMPLATE}" ]]; then
    cp "${CONFIG_TEMPLATE}" "${CONFIG_FILE}"
  else
    jq -S -n '{last_update_at: "never"}' > "${CONFIG_FILE}"
  fi
fi

last_update_value="$(tr -d '[:space:]' < "${LAST_UPDATE_FILE}")"
if [[ -z "${last_update_value}" ]]; then
  last_update_value="never"
fi

tmp_file="$(mktemp "${DURABLE_STATE_DIR}/config.XXXXXX")"
if jq -e . "${CONFIG_FILE}" > /dev/null 2>&1; then
  jq -S --arg value "${last_update_value}" \
    'setpath(["last_update_at"]; $value)' \
    "${CONFIG_FILE}" > "${tmp_file}"
else
  jq -S -n --arg value "${last_update_value}" \
    'setpath(["last_update_at"]; $value)' \
    > "${tmp_file}"
fi
mv "${tmp_file}" "${CONFIG_FILE}"
rm -f "${LAST_UPDATE_FILE}"
