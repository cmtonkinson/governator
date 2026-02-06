# Refactoring Plan: Governator Codebase

## Context

The governator codebase has accumulated significant code duplication across several critical areas: identical supervisor state types and CRUD functions, duplicated formatting utilities between display packages, near-identical supervisor launch functions in main.go, repeated phase-mapping switch statements, and massive structural duplication across the five orchestrator stage handlers. This refactoring reduces duplication, improves maintainability, and increases cohesion — without changing public APIs or runtime behavior.

All tests currently pass. Each step produces an independently testable commit.

---

## Step 1: Extract shared formatting utilities

**Why:** `formatDurationShort()`, `formatTokens()`, and `formatPID()` are duplicated identically between `status/status.go` and `tui/status.go`.

**Changes:**
- Create `internal/format/format.go` — export `DurationShort(d time.Duration) string`, `Tokens(n int) string`, `PID(pid int) string`
- Create `internal/format/format_test.go` — unit tests for edge cases
- Modify `internal/status/status.go` — remove local `formatDurationShort`, `formatTokens`; replace calls with `format.DurationShort`, `format.Tokens`
- Modify `internal/tui/status.go` — remove local duplicates, including collapsing `formatRuntime` to call `format.DurationShort(time.Since(startedAt))`

**Risk:** Very low. All functions are unexported. No public API changes.
**Lines saved:** ~50

---

## Step 2: Extract `stepToPhase` for repeated switch statements

**Why:** Three identical switch statements map step names ("architecture-baseline", "gap-analysis", etc.) to `phase.Phase` values across `phase_runner.go` and `planning_controller.go`.

**Changes:**
- Add `stepToPhase(stepName string) phase.Phase` in `internal/run/step_phase.go` (or inline in `phase_runner.go`) using `phase.ParsePhase()`
- Replace switch blocks in:
  - `internal/run/phase_runner.go` lines 140-151 and 225-238
  - `internal/run/planning_controller.go` lines 172-185 and 189-205

**Risk:** Very low. All unexported, within `run` package.
**Lines saved:** ~40

---

## Step 3: Unify supervisor state types

**Why:** `PlanningSupervisorState` and `ExecutionSupervisorState` are field-for-field identical structs. Six CRUD functions are duplicated (`Load*`, `Save*`, `Clear*`, `*Running`).

**Changes to `internal/supervisor/state.go`:**
- Add `SupervisorKind` type with `SupervisorKindPlanning` and `SupervisorKindExecution` constants
- Add unified `SupervisorStateInfo` struct (same fields as both current types)
- Make `PlanningSupervisorState` and `ExecutionSupervisorState` type aliases (`= SupervisorStateInfo`)
- Add generic CRUD: `LoadState(repoRoot, kind)`, `SaveState(repoRoot, kind, state)`, `ClearState(repoRoot, kind)`, `SupervisorRunning(repoRoot, kind)`
- Convert old functions to thin wrappers that delegate to generic versions
- Add `StatePath(repoRoot, kind)` and `LogPath(repoRoot, kind)` helpers; keep old path functions as wrappers
- Add `internal/supervisor/state_test.go`

**Callers (all continue to compile via type aliases):**
- `main.go` — constructs state structs, calls Save/Clear
- `internal/status/status.go` — calls Load/Running
- `internal/run/planning_supervisor.go`, `execution_supervisor.go` — throughout
- `internal/run/planning_supervisor_control.go`, `execution_supervisor_control.go`

**Also unify in `internal/run/`:**
- Merge `planningSupervisorStateEqual` / `executionSupervisorStateEqual` into `supervisorStateEqual`
- Merge `markPlanningSupervisorTransition` / `markExecutionSupervisorTransition` into `markSupervisorTransition`

**Risk:** Low-medium. Type aliases are backward-compatible. Wrapper functions preserve all signatures.
**Lines saved:** ~120 in state.go, ~30 in supervisor files

---

## Step 4: Extract common supervisor launch logic in main.go

**Why:** `runPlan()` (lines 263-349) and `runExecute()` (lines 363-449) are ~90% identical, differing only in: log path, command arg, phase name, and print message.

**Changes to `main.go`:**
- Extract `launchSupervisor(kind supervisor.SupervisorKind, commandArg string)` helper containing the shared logic: check running, check locks, create log dir, open log file, spawn process, save state, release, print
- Simplify `runPlan` and `runExecute` to parse flags then call `launchSupervisor`

**Risk:** Low. Both functions are unexported. Existing CLI tests cover behavior.
**Lines saved:** ~70

---

## Step 5: Extract orchestrator stage helpers

**Why:** The five `Execute*Stage()` functions in `orchestrator.go` (2083 lines total) share massive duplication in their task collection loops: timeout handling (~30 identical lines per stage), completion handling (~30 lines), worktree resolution, and metric accumulation.

**Changes to `internal/run/orchestrator.go`:**

**5a: Extract `handleTaskTimeout()` helper**
- Takes: task, inflight entry, worktree path, config, stage, fail state, state updater func, auditors, opts
- Returns: whether timeout occurred, blocked count delta, inflight-updated flag
- Replaces ~30-line timeout blocks in Work, Test, Review, ConflictResolution stages

**5b: Extract `collectCompletedTask()` helper**
- Handles: exit code check, `finalizeStageSuccess` call, `logAgentOutcome`, state update, emit, inflight removal
- Parameterized by: stage name, state updater function, success emitter
- Replaces ~30-line completion blocks in each stage

**5c: Add `(m *ExecutionMetrics).Accumulate(other ExecutionMetrics)` method**
- Replaces the 4-line accumulation pattern repeated in each stage's success path

**What NOT to do:**
- Don't try to template entire stage functions — the review stage has inline merge flow, the merge stage has no inflight/collection loop, and the conflict stage has unique role selection logic
- Focus only on the clearly identical blocks

**Risk:** Medium. Orchestrator is the largest and most complex file. Existing test coverage is strong (orchestrator_test.go, work_stage_test.go, test_stage_test.go, conflict_resolution_test.go, merge_test.go, lifecycle_e2e_test.go).
**Lines saved:** ~200-300

---

## Intentionally Not Performed

These were identified as opportunities but deferred for scope/risk reasons:

1. **Unified orchestrator stage handler interface** — The stage functions diverge enough (review has merge flow, merge has no inflight) that a generic interface would add complexity rather than remove it. The helper extraction in Step 5 captures the low-hanging fruit.
2. **`internal/util/` package for `repoRelativePath`, `pathExists`, `cloneStrings`** — Cross-package extraction of small utilities. Valid but lower leverage; risks introducing a grab-bag package.
3. **Config parsing generification** — The 7 parse functions in `config/loader.go` follow a common pattern, but generifying with reflection adds complexity. Better addressed when Go adds more generic stdlib support.
4. **Worker package deduplication** — `selectCommandTemplate()` / `selectCLIName()` duplication, shared validation patterns. Valid but smaller impact.

---

## Verification

After each step:
1. `go build ./...` — confirms compilation
2. `go vet ./...` — checks for issues
3. `go test ./...` — all tests pass (currently 25 packages, all passing)

For Step 5 specifically, run the lifecycle e2e test in isolation:
```
go test ./internal/run/ -run TestLifecycle -v
go test ./test/ -v
```
