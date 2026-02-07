# End-to-End Test Coverage Opportunities

This document outlines opportunities to add or improve end-to-end (e2e) test coverage within the `governator` project. The focus is on verifying critical contracts, system boundaries, interfaces, and overall application logic through realistic user-like scenarios.

## Areas for e2e Test Enhancement

### 1. Core Supervisors and Orchestration

*   **`internal/run/orchestrator.go`**:
    *   **Objective:** Verify the complete end-to-end lifecycle of a Governator plan, from initial planning phase through execution, commit, and potential merge, ensuring seamless transitions and correct state management across all stages.
    *   **Scenarios:**
        *   A simple plan with one planning step and one execution task, both completing successfully.
        *   A multi-phase plan involving planning, execution, and a final merge, verifying that outputs from earlier phases correctly influence later ones.
        *   Plans with parallel planning steps or execution tasks, confirming correct concurrency and dependency handling.
        *   A plan where a worker fails but the system retries successfully (if retry configured).
        *   A plan where a worker fails permanently, leading to a gracefully handled failure state for the overall plan.
    *   **Focus:** Inter-component communication (orchestrator to supervisors), overall workflow correctness, error propagation, and final output state.

*   **`internal/run/planning_supervisor.go`**:
    *   **Objective:** Extend existing planning supervisor tests to cover more complex planning scenarios, including varied validation types and dynamic prompt generation.
    *   **Scenarios:**
        *   Planning steps with mixed validation types (file existence, directory existence, content patterns, custom scripts).
        *   Scenarios where planning prompts are dynamically generated or require inputs from previous steps.
        *   Tests for planning steps that produce no outputs, ensuring the supervisor correctly handles this.
        *   Planning with invalid configurations (e.g., missing prompts, unreachable roles) to verify error reporting.
    *   **Focus:** Input parsing, prompt processing, worker invocation with specific roles, validation execution, and correct index updates.

*   **`internal/run/execution_supervisor.go`**:
    *   **Objective:** Enhance existing execution supervisor tests to validate complex dependency graphs, varied task types, and robust error handling during execution.
    *   **Scenarios:**
        *   Tasks with complex A->B->C dependencies, ensuring correct execution order.
        *   Parallel tasks that depend on a common upstream task.
        *   Tasks requiring different worker roles and verifying correct role assignment.
        *   Execution with tasks that modify the same files, verifying conflict detection and resolution mechanisms (if any are implemented at this layer).
        *   Tasks that timeout, ensuring proper termination and state updates.
        *   Tasks that exit with non-zero status, verifying retry logic and final failure state.
    *   **Focus:** Dependency resolution, task scheduling, worker management, state transitions, and error recovery.

*   **`internal/run/workstream_runner.go`**:
    *   **Objective:** Verify the correct invocation and lifecycle management of individual worker processes, including various command configurations and output capture.
    *   **Scenarios:**
        *   Workers executed with different command line arguments and environment variables.
        *   Workers producing large stdout/stderr outputs, ensuring they are captured correctly without truncation.
        *   Workers that run for extended periods and are correctly terminated on timeout.
        *   Workers that exit immediately with success or failure.
        *   Workers that create temporary files, ensuring proper cleanup.
    *   **Focus:** Process execution, I/O redirection, environment setup, signal handling (e.g., for timeouts), and exit code interpretation.

### 2. Git Operations and Workflow

*   **`internal/run/git_commit.go`**:
    *   **Objective:** Validate that changes are correctly staged, committed, and pushed according to Governator's workflow.
    *   **Scenarios:**
        *   A single task making changes, resulting in a single clean commit.
        *   Multiple tasks contributing to a single designated commit (if supported by workflow).
        *   Tasks creating separate commits with distinct messages.
        *   Commits occurring on different branches and later merged.
        *   Committing when the repository is dirty (e.g., uncommitted changes outside of Governator's scope), ensuring graceful handling or error.
    *   **Focus:** Git command execution, staging, commit message generation, and interaction with the remote.

*   **`internal/run/branch.go`**:
    *   **Objective:** Ensure correct branch management throughout the Governator lifecycle.
    *   **Scenarios:**
        *   Creating a new branch for a workstream and switching to it.
        *   Committing changes to a feature branch.
        *   Rebasing or merging changes from the base branch onto a feature branch.
        *   Deleting a workstream branch after successful merge.
        *   Handling cases where the base branch itself changes during a Governator run.
    *   **Focus:** Branch creation/deletion, checkout, merge/rebase operations, and tracking.

*   **`internal/run/merge.go`**:
    *   **Objective:** Verify the process of integrating workstream changes back into the main branch.
    *   **Scenarios:**
        *   A successful fast-forward merge.
        *   A successful merge requiring a merge commit.
        *   A merge with solvable conflicts, verifying that the system either resolves them automatically (if applicable) or flags for manual intervention.
        *   Merging a branch with no changes (noop merge).
        *   Merging when the target branch has diverged significantly.
    *   **Focus:** Git merge command execution, conflict detection, and resolution strategies.

*   **`internal/digests/drift.go` (integrated via `internal/run` components)**:
    *   **Objective:** Ensure that changes external to Governator (drift) are correctly detected and handled.
    *   **Scenarios:**
        *   Detecting manual changes to files tracked by Governator between runs.
        *   Detecting changes to files *not* explicitly tracked by Governator but still within the repo (e.g., source code).
        *   Verifying that detected drift triggers an appropriate response (e.g., re-evaluation, warning, blocking further action).
        *   Testing scenarios where drift is intentionally ignored or resolved.
    *   **Focus:** Hash computation, comparison logic, and system response to detected changes.

### 3. State Management & Configuration Effects

*   **`internal/config` (across `internal/run` components)**:
    *   **Objective:** Validate that various configuration settings correctly influence the end-to-end behavior of Governator.
    *   **Scenarios:**
        *   Different `concurrency` settings affecting parallel task execution.
        *   `timeouts` for workers correctly terminating long-running processes.
        *   `retries` policy leading to re-execution of failed tasks.
        *   `branch` configurations influencing branch naming and merge strategies.
        *   `reasoning_effort` settings (if they have observable e2e impact).
        *   Invalid configuration files, verifying graceful startup failure or error reporting.
    *   **Focus:** Configuration loading, propagation to execution components, and observable behavioral changes.

*   **`internal/run/phase_runner.go` / `internal/phase/phase.go`**:
    *   **Objective:** Verify the correct execution and transition between distinct phases of a Governator plan.
    *   **Scenarios:**
        *   Successful transition through multiple phases (e.g., PLANNING -> EXECUTION -> MERGE).
        *   A phase failing and preventing subsequent phases from starting.
        *   Skipping an optional phase based on conditions.
        *   Re-running a specific phase after a failure or manual intervention.
    *   **Focus:** Phase state machine transitions, pre- and post-phase hooks, and inter-phase dependencies.

### 4. Error Handling & Edge Cases

*   **General Error Resilience**:
    *   **Objective:** Ensure the system handles unexpected conditions, worker failures, and invalid inputs gracefully without crashing or corrupting state.
    *   **Scenarios:**
        *   Worker scripts exiting with non-zero codes, or being forcibly terminated (e.g., `kill -9`).
        *   Files expected by workers not existing or being inaccessible.
        *   Network failures during Git operations (pull/push).
        *   Malformed `planning.json` or `config.json` files.
        *   Empty repositories or repositories with no Governator configuration.
        *   Running Governator from an incorrect working directory.
    *   **Focus:** Robustness, error reporting, state preservation during failures, and recovery mechanisms.