#!/usr/bin/env bats

load ./helpers.bash

@test "check-zombies retries when branch missing and worker dead" {
  write_task "task-assigned" "007-zombie-ruby"
  add_in_flight "007-zombie-ruby" "ruby"
  create_worktree_dir "007-zombie-ruby" "ruby" >/dev/null
  add_worker_process "007-zombie-ruby" "ruby" "999999"
  commit_all "Prepare zombie task"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  run grep -F "007-zombie-ruby | 1" "${REPO_DIR}/.governator/retry-counts.log"
  [ "$status" -eq 0 ]
}

@test "check-zombies retries when worker process record is missing" {
  write_task "task-assigned" "026-missing-proc-ruby"
  add_in_flight "026-missing-proc-ruby" "ruby"
  commit_all "Prepare missing worker process record"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  run grep -F "026-missing-proc-ruby | 1" "${REPO_DIR}/.governator/retry-counts.log"
  [ "$status" -eq 0 ]
}

@test "check-zombies blocks after second failure" {
  write_task "task-assigned" "008-stuck-ruby"
  add_in_flight "008-stuck-ruby" "ruby"
  add_retry_count "008-stuck-ruby" "1"
  create_worktree_dir "008-stuck-ruby" "ruby" >/dev/null
  add_worker_process "008-stuck-ruby" "ruby" "999999"
  commit_all "Prepare stuck task"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/008-stuck-ruby.md" ]
  run grep -F "008-stuck-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "008-stuck-ruby |" "${REPO_DIR}/.governator/retry-counts.log"
  [ "$status" -ne 0 ]
}

@test "check-zombies blocks multiple tasks in one pass" {
  write_task "task-assigned" "012-zombie-a-ruby"
  write_task "task-assigned" "013-zombie-b-ruby"
  add_in_flight "012-zombie-a-ruby" "ruby"
  add_in_flight "013-zombie-b-ruby" "ruby"
  add_retry_count "012-zombie-a-ruby" "1"
  add_retry_count "013-zombie-b-ruby" "1"
  create_worktree_dir "012-zombie-a-ruby" "ruby" >/dev/null
  create_worktree_dir "013-zombie-b-ruby" "ruby" >/dev/null
  add_worker_process "012-zombie-a-ruby" "ruby" "999999"
  add_worker_process "013-zombie-b-ruby" "ruby" "999999"
  commit_all "Prepare multiple zombie tasks"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  [ -f "${REPO_DIR}/_governator/task-blocked/012-zombie-a-ruby.md" ]
  [ -f "${REPO_DIR}/_governator/task-blocked/013-zombie-b-ruby.md" ]
  run grep -F "012-zombie-a-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
  run grep -F "013-zombie-b-ruby -> ruby" "${REPO_DIR}/.governator/in-flight.log"
  [ "$status" -ne 0 ]
}

@test "check-zombies recovers reviewer output by committing review branch" {
  write_task "task-worked" "016-review-ruby"
  add_in_flight "016-review-ruby" "reviewer"

  # Add worktrees to gitignore to prevent dirty repo detection
  echo ".governator/worktrees/" >> "${REPO_DIR}/.gitignore"

  # Commit task file, in-flight log, and gitignore (before creating worktree)
  commit_paths "Prepare reviewer recovery task" GOVERNATOR.md _governator .governator .gitignore

  # Create worktree for the reviewer
  worktree_dir="${REPO_DIR}/.governator/worktrees/016-review-ruby-reviewer"
  mkdir -p "${REPO_DIR}/.governator/worktrees"
  git -C "${REPO_DIR}" worktree add -b "worker/reviewer/016-review-ruby" "${worktree_dir}" "main" >/dev/null 2>&1
  git -C "${worktree_dir}" config user.email "test@example.com"
  git -C "${worktree_dir}" config user.name "Test User"

  # Create review.json in the worktree (simulating reviewer crash before commit)
  cat > "${worktree_dir}/review.json" <<'EOF_REVIEW'
{"result":"reject","comments":["needs work"]}
EOF_REVIEW

  # Add worker process record (don't commit to avoid embedded repo warning)
  echo "016-review-ruby | reviewer | 999999 | ${worktree_dir} | worker/reviewer/016-review-ruby | 0" >> "${REPO_DIR}/.governator/worker-processes.log"

  # Get the base commit to compare against
  base_commit="$(git -C "${REPO_DIR}" rev-parse main)"

  run bash "${REPO_DIR}/_governator/governator.sh" check-zombies
  [ "$status" -eq 0 ]

  # Verify the local branch exists and has commits beyond the base
  run git -C "${REPO_DIR}" rev-list --count "${base_commit}..worker/reviewer/016-review-ruby"
  [ "$status" -eq 0 ]
  [ "$output" -gt 0 ]
}

@test "cleanup-tmp removes stale worktrees but keeps active ones" {
  active_dir="$(create_worktree_dir "009-cleanup-ruby" "ruby")"
  stale_dir="${REPO_DIR}/.governator/worktrees/stale-task-ruby"
  mkdir -p "${stale_dir}"
  touch -t 202001010000 "${stale_dir}"

  set_config_value "worker_timeout_seconds" "1" "number"
  add_worker_process "009-cleanup-ruby" "ruby" "1234"
  commit_all "Prepare cleanup dirs"

  run bash "${REPO_DIR}/_governator/governator.sh" cleanup-tmp
  [ "$status" -eq 0 ]

  [ -d "${active_dir}" ]
  [ ! -d "${stale_dir}" ]
}

@test "cleanup-tmp dry-run lists stale worktrees only" {
  active_dir="$(create_worktree_dir "017-cleanup-ruby" "ruby")"
  stale_dir="${REPO_DIR}/.governator/worktrees/stale-task-sre"
  mkdir -p "${stale_dir}"
  touch -t 202001010000 "${stale_dir}"

  set_config_value "worker_timeout_seconds" "1" "number"
  add_worker_process "017-cleanup-ruby" "ruby" "1234"
  commit_all "Prepare cleanup dry-run"

  run bash "${REPO_DIR}/_governator/governator.sh" cleanup-tmp --dry-run
  [ "$status" -eq 0 ]
  run grep -F "${stale_dir}" <<< "${output}"
  [ "$status" -eq 0 ]
  run grep -F "${active_dir}" <<< "${output}"
  [ "$status" -ne 0 ]
}

@test "count-in-flight totals and per-role counts" {
  add_in_flight "014-one-ruby" "ruby"
  add_in_flight "015-one-sre" "sre"
  commit_all "Add in-flight"

  run bash "${REPO_DIR}/_governator/governator.sh" count-in-flight
  [ "$status" -eq 0 ]
  [ "${output}" = "2" ]

  run bash "${REPO_DIR}/_governator/governator.sh" count-in-flight ruby
  [ "$status" -eq 0 ]
  [ "${output}" = "1" ]
}

@test "read_agent_provider errors when default provider is missing" {
  tmp_file="$(mktemp "${BATS_TMPDIR}/config.XXXXXX")"
  jq 'del(.agents.provider_by_role.default)' "${REPO_DIR}/.governator/config.json" > "${tmp_file}"
  mv "${tmp_file}" "${REPO_DIR}/.governator/config.json"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    CONFIG_FILE=\"\${DB_DIR}/config.json\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"\${STATE_DIR}/lib/utils.sh\"
    source \"\${STATE_DIR}/lib/logging.sh\"
    source \"\${STATE_DIR}/lib/config.sh\"
    read_agent_provider \"generalist\"
  "
  [ "$status" -ne 0 ]
  [[ "${output}" == *"Missing agents.provider_by_role.default"* ]]
}

@test "read_agent_provider_bin errors when binary is missing" {
  set_config_value "agents.providers.bad.bin" "missing-bin-123" "string"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    CONFIG_FILE=\"\${DB_DIR}/config.json\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"\${STATE_DIR}/lib/utils.sh\"
    source \"\${STATE_DIR}/lib/logging.sh\"
    source \"\${STATE_DIR}/lib/config.sh\"
    read_agent_provider_bin \"bad\"
  "
  [ "$status" -ne 0 ]
  [[ "${output}" == *"Agent provider binary not found in PATH"* ]]
}

@test "read_agent_provider_bin errors when binary is not executable" {
  bin_path="$(mktemp "${BATS_TMPDIR}/bin.XXXXXX")"
  printf '%s\n' "echo nope" > "${bin_path}"
  chmod 600 "${bin_path}"
  set_config_value "agents.providers.bad.bin" "${bin_path}" "string"

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    CONFIG_FILE=\"\${DB_DIR}/config.json\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"\${STATE_DIR}/lib/utils.sh\"
    source \"\${STATE_DIR}/lib/logging.sh\"
    source \"\${STATE_DIR}/lib/config.sh\"
    read_agent_provider_bin \"bad\"
  "
  [ "$status" -ne 0 ]
  [[ "${output}" == *"Agent provider binary not executable"* ]]
}

@test "build_worker_command uses provider args and reasoning substitution" {
  bin_path="$(mktemp "${BATS_TMPDIR}/worker-bin.XXXXXX")"
  cat > "${bin_path}" <<'EOF_BIN'
#!/usr/bin/env bash
exit 0
EOF_BIN
  chmod +x "${bin_path}"

  set_config_value "agents.provider_by_role.default" "unit" "string"
  set_config_value "agents.providers.unit.bin" "${bin_path}" "string"
  tmp_file="$(mktemp "${BATS_TMPDIR}/config.XXXXXX")"
  jq '
    .agents.providers.unit.args = ["--foo", "{REASONING_EFFORT}", "--bar"]
  ' "${REPO_DIR}/.governator/config.json" > "${tmp_file}"
  mv "${tmp_file}" "${REPO_DIR}/.governator/config.json"
  set_config_map_value "reasoning_effort" "default" "high" "string"

  # Verify config was updated (sanity check for CI debugging)
  run jq -r '.reasoning_effort.default' "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  [ "$output" = "high" ]

  # Additional debug: show reasoning_effort section
  echo "DEBUG: reasoning_effort section:" >&2
  jq '.reasoning_effort' "${REPO_DIR}/.governator/config.json" >&2

  # Debug: test the exact jq command that config_json_read_map_value uses
  echo "DEBUG: Testing jq variable access:" >&2
  echo "  Direct path result: $(jq -r '.reasoning_effort.default' "${REPO_DIR}/.governator/config.json")" >&2
  echo "  Variable access result: $(jq -r --arg map "reasoning_effort" --arg def "default" '.[$map][$def]' "${REPO_DIR}/.governator/config.json")" >&2
  echo "DEBUG: CONFIG_FILE will be: ${REPO_DIR}/.governator/config.json" >&2

  ROOT_DIR="${REPO_DIR}"
  STATE_DIR="${REPO_DIR}/_governator"
  DB_DIR="${REPO_DIR}/.governator"
  CONFIG_FILE="${DB_DIR}/config.json"
  GOV_QUIET=1
  GOV_VERBOSE=0
  source "${STATE_DIR}/lib/utils.sh"
  source "${STATE_DIR}/lib/logging.sh"
  source "${STATE_DIR}/lib/config.sh"
  source "${STATE_DIR}/lib/workers.sh"

  # Debug: call read function directly to see what it returns
  echo "DEBUG: CONFIG_FILE=${CONFIG_FILE}" >&2
  echo "DEBUG: Calling config_json_read_map_value directly:" >&2
  debug_value="$(config_json_read_map_value "reasoning_effort" "generalist" "default" "medium")"
  echo "DEBUG:   config_json_read_map_value returned: '${debug_value}'" >&2
  echo "DEBUG: Calling read_reasoning_effort directly:" >&2
  debug_reasoning="$(read_reasoning_effort "generalist")"
  echo "DEBUG:   read_reasoning_effort returned: '${debug_reasoning}'" >&2

  build_worker_command "generalist" "Prompt text"
  [ "$?" -eq 0 ]
  [ "${WORKER_COMMAND[0]}" = "${bin_path}" ]
  [ "${WORKER_COMMAND[1]}" = "--foo" ]
  # Debug: print actual value if assertion would fail
  if [ "${WORKER_COMMAND[2]}" != "high" ]; then
    echo "DEBUG: WORKER_COMMAND[2]='${WORKER_COMMAND[2]}' (expected 'high')" >&2
    echo "DEBUG: Full WORKER_COMMAND: ${WORKER_COMMAND[*]}" >&2
  fi
  [ "${WORKER_COMMAND[2]}" = "high" ]
  [ "${WORKER_COMMAND[3]}" = "--bar" ]
  [ "${WORKER_COMMAND[4]}" = "Prompt text" ]
}

@test "build_worker_prompt includes reasoning file only for non-codex providers" {
  set_config_value "agents.provider_by_role.default" "gemini" "string"

  # Verify config was updated (sanity check for CI debugging)
  run jq -r '.agents.provider_by_role.default' "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  [ "$output" = "gemini" ]

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    CONFIG_FILE=\"\${DB_DIR}/config.json\"
    ROLES_DIR=\"\${STATE_DIR}/roles\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"\${STATE_DIR}/lib/utils.sh\"
    source \"\${STATE_DIR}/lib/logging.sh\"
    source \"\${STATE_DIR}/lib/config.sh\"
    source \"\${STATE_DIR}/lib/workers.sh\"
    build_worker_prompt \"generalist\" \"_governator/task-backlog/001-test-generalist.md\"
  "
  [ "$status" -eq 0 ]
  [[ "${output}" == *"_governator/reasoning/medium.md"* ]]

  set_config_value "agents.provider_by_role.default" "codex" "string"

  # Verify config was updated (sanity check for CI debugging)
  run jq -r '.agents.provider_by_role.default' "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  [ "$output" = "codex" ]

  run bash -c "
    set -euo pipefail
    ROOT_DIR=\"${REPO_DIR}\"
    STATE_DIR=\"${REPO_DIR}/_governator\"
    DB_DIR=\"${REPO_DIR}/.governator\"
    CONFIG_FILE=\"\${DB_DIR}/config.json\"
    ROLES_DIR=\"\${STATE_DIR}/roles\"
    GOV_QUIET=1
    GOV_VERBOSE=0
    source \"\${STATE_DIR}/lib/utils.sh\"
    source \"\${STATE_DIR}/lib/logging.sh\"
    source \"\${STATE_DIR}/lib/config.sh\"
    source \"\${STATE_DIR}/lib/workers.sh\"
    build_worker_prompt \"generalist\" \"_governator/task-backlog/001-test-generalist.md\"
  "
  [ "$status" -eq 0 ]
  [[ "${output}" != *"_governator/reasoning/"* ]]
}
