<!--
File: specs/v2-go-module-layout.md
Purpose: Define the Go module location and package layout for Governator v2.
-->
# Governator v2 Go Module Layout

## Decision
- The Go module root remains at the repository root (`go.mod` already lives
  here).
- The v2 CLI entrypoint is `cmd/governator`.
- Core implementation lives in `internal/` packages to keep the API surface
  intentionally narrow.
- `pkg/` is reserved for rare, intentionally exported utilities; default to
  `internal/` unless a stable public API is required.

## Proposed Package Structure
- `cmd/governator`: `main` package for the CLI binary.
- `internal/config`: config loading and precedence.
- `internal/index`: task index models and IO helpers.
- `internal/planner`: planning pipeline logic.
- `internal/scheduler`: eligibility and ordering logic.
- `internal/worker`: worker lifecycle orchestration.
- `internal/state`: lifecycle state machine and guards.
- `internal/cli`: command wiring and output formatting.

## Build and Install Notes
- `go build ./cmd/governator` produces a single `governator` binary suitable
  for Homebrew/dpkg packaging.
- This layout supports system-level installs without requiring any runtime
  module discovery beyond the repo root.

## Coexistence With v1
- v1 source remains in `v1-reference/` and is treated as read-only historical
  context, not part of the v2 module layout.
- v2 code lives entirely under the module root in `cmd/` and `internal/`,
  avoiding naming conflicts with the v1 reference snapshot.

## README Impact
- No README changes are required by this layout decision. If future changes
  would require README edits, operator approval is needed first.
