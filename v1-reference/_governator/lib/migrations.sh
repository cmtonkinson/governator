# shellcheck shell=bash

# ensure_migrations_state_file
# Purpose: Ensure the migrations state file exists with the default schema.
# Args: None.
# Output: Writes the state file when missing.
# Returns: 0 on completion.
ensure_migrations_state_file() {
  if [[ -f "${MIGRATIONS_STATE_FILE}" ]]; then
    return 0
  fi
  mkdir -p "$(dirname "${MIGRATIONS_STATE_FILE}")"
  cat > "${MIGRATIONS_STATE_FILE}" <<'EOF'
{
  "version": 1,
  "applied": []
}
EOF
}

# migration_is_applied
# Purpose: Check whether a migration ID has already been applied.
# Args:
#   $1: Migration ID (string).
# Output: None.
# Returns: 0 if applied; 1 otherwise.
migration_is_applied() {
  local migration_id="$1"
  jq -e --arg id "${migration_id}" '.applied[]? | select(.id == $id)' "${MIGRATIONS_STATE_FILE}" > /dev/null 2>&1
}

# record_migration_applied
# Purpose: Record a migration ID in the state file with a timestamp.
# Args:
#   $1: Migration ID (string).
# Output: Updates the migrations state file.
# Returns: 0 on success; exits on failure.
record_migration_applied() {
  local migration_id="$1"
  local tmp_file
  tmp_file="$(mktemp)"
  if ! jq --arg id "${migration_id}" \
    --arg applied_at "$(timestamp_utc_seconds)" \
    '.applied += [{"id": $id, "applied_at": $applied_at}]' \
    "${MIGRATIONS_STATE_FILE}" > "${tmp_file}"; then
    rm -f "${tmp_file}"
    log_error "Failed to update migrations state file."
    exit 1
  fi
  mv "${tmp_file}" "${MIGRATIONS_STATE_FILE}"
}

# list_migration_files
# Purpose: List migration script files in deterministic order.
# Args: None.
# Output: Prints migration file paths, one per line.
# Returns: 0 always.
list_migration_files() {
  if [[ ! -d "${MIGRATIONS_DIR}" ]]; then
    return 0
  fi
  find "${MIGRATIONS_DIR}" -maxdepth 1 -type f -print 2> /dev/null | sort |
    while IFS= read -r migration_path; do
      local base
      base="$(basename "${migration_path}")"
      if [[ "${base}" == .* ]]; then
        continue
      fi
      printf '%s\n' "${migration_path}"
    done
}

# run_governator_migrations
# Purpose: Execute unapplied migrations and update the state file.
# Args: None.
# Output: Logs migration execution progress.
# Returns: 0 on success; exits on failure.
run_governator_migrations() {
  if [[ ! -d "${MIGRATIONS_DIR}" ]]; then
    return 0
  fi
  ensure_migrations_state_file

  local migration_path
  while IFS= read -r migration_path; do
    local migration_id
    migration_id="$(basename "${migration_path}")"
    if migration_is_applied "${migration_id}"; then
      continue
    fi
    log_info "Running migration ${migration_id}"
    if ! (cd "${ROOT_DIR}" && bash "${migration_path}"); then
      log_error "Migration failed: ${migration_id}"
      exit 1
    fi
    record_migration_applied "${migration_id}"
  done < <(list_migration_files)
}
