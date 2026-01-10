#!/usr/bin/env bats

load ./helpers.bash

# Migration test convention:
# - Add a new @test for each migration script in _governator/migrations.
# - Exercise the migration directly and assert on resulting filesystem state.

@test "migration moves legacy last_update_at into config and removes file" {
  printf '%s\n' "12345" > "${REPO_DIR}/.governator/last_update_at"

  run bash "${REPO_DIR}/_governator/migrations/2026-01-08-migrate-last-update-at.sh"
  [ "$status" -eq 0 ]

  run jq -r '.last_update_at' "${REPO_DIR}/.governator/config.json"
  [ "$status" -eq 0 ]
  [ "${output}" = "12345" ]
  [ ! -f "${REPO_DIR}/.governator/last_update_at" ]
}
