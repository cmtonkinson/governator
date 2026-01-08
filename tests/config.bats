#!/usr/bin/env bats

load ./helpers.bash

@test "read-caps prints global and role defaults" {
  run bash "${REPO_DIR}/_governator/governator.sh" read-caps
  local caps_output="${output}"
  run grep -F "global 1" <<< "${caps_output}"
  [ "$status" -eq 0 ]
  run grep -F "ruby 1" <<< "${caps_output}"
  [ "$status" -eq 0 ]
}

@test "read-caps returns configured role cap" {
  set_config_map_value "worker_caps" "ruby" "4" "number"

  run bash "${REPO_DIR}/_governator/governator.sh" read-caps ruby
  [ "$status" -eq 0 ]
  [ "${output}" = "4" ]
}

@test "init supports defaults and non-interactive options" {
  rm -f "${REPO_DIR}/.governator/config.json"
  commit_paths "Clear init files" ".governator"

  run bash "${REPO_DIR}/_governator/governator.sh" init --defaults
  [ "$status" -eq 0 ]
  run jq -r '.project_mode // ""' "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  [ "${output}" = "new" ]
  run cat "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  run grep -F "\"default\": \"medium\"" "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  run grep -F "\"architect\": \"high\"" "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  run grep -F "\"planner\": \"high\"" "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  run grep -F "\"test_engineer\": \"low\"" "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]

  rm -f "${REPO_DIR}/.governator/config.json"
  commit_paths "Clear init files again" ".governator"

  run bash "${REPO_DIR}/_governator/governator.sh" init --non-interactive --project-mode=existing --remote=upstream --branch=trunk
  [ "$status" -eq 0 ]
  run jq -r '.project_mode // ""' "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  [ "${output}" = "existing" ]
  run jq -r '.remote_name // ""' "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  [ "${output}" = "upstream" ]
  run jq -r '.default_branch // ""' "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  [ "${output}" = "trunk" ]
}
