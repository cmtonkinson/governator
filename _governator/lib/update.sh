# shellcheck shell=bash

UPDATE_TMP_ROOT=""

# manifest_sha_for_path
# Purpose: Read a manifest SHA entry for a relative path.
# Args:
#   $1: Relative path under repository root (string).
# Output: Prints the SHA string or empty string.
# Returns: 0 always.
manifest_sha_for_path() {
  local rel_path="$1"
  if [[ ! -f "${MANIFEST_FILE}" ]]; then
    printf '%s\n' ""
    return 0
  fi
  jq -r --arg path "${rel_path}" '.files[$path] // ""' "${MANIFEST_FILE}"
}

# list_manifest_paths
# Purpose: List manifest-eligible files under a base directory.
# Args:
#   $1: Base directory path (string).
# Output: Prints relative paths prefixed with _governator/.
# Returns: 0 always.
list_manifest_paths() {
  local base_dir="$1"
  find "${base_dir}" \
    \( -path "${base_dir}/docs" -o -path "${base_dir}/task-*" \) -prune -o \
    -type f -print 2> /dev/null | sort |
    while IFS= read -r path; do
      local base
      base="$(basename "${path}")"
      if [[ "${base}" == ".keep" ]]; then
        continue
      fi
      printf '%s\n' "_governator/${path#"${base_dir}/"}"
    done
}

# write_manifest
# Purpose: Write a manifest JSON for tracked _governator files.
# Args:
#   $1: Base root path (string).
#   $2: Base directory path (string).
#   $3: Output file path (string).
# Output: Writes JSON manifest file.
# Returns: 0 on success.
write_manifest() {
  local base_root="$1"
  local base_dir="$2"
  local out_file="$3"
  local tmp_file
  tmp_file="$(mktemp)"
  {
    printf '{\n'
    printf '  "version": 1,\n'
    printf '  "files": {\n'
    local first=1
    local rel
    while IFS= read -r rel; do
      local abs="${base_root}/${rel}"
      local sha
      sha="$(sha256_or_fail "${abs}")"
      if [[ "${first}" -eq 1 ]]; then
        first=0
      else
        printf ',\n'
      fi
      printf '    "%s": "%s"' "${rel}" "${sha}"
    done < <(list_manifest_paths "${base_dir}")
    printf '\n  }\n'
    printf '}\n'
  } > "${tmp_file}"
  mv "${tmp_file}" "${out_file}"
}

# ensure_manifest_exists
# Purpose: Ensure a manifest exists, creating one if missing.
# Args: None.
# Output: Logs warning and writes manifest when missing.
# Returns: 0 on completion.
ensure_manifest_exists() {
  if [[ -f "${MANIFEST_FILE}" ]]; then
    return 0
  fi
  log_warn "Manifest missing at ${MANIFEST_FILE}; creating from current files."
  write_manifest "${ROOT_DIR}" "${STATE_DIR}" "${MANIFEST_FILE}"
}

# sha256_or_fail
# Purpose: Compute a SHA-256 hash for a file or exit with an error.
# Args:
#   $1: File path (string).
# Output: Prints the hash to stdout.
# Returns: 0 on success; exits 1 on failure.
sha256_or_fail() {
  local path="$1"
  local sha=""
  if ! sha="$(sha256_file "${path}")"; then
    log_error "Failed to compute sha256 for ${path}"
    exit 1
  fi
  if [[ -z "${sha}" ]]; then
    log_error "Failed to compute sha256 for ${path}"
    exit 1
  fi
  printf '%s\n' "${sha}"
}

# is_code_file
# Purpose: Determine whether a path is treated as code (always updated).
# Args:
#   $1: Relative path (string).
# Output: None.
# Returns: 0 if code file; 1 otherwise.
is_code_file() {
  local rel_path="$1"
  if [[ "${rel_path}" == "_governator/governator.sh" ]]; then
    return 0
  fi
  if [[ "${rel_path}" == _governator/lib/*.sh ]]; then
    return 0
  fi
  return 1
}

# is_prompt_file
# Purpose: Determine whether a path is treated as a prompt/template file.
# Args:
#   $1: Relative path (string).
# Output: None.
# Returns: 0 if prompt file; 1 otherwise.
is_prompt_file() {
  local rel_path="$1"
  case "${rel_path}" in
    _governator/templates/* | _governator/custom-prompts/* | _governator/roles/* | _governator/worker-contract.md)
      return 0
      ;;
  esac
  return 1
}

# confirm_template_action
# Purpose: Decide whether to apply a prompt/template update.
# Args:
#   $1: Relative path (string).
#   $2: Prompt question (string).
# Output: Logs warnings in non-interactive mode.
# Returns: 0 if update should proceed; 1 otherwise.
confirm_template_action() {
  local rel_path="$1"
  local prompt="$2"
  if [[ "${UPDATE_FORCE_REMOTE}" -eq 1 ]]; then
    return 0
  fi
  if [[ "${UPDATE_KEEP_LOCAL}" -eq 1 ]]; then
    return 1
  fi
  if [[ ! -t 0 ]]; then
    log_warn "Non-interactive update; keeping local ${rel_path}."
    return 1
  fi
  local reply
  read -r -p "${prompt} ${rel_path}? [y/N]: " reply
  case "${reply}" in
    y | Y | yes | YES)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

# record_update
# Purpose: Record an update action for reporting.
# Args:
#   $1: Action verb (string: added/updated/removed).
#   $2: Relative path (string).
# Output: Appends to UPDATED_FILES array.
# Returns: 0 always.
record_update() {
  local action="$1"
  local rel_path="$2"
  UPDATED_FILES+=("${action} ${rel_path}")
}

# replace_file_atomically
# Purpose: Replace a file via a temporary copy to avoid partial writes.
# Args:
#   $1: Source file path (string).
#   $2: Destination file path (string).
# Output: None.
# Returns: 0 on completion.
replace_file_atomically() {
  local source_path="$1"
  local dest_path="$2"
  local dest_dir
  dest_dir="$(dirname "${dest_path}")"
  mkdir -p "${dest_dir}"
  local tmp_file
  tmp_file="$(mktemp "${dest_dir}/.governator-update.XXXXXX")"
  cp "${source_path}" "${tmp_file}"
  mv "${tmp_file}" "${dest_path}"
}

# update_code_file
# Purpose: Update a code file unconditionally when upstream differs.
# Args:
#   $1: Relative path (string).
#   $2: Upstream file path (string).
#   $3: Local file path (string).
#   $4: Upstream SHA (string).
#   $5: Variable name to set for update flag (string).
# Output: Copies file and records update when changed.
# Returns: 0 on completion.
update_code_file() {
  local rel_path="$1"
  local upstream_path="$2"
  local local_path="$3"
  local upstream_sha="$4"
  local updated_ref="$5"
  local local_sha=""
  if [[ -f "${local_path}" ]]; then
    local_sha="$(sha256_or_fail "${local_path}")"
  fi
  if [[ "${local_sha}" == "${upstream_sha}" ]]; then
    return 0
  fi
  replace_file_atomically "${upstream_path}" "${local_path}"
  record_update "updated" "${rel_path}"
  eval "${updated_ref}=1"
}

# update_prompt_file
# Purpose: Update a prompt/template file respecting manifest and flags.
# Args:
#   $1: Relative path (string).
#   $2: Upstream file path (string).
#   $3: Local file path (string).
#   $4: Upstream SHA (string).
#   $5: Variable name to set for update flag (string).
# Output: Copies file and records update when changed.
# Returns: 0 on completion.
update_prompt_file() {
  local rel_path="$1"
  local upstream_path="$2"
  local local_path="$3"
  local upstream_sha="$4"
  local updated_ref="$5"

  if [[ ! -f "${local_path}" ]]; then
    replace_file_atomically "${upstream_path}" "${local_path}"
    record_update "added" "${rel_path}"
    eval "${updated_ref}=1"
    return 0
  fi

  local local_sha
  local_sha="$(sha256_or_fail "${local_path}")"
  if [[ "${local_sha}" == "${upstream_sha}" ]]; then
    return 0
  fi

  local manifest_sha
  manifest_sha="$(manifest_sha_for_path "${rel_path}")"

  if [[ -n "${manifest_sha}" && "${manifest_sha}" == "${local_sha}" ]]; then
    replace_file_atomically "${upstream_path}" "${local_path}"
    record_update "updated" "${rel_path}"
    eval "${updated_ref}=1"
    return 0
  fi

  if confirm_template_action "${rel_path}" "Update local template to upstream version"; then
    replace_file_atomically "${upstream_path}" "${local_path}"
    record_update "updated" "${rel_path}"
    eval "${updated_ref}=1"
  fi
}

# remove_code_file
# Purpose: Remove a code file that no longer exists upstream.
# Args:
#   $1: Local file path (string).
#   $2: Variable name to set for update flag (string).
# Output: Deletes file and records update.
# Returns: 0 on completion.
remove_code_file() {
  local local_path="$1"
  local updated_ref="$2"
  if [[ -f "${local_path}" ]]; then
    rm -f "${local_path}"
    record_update "removed" "${local_path#"${ROOT_DIR}/"}"
    eval "${updated_ref}=1"
  fi
}

# remove_prompt_file
# Purpose: Remove a prompt/template file based on manifest and flags.
# Args:
#   $1: Relative path (string).
#   $2: Local file path (string).
#   $3: Variable name to set for update flag (string).
# Output: Deletes file and records update when changed.
# Returns: 0 on completion.
remove_prompt_file() {
  local rel_path="$1"
  local local_path="$2"
  local updated_ref="$3"
  if [[ ! -f "${local_path}" ]]; then
    return 0
  fi

  local local_sha
  local_sha="$(sha256_or_fail "${local_path}")"
  local manifest_sha
  manifest_sha="$(manifest_sha_for_path "${rel_path}")"

  if [[ -n "${manifest_sha}" && "${manifest_sha}" == "${local_sha}" ]]; then
    rm -f "${local_path}"
    record_update "removed" "${rel_path}"
    eval "${updated_ref}=1"
    return 0
  fi

  if confirm_template_action "${rel_path}" "Upstream removed template; delete local file"; then
    rm -f "${local_path}"
    record_update "removed" "${rel_path}"
    eval "${updated_ref}=1"
  fi
}

# update_governator
# Purpose: Update Governator code and prompt templates from upstream tarball.
# Args:
#   --keep-local: Keep local prompt changes without prompting.
#   --force-remote: Overwrite local prompt changes without prompting.
# Output: Prints update summary, runs pending migrations, and records audit/timestamp
#   metadata when updates are applied.
# Returns: 0 on success; exits on fatal errors.
update_governator() {
  UPDATE_KEEP_LOCAL=0
  UPDATE_FORCE_REMOTE=0
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --keep-local)
        UPDATE_KEEP_LOCAL=1
        ;;
      --force-remote)
        UPDATE_FORCE_REMOTE=1
        ;;
      -h | --help)
        printf 'Usage: governator.sh update [--keep-local|--force-remote]\n'
        printf 'Last updated at: %s\n' "$(read_last_update_at)"
        return 0
        ;;
      *)
        log_error "Unknown option for update: $1"
        exit 1
        ;;
    esac
    shift
  done

  if [[ "${UPDATE_KEEP_LOCAL}" -eq 1 && "${UPDATE_FORCE_REMOTE}" -eq 1 ]]; then
    log_error "Cannot use --keep-local and --force-remote together."
    exit 1
  fi

  ensure_update_dependencies
  ensure_db_dir
  ensure_manifest_exists
  printf 'Last updated at: %s\n' "$(read_last_update_at)"

  UPDATE_TMP_ROOT="$(mktemp -d "/tmp/governator-${PROJECT_NAME}-update-XXXXXX")"
  local cleanup
  cleanup() {
    if [[ -n "${UPDATE_TMP_ROOT}" ]]; then
      rm -rf "${UPDATE_TMP_ROOT}"
      UPDATE_TMP_ROOT=""
    fi
  }
  trap cleanup EXIT

  local tar_url="https://github.com/cmtonkinson/governator/archive/refs/heads/main.tar.gz"
  if ! curl -fsSL "${tar_url}" | tar -xz -C "${UPDATE_TMP_ROOT}" --strip-components=1 -f - governator-main/_governator; then
    log_error "Failed to download ${tar_url}"
    exit 1
  fi

  local upstream_dir="${UPDATE_TMP_ROOT}/_governator"
  if [[ ! -d "${upstream_dir}" ]]; then
    log_error "Update archive missing _governator directory"
    exit 1
  fi

  local code_updated=0
  local prompt_updated=0
  UPDATED_FILES=()

  local upstream_list
  upstream_list="$(mktemp)"
  list_manifest_paths "${upstream_dir}" > "${upstream_list}"

  local rel_path
  while IFS= read -r rel_path; do
    local upstream_path="${UPDATE_TMP_ROOT}/${rel_path}"
    local local_path="${ROOT_DIR}/${rel_path}"
    local upstream_sha
    upstream_sha="$(sha256_or_fail "${upstream_path}")"

    if is_code_file "${rel_path}"; then
      update_code_file "${rel_path}" "${upstream_path}" "${local_path}" "${upstream_sha}" code_updated
    elif is_prompt_file "${rel_path}"; then
      update_prompt_file "${rel_path}" "${upstream_path}" "${local_path}" "${upstream_sha}" prompt_updated
    else
      update_code_file "${rel_path}" "${upstream_path}" "${local_path}" "${upstream_sha}" code_updated
    fi
  done < "${upstream_list}"

  local local_list
  local_list="$(mktemp)"
  list_manifest_paths "${STATE_DIR}" > "${local_list}"

  while IFS= read -r rel_path; do
    if grep -Fxq "${rel_path}" "${upstream_list}"; then
      continue
    fi
    local local_path="${ROOT_DIR}/${rel_path}"
    if is_code_file "${rel_path}"; then
      remove_code_file "${local_path}" code_updated
    elif is_prompt_file "${rel_path}"; then
      remove_prompt_file "${rel_path}" "${local_path}" prompt_updated
    else
      remove_code_file "${local_path}" code_updated
    fi
  done < "${local_list}"

  rm -f "${upstream_list}" "${local_list}"

  run_governator_migrations
  chmod +x "${STATE_DIR}/governator.sh"
  write_manifest "${ROOT_DIR}" "${STATE_DIR}" "${MANIFEST_FILE}"

  if [[ "${#UPDATED_FILES[@]}" -gt 0 ]]; then
    audit_log "governator" "update applied: $(join_by ", " "${UPDATED_FILES[@]}")"
    local updated_at
    updated_at="$(timestamp_utc_seconds)"
    write_last_update_at "${updated_at}"
    log_info "Updated files:"
    printf 'Updated files:\n'
    printf '  - %s\n' "${UPDATED_FILES[@]}"
  else
    log_info "No updates applied."
    printf 'No updates applied.\n'
  fi

  git -C "${ROOT_DIR}" add "${STATE_DIR}" "${MANIFEST_FILE}" "${MIGRATIONS_STATE_FILE}"
  if [[ "${#UPDATED_FILES[@]}" -gt 0 ]]; then
    git -C "${ROOT_DIR}" add "${AUDIT_LOG}" "${LAST_UPDATE_FILE}"
  fi
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain -- "${STATE_DIR}" "${MANIFEST_FILE}" "${AUDIT_LOG}" "${LAST_UPDATE_FILE}")" ]]; then
    git -C "${ROOT_DIR}" commit -q -m "[governator] Update governator"
  fi
}
