#!/usr/bin/env bash

repo_git() {
  git -C "${REPO_DIR}" "$@"
}

# file_sha256
# Purpose: Compute a SHA-256 hash for a file using available tooling.
# Args:
#   $1: File path (string).
# Output: Prints the hash to stdout.
# Returns: 0 on success; 1 if no supported tool is available.
file_sha256() {
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

commit_all() {
  local message="$1"
  repo_git add GOVERNATOR.md _governator .governator
  repo_git commit -m "${message}" >/dev/null
  repo_git push origin main >/dev/null
}

commit_paths() {
  local message="$1"
  shift
  repo_git add "$@"
  repo_git commit -m "${message}" >/dev/null
  repo_git push origin main >/dev/null
}

write_task() {
  local dir="$1"
  local name="$2"
  cat > "${REPO_DIR}/_governator/${dir}/${name}.md" <<'EOF_TASK'
# Task
EOF_TASK
}

write_task_with_frontmatter() {
  local dir="$1"
  local name="$2"
  local frontmatter="$3"
  cat > "${REPO_DIR}/_governator/${dir}/${name}.md" <<EOF_TASK
---
${frontmatter}
---
# Task
EOF_TASK
}

complete_bootstrap() {
  mkdir -p "${REPO_DIR}/_governator/docs"
  printf '%s\n' "# ASR" > "${REPO_DIR}/_governator/docs/asr.md"
  printf '%s\n' "# arc42" > "${REPO_DIR}/_governator/docs/arc42.md"
  printf '%s\n' "# Personas" > "${REPO_DIR}/_governator/docs/personas.md"
  printf '%s\n' "# Wardley" > "${REPO_DIR}/_governator/docs/wardley.md"
  printf '%s\n' "# ADR-0001" > "${REPO_DIR}/_governator/docs/adr-0001.md"
  write_task "task-done" "000-architecture-bootstrap-architect"
  commit_paths "Complete bootstrap" \
    "_governator/docs/asr.md" \
    "_governator/docs/arc42.md" \
    "_governator/docs/personas.md" \
    "_governator/docs/wardley.md" \
    "_governator/docs/adr-0001.md" \
    "_governator/task-done/000-architecture-bootstrap-architect.md"
}

create_worker_branch() {
  local task_name="$1"
  local worker="$2"
  repo_git checkout -b "worker/${worker}/${task_name}" >/dev/null
  printf '%s\n' "work ${task_name}" > "${REPO_DIR}/work-${task_name}.txt"
  repo_git add "work-${task_name}.txt"
  repo_git commit -m "Work ${task_name}" >/dev/null
  repo_git checkout main >/dev/null
}

create_upstream_dir() {
  local upstream_root
  upstream_root="$(mktemp -d "${BATS_TMPDIR}/upstream.XXXXXX")"
  mkdir -p "${upstream_root}/governator-main"
  cp -R "${REPO_DIR}/_governator" "${upstream_root}/governator-main/_governator"
  printf '%s\n' "${upstream_root}"
}

build_upstream_tarball() {
  local upstream_root="$1"
  local tar_path="$2"
  tar -cz -C "${upstream_root}" -f "${tar_path}" governator-main/_governator
}

stub_curl_with_tarball() {
  local tar_path="$1"
  cat > "${BIN_DIR}/curl" <<EOF_CURL
#!/usr/bin/env bash
cat "${tar_path}"
EOF_CURL
  chmod +x "${BIN_DIR}/curl"
}

set_next_task_id() {
  set_config_value "next_task_id" "$1" "number"
  commit_paths "Set task id" ".governator/config.json"
}

set_config_value() {
  local key_path="$1"
  local value="$2"
  local value_type="${3:-string}"
  local tmp_file
  tmp_file="$(mktemp "${BATS_TMPDIR}/config.XXXXXX")"
  local safe_value="${value}"
  local jq_args=()
  local jq_value_expr
  if [[ "${value_type}" == "number" ]]; then
    if [[ ! "${safe_value}" =~ ^-?[0-9]+$ ]]; then
      safe_value=0
    fi
    jq_args=(--argjson value "${safe_value}")
    jq_value_expr='$value'
  else
    jq_args=(--arg value "${safe_value}")
    jq_value_expr='$value'
  fi

  jq -S --arg path "${key_path}" "${jq_args[@]}" \
    "setpath(\$path | split(\".\"); ${jq_value_expr})" \
    "${REPO_DIR}/.governator/config.json" > "${tmp_file}"
  mv "${tmp_file}" "${REPO_DIR}/.governator/config.json"
}

set_config_map_value() {
  local map_key="$1"
  local entry_key="$2"
  local value="$3"
  local value_type="${4:-string}"
  local tmp_file
  tmp_file="$(mktemp "${BATS_TMPDIR}/config.XXXXXX")"
  local safe_value="${value}"
  local jq_args=()
  local jq_value_expr
  if [[ "${value_type}" == "number" ]]; then
    if [[ ! "${safe_value}" =~ ^-?[0-9]+$ ]]; then
      safe_value=0
    fi
    jq_args=(--argjson value "${safe_value}")
    jq_value_expr='$value'
  else
    jq_args=(--arg value "${safe_value}")
    jq_value_expr='$value'
  fi

  jq -S --arg map "${map_key}" --arg entry "${entry_key}" "${jq_args[@]}" \
    "setpath([\$map, \$entry]; ${jq_value_expr})" \
    "${REPO_DIR}/.governator/config.json" > "${tmp_file}"
  mv "${tmp_file}" "${REPO_DIR}/.governator/config.json"
}

setup() {
  REPO_DIR="$(mktemp -d "${BATS_TMPDIR}/repo.XXXXXX")"
  ORIGIN_DIR="$(mktemp -d "${BATS_TMPDIR}/origin.XXXXXX")"
  BIN_DIR="$(mktemp -d "${BATS_TMPDIR}/bin.XXXXXX")"

  cp -R "${BATS_TEST_DIRNAME}/../_governator" "${REPO_DIR}/_governator"
  cp -R "${BATS_TEST_DIRNAME}/../.governator" "${REPO_DIR}/.governator"
  cp "${BATS_TEST_DIRNAME}/../GOVERNATOR.md" "${REPO_DIR}/GOVERNATOR.md"
  cp "${REPO_DIR}/_governator/templates/config.json" "${REPO_DIR}/.governator/config.json"

  repo_git init -b main >/dev/null
  repo_git config user.email "test@example.com"
  repo_git config user.name "Test User"
  repo_git add GOVERNATOR.md _governator .governator
  repo_git commit -m "Init" >/dev/null

  git init --bare "${ORIGIN_DIR}" >/dev/null
  repo_git remote add origin "${ORIGIN_DIR}"
  repo_git config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
  repo_git push -u origin main >/dev/null

  set_config_value "project_mode" "new"
  commit_paths "Set project mode" ".governator/config.json"
  local gov_sha
  gov_sha="$(file_sha256 "${REPO_DIR}/GOVERNATOR.md")"
  set_config_value "planning.gov_hash" "${gov_sha}"
  commit_paths "Set planning hash" ".governator/config.json"

  cat > "${BIN_DIR}/codex" <<'EOF_CODEX'
#!/usr/bin/env bash
exit 0
EOF_CODEX
  chmod +x "${BIN_DIR}/codex"

  export PATH="${BIN_DIR}:${PATH}"
}
