# shellcheck shell=bash

trim_whitespace() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "${value}"
}

format_duration() {
  local seconds="$1"
  if [[ -z "${seconds}" || "${seconds}" -lt 0 ]]; then
    printf 'n/a'
    return
  fi
  local hours=$((seconds / 3600))
  local minutes=$((seconds / 60 % 60))
  local secs=$((seconds % 60))
  if [[ "${hours}" -gt 0 ]]; then
    printf '%dh%02dm%02ds' "${hours}" "${minutes}" "${secs}"
  elif [[ "${minutes}" -gt 0 ]]; then
    printf '%dm%02ds' "${minutes}" "${secs}"
  else
    printf '%02ds' "${secs}"
  fi
}

# Join arguments by a delimiter.
join_by() {
  local delimiter="$1"
  shift
  local first=1
  local item
  for item in "$@"; do
    if [[ "${first}" -eq 1 ]]; then
      printf '%s' "${item}"
      first=0
    else
      printf '%s%s' "${delimiter}" "${item}"
    fi
  done
}

escape_log_value() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "${value}"
}

# Read a file mtime in epoch seconds (BSD/GNU stat compatible).
file_mtime_epoch() {
  local path="$1"
  if stat -f %m "${path}" > /dev/null 2>&1; then
    stat -f %m "${path}" 2> /dev/null || return 1
    return 0
  fi
  stat -c %Y "${path}" 2> /dev/null || return 1
}

# Normalize tmp paths so /tmp and /private/tmp compare consistently.
normalize_tmp_path() {
  local path="$1"
  if [[ -d "/private/tmp" && "${path}" == /tmp/* ]]; then
    printf '%s\n' "/private${path}"
    return 0
  fi
  printf '%s\n' "${path}"
}
