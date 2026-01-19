# shellcheck shell=bash

# trim_whitespace
# Purpose: Strip leading and trailing whitespace from a string.
# Args:
#   $1: Input string.
# Output: Prints the trimmed string to stdout.
# Returns: 0 always.
trim_whitespace() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "${value}"
}

# format_duration
# Purpose: Convert seconds into a human-readable duration string.
# Args:
#   $1: Duration in seconds (integer).
# Output: Prints formatted duration (e.g., 1h02m03s).
# Returns: 0 always.
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

# join_by
# Purpose: Join arguments with a delimiter.
# Args:
#   $1: Delimiter string.
#   $2+: Items to join.
# Output: Prints the joined string to stdout.
# Returns: 0 always.
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

# escape_log_value
# Purpose: Escape backslashes and quotes for safe logging.
# Args:
#   $1: Input string.
# Output: Prints the escaped string to stdout.
# Returns: 0 always.
escape_log_value() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "${value}"
}

# sha256_file
# Purpose: Compute a SHA-256 hash for a file using available tooling.
# Args:
#   $1: File path (string).
# Output: Prints the hash to stdout.
# Returns: 0 on success; 1 if no supported tool is available.
sha256_file() {
  local path="$1"
  local sha=""
  if command -v shasum > /dev/null 2>&1; then
    if sha="$(shasum -a 256 "${path}" 2> /dev/null)"; then
      sha="$(printf '%s' "${sha}" | awk '{print $1}')"
      if [[ -n "${sha}" ]]; then
        printf '%s\n' "${sha}"
        return 0
      fi
    fi
  fi
  if command -v sha256sum > /dev/null 2>&1; then
    if sha="$(sha256sum "${path}" 2> /dev/null)"; then
      sha="$(printf '%s' "${sha}" | awk '{print $1}')"
      if [[ -n "${sha}" ]]; then
        printf '%s\n' "${sha}"
        return 0
      fi
    fi
  fi
  if command -v openssl > /dev/null 2>&1; then
    if sha="$(openssl dgst -sha256 "${path}" 2> /dev/null)"; then
      sha="$(printf '%s' "${sha}" | awk '{print $2}')"
      if [[ -n "${sha}" ]]; then
        printf '%s\n' "${sha}"
        return 0
      fi
    fi
  fi
  return 1
}

# file_mtime_epoch
# Purpose: Read file modification time in epoch seconds.
# Args:
#   $1: File path (string).
# Output: Prints epoch seconds to stdout.
# Returns: 0 on success; 1 on failure.
file_mtime_epoch() {
  local path="$1"
  if stat -f %m "${path}" > /dev/null 2>&1; then
    stat -f %m "${path}" 2> /dev/null || return 1
    return 0
  fi
  stat -c %Y "${path}" 2> /dev/null || return 1
}

# normalize_tmp_path
# Purpose: Normalize /tmp paths to /private/tmp when applicable.
# Args:
#   $1: Path string.
# Output: Prints normalized path to stdout.
# Returns: 0 always.
normalize_tmp_path() {
  local path="$1"
  if [[ -d "/private/tmp" && "${path}" == /tmp/* ]]; then
    printf '%s\n' "/private${path}"
    return 0
  fi
  printf '%s\n' "${path}"
}
