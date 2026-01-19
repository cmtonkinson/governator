#!/usr/bin/env bash

# repo_git
# Purpose: Run git commands in the test repo.
# Args:
#   $@: Git arguments.
# Output: Passthrough git output.
# Returns: Exit code from git.
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

# commit_all
# Purpose: Commit and push standard Governator files in tests.
# Args:
#   $1: Commit message (string).
# Output: None.
# Returns: 0 on success; propagates git errors.
commit_all() {
  local message="$1"
  repo_git add -f GOVERNATOR.md _governator _governator/_durable_state
  repo_git commit -m "${message}" >/dev/null
  repo_git push origin main >/dev/null
}

# commit_paths
# Purpose: Commit and push specific paths in tests.
# Args:
#   $1: Commit message (string).
#   $@: Paths to add (strings).
# Output: None.
# Returns: 0 on success; propagates git errors.
commit_paths() {
  local message="$1"
  shift
  repo_git add -f "$@"
  repo_git commit -m "${message}" >/dev/null
  repo_git push origin main >/dev/null
}

# write_task
# Purpose: Create a minimal task file in the requested directory.
# Args:
#   $1: Task directory name (string).
#   $2: Task base name (string).
# Output: Writes a task file to disk.
# Returns: 0 on success.
write_task() {
  local dir="$1"
  local name="$2"
  cat > "${REPO_DIR}/_governator/${dir}/${name}.md" <<'EOF_TASK'
# Task
EOF_TASK
}

# write_task_with_frontmatter
# Purpose: Create a task file with provided frontmatter.
# Args:
#   $1: Task directory name (string).
#   $2: Task base name (string).
#   $3: Frontmatter content (string).
# Output: Writes a task file to disk.
# Returns: 0 on success.
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

# complete_bootstrap
# Purpose: Seed the repo with required bootstrap artifacts and mark complete.
# Args: None.
# Output: Writes bootstrap artifacts and commits them.
# Returns: 0 on success.
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

# create_worker_branch
# Purpose: Create a worker branch with a simple commit for tests.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
# Output: Creates a commit on the worker branch.
# Returns: 0 on success; propagates git errors.
create_worker_branch() {
  local task_name="$1"
  local worker="$2"
  repo_git checkout -b "worker/${worker}/${task_name}" >/dev/null
  printf '%s\n' "work ${task_name}" > "${REPO_DIR}/work-${task_name}.txt"
  repo_git add "work-${task_name}.txt"
  repo_git commit -m "Work ${task_name}" >/dev/null
  repo_git checkout main >/dev/null
}

# add_in_flight
# Purpose: Add a task/worker pair to the in-flight log.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
add_in_flight() {
  local task_name="$1"
  local worker="$2"
  printf '%s -> %s\n' "${task_name}" "${worker}" >> "${REPO_DIR}/_governator/_local_state/in-flight.log"
}

# add_retry_count
# Purpose: Add a retry count entry for a task.
# Args:
#   $1: Task name (string).
#   $2: Count (number).
add_retry_count() {
  local task_name="$1"
  local count="$2"
  printf '%s | %s\n' "${task_name}" "${count}" >> "${REPO_DIR}/_governator/_local_state/retry-counts.log"
}

# worktree_dir_for
# Purpose: Get the worktree directory path for a task/worker.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
# Output: Prints the worktree path.
worktree_dir_for() {
  local task_name="$1"
  local worker="$2"
  printf '%s/_governator/_local_state/worktrees/%s-%s' "${REPO_DIR}" "${task_name}" "${worker}"
}

# create_worktree_dir
# Purpose: Create a worktree directory for a task/worker.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
# Output: Prints the worktree path.
create_worktree_dir() {
  local task_name="$1"
  local worker="$2"
  local worktree_dir
  worktree_dir="$(worktree_dir_for "${task_name}" "${worker}")"
  mkdir -p "${worktree_dir}"
  printf '%s' "${worktree_dir}"
}

# add_worker_process
# Purpose: Add a worker process entry to the log.
# Args:
#   $1: Task name (string).
#   $2: Worker role (string).
#   $3: PID (number).
#   $4: Worktree directory (string, optional - defaults to standard path).
#   $5: Started at timestamp (number, optional - defaults to 0).
add_worker_process() {
  local task_name="$1"
  local worker="$2"
  local pid="$3"
  local worktree_dir="${4:-$(worktree_dir_for "${task_name}" "${worker}")}"
  local started_at="${5:-0}"
  local branch="worker/${worker}/${task_name}"
  printf '%s | %s | %s | %s | %s | %s\n' \
    "${task_name}" "${worker}" "${pid}" "${worktree_dir}" "${branch}" "${started_at}" \
    >> "${REPO_DIR}/_governator/_local_state/worker-processes.log"
}

# create_upstream_dir
# Purpose: Create a temp upstream directory containing _governator files.
# Args:
#   $1: Release tag (optional, defaults to "v1.0.0").
# Output: Prints the upstream root path.
# Returns: 0 on success.
create_upstream_dir() {
  local tag="${1:-v1.0.0}"
  local version="${tag#v}"
  local upstream_root
  upstream_root="$(mktemp -d "${BATS_TMPDIR}/upstream.XXXXXX")"
  mkdir -p "${upstream_root}/governator-${version}"
  cp -R "${REPO_DIR}/_governator" "${upstream_root}/governator-${version}/_governator"
  printf '%s\n' "${upstream_root}"
}

# create_upstream_commit_dir
# Purpose: Create a temp upstream directory using commit archive layout.
# Args:
#   $1: Commit SHA (string).
# Output: Prints the upstream root path.
# Returns: 0 on success.
create_upstream_commit_dir() {
  local commit_sha="$1"
  local upstream_root
  upstream_root="$(mktemp -d "${BATS_TMPDIR}/upstream.XXXXXX")"
  mkdir -p "${upstream_root}/governator-${commit_sha}"
  cp -R "${REPO_DIR}/_governator" "${upstream_root}/governator-${commit_sha}/_governator"
  printf '%s\n' "${upstream_root}"
}

# build_upstream_tarball
# Purpose: Package an upstream directory into a tarball for update tests.
# Args:
#   $1: Upstream root path (string).
#   $2: Output tarball path (string).
#   $3: Release tag (string, e.g., "v1.0.0").
# Output: Writes a tarball to disk.
# Returns: 0 on success.
build_upstream_tarball() {
  local upstream_root="$1"
  local tar_path="$2"
  local tag="${3:-v1.0.0}"
  local version="${tag#v}"
  tar -cz -C "${upstream_root}" -f "${tar_path}" "governator-${version}/_governator"
}

# build_upstream_commit_tarball
# Purpose: Package a commit-layout upstream directory into a tarball for update tests.
# Args:
#   $1: Upstream root path (string).
#   $2: Output tarball path (string).
#   $3: Commit SHA (string).
# Output: Writes a tarball to disk.
# Returns: 0 on success.
build_upstream_commit_tarball() {
  local upstream_root="$1"
  local tar_path="$2"
  local commit_sha="$3"
  tar -cz -C "${upstream_root}" -f "${tar_path}" "governator-${commit_sha}/_governator"
}

# stub_curl_for_release
# Purpose: Stub curl to simulate GitHub release API, checksum, and tarball downloads.
# Args:
#   $1: Tarball path (string).
#   $2: Release tag (string, e.g., "v1.0.0").
#   $3: Checksum override (optional, for testing verification failure).
#   $4: Commit SHA to return from commit API (optional).
#   $5: Compare status to return (optional).
# Output: Writes a curl stub into BIN_DIR that handles different URLs.
# Returns: 0 on success.
stub_curl_for_release() {
  local tar_path="$1"
  local tag="${2:-v1.0.0}"
  local checksum_override="${3:-}"
  local commit_sha="${4:-deadbeefdeadbeefdeadbeefdeadbeefdeadbeef}"
  local compare_status="${5:-ahead}"

  local actual_checksum
  actual_checksum="$(file_sha256 "${tar_path}")"
  local checksum_to_serve="${checksum_override:-${actual_checksum}}"

  cat > "${BIN_DIR}/curl" <<EOF_CURL
#!/usr/bin/env bash
# Stub curl for update tests
url=""
output_file=""
while [[ "\$#" -gt 0 ]]; do
  case "\$1" in
    -o) output_file="\$2"; shift ;;
    -*) ;;  # ignore other flags
    *) url="\$1" ;;
  esac
  shift
done

if [[ "\${url}" == *"/releases/latest"* ]]; then
  # GitHub API: return release info JSON
  printf '{"tag_name": "${tag}"}\n'
elif [[ "\${url}" == *"/commits/"* ]]; then
  # GitHub API: resolve ref to commit SHA
  printf '{"sha": "${commit_sha}"}\n'
elif [[ "\${url}" == *"/compare/"* ]]; then
  # GitHub API: compare commits
  printf '{"status": "${compare_status}"}\n'
elif [[ "\${url}" == *"/checksums.sha256"* ]]; then
  # Checksum file download
  printf '%s  governator-${tag}.tar.gz\n' "${checksum_to_serve}"
elif [[ "\${url}" == *".tar.gz"* ]]; then
  # Tarball download
  if [[ -n "\${output_file}" ]]; then
    cat "${tar_path}" > "\${output_file}"
  else
    cat "${tar_path}"
  fi
else
  echo "Unknown URL: \${url}" >&2
  exit 1
fi
EOF_CURL
  chmod +x "${BIN_DIR}/curl"
}

# set_next_task_id
# Purpose: Set the next task id in config for tests.
# Args:
#   $1: Task id (number).
# Output: Writes config and commits it.
# Returns: 0 on success.
set_next_task_id() {
  set_config_value "next_task_id" "$1" "number"
  commit_paths "Set task id" "_governator/_durable_state/config.json"
}

# set_config_value
# Purpose: Update a scalar config value via jq.
# Args:
#   $1: Dot-delimited key path (string).
#   $2: Value to write (string).
#   $3: Value type ("string" or "number").
# Output: Writes config.json to disk.
# Returns: 0 on success; 1 on jq failure.
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

  if ! jq -S --arg path "${key_path}" "${jq_args[@]}" \
    "setpath(\$path | split(\".\"); ${jq_value_expr})" \
    "${REPO_DIR}/_governator/_durable_state/config.json" > "${tmp_file}"; then
    echo "set_config_value: jq failed for path '${key_path}'" >&2
    rm -f "${tmp_file}"
    return 1
  fi
  mv "${tmp_file}" "${REPO_DIR}/_governator/_durable_state/config.json"
}

# set_config_map_value
# Purpose: Update a map entry in config.json via jq.
# Args:
#   $1: Map key (string).
#   $2: Entry key (string).
#   $3: Value to write (string).
#   $4: Value type ("string" or "number").
# Output: Writes config.json to disk.
# Returns: 0 on success; 1 on jq failure.
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

  if ! jq -S --arg map "${map_key}" --arg entry "${entry_key}" "${jq_args[@]}" \
    "setpath([\$map, \$entry]; ${jq_value_expr})" \
    "${REPO_DIR}/_governator/_durable_state/config.json" > "${tmp_file}"; then
    echo "set_config_map_value: jq failed for map '${map_key}' entry '${entry_key}'" >&2
    rm -f "${tmp_file}"
    return 1
  fi
  mv "${tmp_file}" "${REPO_DIR}/_governator/_durable_state/config.json"
}

# setup
# Purpose: Initialize a test repo, origin, and tool stubs for bats.
# Args: None.
# Output: Sets global vars and writes repo files.
# Returns: 0 on success.
setup() {
  REPO_DIR="$(mktemp -d "${BATS_TMPDIR}/repo.XXXXXX")"
  ORIGIN_DIR="$(mktemp -d "${BATS_TMPDIR}/origin.XXXXXX")"
  BIN_DIR="$(mktemp -d "${BATS_TMPDIR}/bin.XXXXXX")"

  cp -R "${BATS_TEST_DIRNAME}/../_governator" "${REPO_DIR}/_governator"
  cp -R "${BATS_TEST_DIRNAME}/../_governator/_local_state" "${REPO_DIR}/_governator/_local_state"
  cp "${BATS_TEST_DIRNAME}/../GOVERNATOR.md" "${REPO_DIR}/GOVERNATOR.md"
  cp "${REPO_DIR}/_governator/templates/config.json" "${REPO_DIR}/_governator/_durable_state/config.json"

  repo_git init -b main >/dev/null
  repo_git config user.email "test@example.com"
  repo_git config user.name "Test User"
  repo_git add -f GOVERNATOR.md _governator _governator/_durable_state
  repo_git commit -m "Init" >/dev/null

  git init --bare "${ORIGIN_DIR}" >/dev/null
  repo_git remote add origin "${ORIGIN_DIR}"
  repo_git config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
  repo_git push -u origin main >/dev/null

  set_config_value "project_mode" "new"
  commit_paths "Set project mode" "_governator/_durable_state/config.json"
  local gov_sha
  gov_sha="$(file_sha256 "${REPO_DIR}/GOVERNATOR.md")"
  set_config_value "planning.gov_hash" "${gov_sha}"
  commit_paths "Set planning hash" "_governator/_durable_state/config.json"

  cat > "${BIN_DIR}/codex" <<'EOF_CODEX'
#!/usr/bin/env bash
exit 0
EOF_CODEX
  chmod +x "${BIN_DIR}/codex"

  export PATH="${BIN_DIR}:${PATH}"
}
