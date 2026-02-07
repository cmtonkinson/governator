# Unit Test Coverage Opportunities

This document outlines opportunities for adding or improving unit test coverage in the codebase.

## Internal

### `internal/config/model.go`

- **Opportunity:** This file defines the main configuration structs and has several helper functions that lack unit tests.
- **Suggestions:**
    - **`TestBuiltInCommand`**: Create a test to verify that `BuiltInCommand` returns the correct command templates for all supported CLIs (`codex`, `claude`, `gemini`) and that it returns `false` for an unknown CLI.
    - **`TestIsValidCLI`**: Test `IsValidCLI` with both valid and invalid CLI names to ensure it correctly identifies them.
    - **`TestReasoningEffortConfig_LevelForRole`**: Test the `LevelForRole` method to ensure it returns the correct reasoning effort level. Cases should include:
        - A role with a specific level defined.
        - A role that falls back to the default level.
        - A configuration with an empty or whitespace default level, which should fall back to `DefaultReasoningEffort`.
        - A role with an empty or whitespace level.

### `internal/phase/validation.go`

- **Opportunity:** This file contains logic for validating prerequisites for different phases of the process. The validation functions, which interact with the file system, are untested.
- **Suggestions:**
    - Use a test helper to create a temporary directory structure with mock files to simulate different scenarios.
    - **`TestValidatePrerequisites`**: Test this function for each phase (`PhaseGapAnalysis`, `PhaseProjectPlanning`, `PhaseTaskPlanning`, `PhaseExecution`) to ensure it calls the correct validation functions and aggregates the results.
    - **`TestValidateArchitectureArtifacts`**: Test with both required artifacts present and missing.
    - **`TestValidateGapReport`**: Test the different possible locations for the gap report and the case where it is missing.
    - **`TestValidateRoadmapArtifacts`**: Test with `milestones.md` and `epics.md` present and missing.
    - **`TestValidateTaskBacklog`**: Test with an empty `_governator/tasks` directory, a directory with empty files, and a directory with valid task files.
    - **`TestFileExists`**: Test `fileExists` with a path to a file, a directory, and a non-existent path.
    - **`TestRelativePath`**: Add tests for the `relativePath` helper to cover different root and target path combinations.

### `internal/placeholder/`

- **File:** `internal/placeholder/placeholder.go`
- **Opportunity:** This directory is currently empty. If it is intended to have functionality, it needs both implementation and corresponding tests.

### `internal/run/agent_audit.go`

- **Opportunity:** This file contains helper functions for logging agent events. These functions have logic that is not covered by unit tests.
- **Suggestions:**
    - Create a mock `AgentAuditor` to capture the calls to its methods.
    - **`TestAgentNameForStage`**: Test that `agentNameForStage` returns the correct agent name for each `roles.Stage` and a default name for unknown stages.
    - **`TestLogAgentInvoke`**:
        - Test that `auditor.LogAgentInvoke` is called with the correct parameters.
        - Test that the function handles a `nil` auditor gracefully.
        - Test the cases where `taskID` or `role` are empty.
        - Test that `attempt` is defaulted to 1 if it's less than 1.
    - **`TestLogAgentOutcome`**:
        - Similar to `TestLogAgentInvoke`, test that `auditor.LogAgentOutcome` is called with the correct parameters.
        - Test the `nil` auditor, empty `taskID`, and empty `role` cases.
        - Test that `status` is defaulted to "unknown" if it's empty.
    - **`TestStatusFromIngestResult`**: Test for all combinations of `result.TimedOut` and `result.Success`.
    - **`TestExitCodeForOutcome`**: Test that the exit code is correctly set to -1 for a timeout.

### `internal/run/async.go`

- **Opportunity:** This file has several helpers for managing asynchronous workers that are untested.
- **Suggestions:**
    - **`TestInFlightMap`**: Test with both an empty and a populated `inflight.Set`.
    - **`TestAdjustCapsForInFlight`**: This is a key function for controlling concurrency. Tests should cover:
        - An empty in-flight set (caps should be unchanged).
        - Reduction of the global cap.
        - Reduction of role-specific caps.
        - Reduction of the default role cap.
        - A scenario where in-flight tasks exceed a cap, ensuring the cap becomes 0 and not negative.
    - **`TestBuildInFlightRoleCounts`**: Test that tasks from the `inflight.Set` are correctly aggregated by their role from the `index.Index`.
    - **`TestMaxInt`**: Test with `left > right`, `right > left`, and `left == right`.
    - **`TestTimedOut`**: Test the timeout logic with various scenarios:
        - `timeoutSecs <= 0` (should always be false).
        - `startedAt` is zero (should always be false).
        - Elapsed time is less than the timeout.
        - Elapsed time is greater than the timeout.
    - **`TestStartedAtForTask`**: Test for a task that is in the set and one that is not.
    - **`TestWorktreePathForTask`**: Test for a task that is in the set and one that is not.
    - **`TestFormatTimeoutReason`**: Check that the output string is formatted as expected.

### `internal/run/branch_name.go`

- **Opportunity:** The `TaskBranchName` function is simple, but a test ensures its behavior is locked in.
- **Suggestions:**
    - **`TestTaskBranchName`**: Create a sample `index.Task` and assert that the function returns the task's `ID`.

### `internal/run/dispatch_state.go`

- **Opportunity:** The `readDispatchWrapperPID` function reads and parses a file from the filesystem and is untested.
- **Suggestions:**
    - Use a test helper to create a temporary directory and `dispatch.json` file.
    - **`TestReadDispatchWrapperPID`**: Test the following cases:
        - The `workerStateDir` path is empty.
        - The `dispatch.json` file does not exist.
        - The `dispatch.json` file contains invalid JSON.
        - The `dispatch.json` file is valid but `wrapper_pid` is missing or `<= 0`.
        - A valid `dispatch.json` file with a `wrapper_pid`.

### `internal/run/execute.go`

- **Opportunity:** The `Execute` function is the entrypoint for the execution phase and lacks unit tests.
- **Suggestions:**
    - **`TestExecute`**:
        - Test the guard clauses: empty `repoRoot`, `nil` `Stdout`, `nil `Stderr`.
        - Mock `index.Load` to return an error.
        - Mock `newPlanningTask` to return an error.
        - Mock `planningComplete` to return `false` or an error.
        - Mock a successful run where `Run` is called, and verify that `Run` is called with the correct parameters.

### `internal/run/execution_controller.go`

- **Opportunity:** This file defines the execution workstream controller, a key part of the orchestration logic, and it is untested.
- **Suggestions:**
    - **`TestNewExecutionController`**: Verify that the controller is initialized with the correct values.
    - **`TestExecutionController_CurrentStep`**: Test that the function returns the correct step and continuation flag.
    - **`TestExecutionController_Dispatch`**: This is a complex function requiring extensive testing:
        - For each execution stage (`work`, `test`, `review`, `resolve`, `merge`), mock the corresponding `Execute...Stage` function.
        - Verify that the correct `Execute...Stage` function is called for each step.
        - Verify that the results from the stage functions are correctly stored in the controller.
        - Verify that `inFlightWasUpdated` and `result.Handled` are set correctly based on the stage results.
        - Verify that `worktreeOverrides` is updated after the `work` stage.
        - Test error handling for `stageForStep` and unknown stages.
    - **`TestExecutionController_stageForStep`**: Test with valid, empty, and unknown step names.

### `internal/run/execution_supervisor_control.go`

- **Opportunity:** This file contains logic for stopping the execution supervisor and its workers, which involves filesystem and process operations.
- **Suggestions:**
    - **`TestStopExecutionSupervisor`**:
        - Test the `repoRoot` guard clause.
        - Mock `supervisor.LoadExecutionState` to return various states (error, not running, running).
        - Mock `supervisor.ExecutionSupervisorRunning` to return various states (error, not running).
        - When `opts.StopWorker` is true, mock `stopExecutionWorkers` and test its error case.
        - Mock `TerminateProcess` and test its error case.
        - Verify that `supervisor.SaveExecutionState` is called with the correct final state.
    - **`TestStopExecutionWorkers`**:
        - Mock `inflight.NewStore` and `store.Load` to test their error cases.
        - Create a mock `inflight.Set` and verify that `readDispatchWrapperPID` and `killWorkerProcess` are called for each worker.

### `internal/run/planning_controller.go`

- **Opportunity:** This file defines the planning workstream controller and has complex logic that is untested.
- **Suggestions:**
    - **`TestPlanningController_CurrentStep`**: Test the error and success cases, especially the behavior of `currentPlanningStep`.
    - **`TestPlanningController_Collect`**:
        - Test the various return paths (not in-flight, still running, error cases).
        - Mock `resolvePlanningPaths`, `runningPlanningPID`, `collectPhaseCompletion`, `inFlight.Remove`, and `persistInFlight` to test their error handling.
        - Test a successful collection.
    - **`TestPlanningController_Advance`**:
        - Test the case where `collect.Completed` is false.
        - Mock `validationEngine.RunValidations` to return an error or a failed validation.
        - Mock `runner.completePhase` to return an error.
        - Test the final planning step logic, including mocking `taskInventory.InventoryTasks`, `index.Save`, and `persistPlanningState` for both success and error cases.
        - Test the non-final planning step logic, mocking `persistPlanningState`.
    - **`TestPlanningController_Dispatch`**: Mock `runner.dispatchPhase` to test the error and success cases.
    - **`TestPlanningController_reloadIndex`**: Test error and success cases for `index.Load`.
    - **`TestPlanningController_persistPlanningState`**: Mock `index.Load`, `digests.Compute`, and `index.Save` to test error and success cases.

### `internal/run/planning_index.go`

- **Opportunity:** This file handles the creation and updating of the planning index on the filesystem and in git.
- **Suggestions:**
    - **`TestSeedPlanningIndex`**:
        - Test the `repoRoot` guard clause.
        - Test when the index file already exists.
        - Mock `os.Stat`, `LoadPlanningSpec`, `digests.Compute`, and `index.Save` to test error handling.
    - **`TestUpdatePlanningIndex`**:
        - Test the `worktreePath` guard clause.
        - Mock `index.Load` to handle the `os.ErrNotExist` case and other errors.
        - Mock `digests.Compute`, `index.Save`, and `commitPlanningIndex` for error handling.
    - **`TestCommitPlanningIndex`**:
        - Mock `runGitOutput` to simulate no changes.
        - Mock `runGit` and `runGitWithEnv` to test error handling during git add and commit.

### `internal/run/planning_spec.go`

- **Opportunity:** This file contains the parsing and validation logic for `planning.json`.
- **Suggestions:**
    - **`TestLoadPlanningSpec`**: Mock `os.ReadFile` and `ParsePlanningSpec` to test error handling.
    - **`TestParsePlanningSpec`**: Test with invalid JSON, trailing data, and specs that fail validation.
    - **`TestValidatePlanningSpec`**: Write tests to cover all validation rules (version, steps exist, unique IDs, required fields, etc.).
    - **`TestValidatePlanningValidations`**: Test each validation type (`command`, `file`, `directory`, `prompt`) and its specific field requirements and rejections.
    - **`TestPlanningTaskFromSpec`**: Verify that a valid spec is correctly transformed into a `planningTask`.
    - **`TestNormalizePlanningPromptPath`**: Test various invalid paths (empty, absolute, containing `..` or `\`).
    - **`TestValidatePlanningStepID`**: Test various invalid IDs (empty, containing `/`, `..`).

### `internal/run/planning_supervisor_control.go`

- **Opportunity:** This file has logic for stopping the planning supervisor, which is similar to the execution supervisor but with its own state and worker management.
- **Suggestions:**
    - **`TestStopPlanningSupervisor`**:
        - Test the `repoRoot` guard clause.
        - Mock `supervisor.LoadPlanningState` to return various states (error, not running, running).
        - Mock `supervisor.PlanningSupervisorRunning` to return various states (error, not running).
        - When `opts.StopWorker` is true, mock `stopPlanningWorker` and test its error case.
        - Mock `TerminateProcess` and test its error case.
        - Verify that `supervisor.SavePlanningState` is called with the correct final state.
    - **`TestStopPlanningWorker`**:
        - Test the case where `workerStateDir` is already present in the state.
        - Test the case where `workerStateDir` must be retrieved from the inflight store, and mock `inflight.NewStore` and `store.Load` for error handling.
        - Verify that `killWorkerProcess` is called with the correct parameters.

### `internal/run/role_assignment_invoker.go`

- **Opportunity:** This file invokes an LLM for role assignment, involving file I/O and command execution.
- **Suggestions:**
    - **`TestNewWorkerCommandInvoker`**: Test the `repoRoot` guard clause and that the timeout is set correctly.
    - **`TestWorkerCommandInvoker_Invoke`**:
        - Test the `prompt` guard clause.
        - Mock `writePrompt`, `worker.ResolveCommand`, and `exec.CommandContext` to test error handling.
        - Test the timeout logic.
        - Test a successful invocation and verify the command's output is returned.
        - Test cases where the command produces stderr, fails, or returns empty output.
    - **`TestWorkerCommandInvoker_writePrompt`**: Mock `os.MkdirAll` and `os.WriteFile` to test error handling, and verify the prompt content is written correctly.

### `internal/run/stage_input.go`

- **Opportunity:** This file prepares the input for a worker stage.
- **Suggestions:**
    - **`TestNewWorkerStageInput`**:
        - Mock `worker.IsCodexCommand` to test both true, false, and error cases.
        - Verify that all fields of `worker.StageInput` are populated correctly based on the inputs.
    - **`TestWorkerStateDirName`**: Test different combinations of attempt, stage, and role, including empty and invalid values.
    - **`TestSanitizeComponent`**: Test with various strings containing spaces, underscores, and mixed casing to ensure they are sanitized correctly.

### `internal/run/step_phase.go`

- **Opportunity:** This file contains a simple mapping from a step name to a `phase.Phase`.
- **Suggestions:**
    - **`TestStepToPhase`**: Test each defined step name to ensure it maps to the correct phase, and test that an unknown step name maps to `phase.PhaseNew`.

### `internal/run/supervisor_control_helpers.go`

- **Opportunity:** This file provides helper functions for supervisor control.
- **Suggestions:**
    - **`TestSupervisorStateEqual`**: Test with both equal and non-equal `SupervisorStateInfo` structs to ensure all fields are compared.
    - **`TestMarkSupervisorTransition`**: Verify that the `LastTransition` field is updated.
    - **`TestTerminateProcess`**:
        - Test with a PID of 0 or less.
        - Mock `os.FindProcess` to return an error.
        - Mock `proc.Signal` to return `syscall.ESRCH` (should be treated as success) and other errors.

### `internal/run/worker_kill.go`

- **Opportunity:** This file contains logic for killing worker processes.
- **Suggestions:**
    - **`TestKillWorkerProcess`**:
        - Mock `resolveAgentPID` to return a PID and verify `killPID` is called with it.
        - Mock `resolveAgentPID` to return `false` and verify `killPID` is called with the wrapper PID.
    - **`TestResolveAgentPID`**:
        - Test with an empty `workerStateDir`.
        - Mock `worker.ReadAgentPID` to return an error, and to return a PID after a delay.
        - Test the timeout case.
    - **`TestKillPID`**: Test with an invalid PID, and mock `os.FindProcess` and `proc.Signal` for error handling.

### `internal/run/workstream_runner.go`

- **Opportunity:** This file contains the generic workstream runner logic.
- **Suggestions:**
    - **`TestWorkstreamRunner_Run`**:
        - Use a mock `workstreamController` to test the runner's loop.
        - Test the `nil` controller case.
        - Mock `controller.CurrentStep` to return an error or `false` to terminate the loop.
        - Mock `controller.Collect` to return an error or a result with running PIDs.
        - Mock `controller.Advance` to return an error or `true` to test the continue logic.
        - Mock `controller.GateBeforeDispatch` and `controller.Dispatch` to test their error handling and the `Continue` flag.

### `internal/run/workstream.go`

- **Opportunity:** This file defines the basic workstream abstractions.
- **Suggestions:**
    - **`TestWorkstreamStep_title`**: Test that the `displayName` is used for the title if present, otherwise the `name` is used.

### `internal/scheduler/routing.go`

- **Opportunity:** This file contains the logic for selecting which tasks to run based on concurrency caps and overlap constraints. The existing tests could be improved.
- **Suggestions:**
    - **`TestRouteEligibleTasks`**:
        - Mock `OrderedEligibleTasks` to return an error.
        - Verify that `RouteOrderedTasks` is called with the result from `OrderedEligibleTasks`.
    - **`TestRouteOrderedTasks`**:
        - Test with a global cap of 0 or an empty task list.
        - Test that tasks are selected until the global cap is reached.
        - Test that tasks are skipped if their role cap is reached.
        - Test that tasks are skipped if their role cap is zero.
        - Test that tasks are skipped due to overlap conflicts.
        - Verify that the `Reason` in each `RoutingDecision` is correct for each case.
    - **`TestOverlapConflict`**: Test with and without overlap tags on the task, and with and without conflicts in the active set.
    - **`TestRecordOverlap`**: Verify that all of a task's overlap tags are added to the active set.

### `internal/templates/`

- **File:** `internal/templates/templates.go`
- **Opportunity:** This directory is currently empty. If it is intended to have functionality, it needs both implementation and corresponding tests.

### `internal/tui/`

- **File:** `internal/tui/tui.go`
- **Opportunity:** This directory is currently empty. If it is intended to have functionality, it needs both implementation and corresponding tests.

### `internal/worker/completion.go`

- **Opportunity:** This file contains logic to determine if a worker stage is complete.
- **Suggestions:**
    - **`TestCheckStageCompletion`**:
        - Test the guard clauses for `worktreePath` and `workerStateDir`.
        - Test with an invalid stage.
        - Mock `checkForCommit` and `checkForMarkerFile` to return errors and different combinations of true/false.
    - **`TestCompletionResultToIngest`**:
        - Test the guard clauses for `taskID` and `stage`.
        - Test both the `completion.Completed == true` and `completion.Completed == false` cases, verifying the resulting `IngestResult` is correct.

### `internal/worker/`

- **File:** `internal/worker/worker.go`
- **Opportunity:** This directory is currently empty. If it is intended to have functionality, it needs both implementation and corresponding tests.

### `internal/worker/pidfile.go`

- **Opportunity:** This file contains helpers for reading PID files.
- **Suggestions:**
    - **`TestReadAgentPID`**:
        - Test the `workerStateDir` guard clause.
        - Mock `readPIDFile` to return an error.
        - Test with no PID files, a valid PID file, and a PID file with invalid content to ensure the loop works correctly.
    - **`TestReadPIDFile`**:
        - Mock `os.ReadFile` to test `os.ErrNotExist` and other error cases.
        - Test with an empty file, a non-numeric file, a file with a zero PID, and a valid PID.

### `internal/worker/reasoning.go`

- **Opportunity:** This file contains logic for handling different reasoning levels for AI workers.
- **Suggestions:**
    - **`TestNeedsReasoningSupport`**: Test with "high", "low", and other strings.
    - **`TestShouldIncludeReasoningPrompt`**: Test all combinations of `level` and `agentUsesCodex`.
    - **`TestApplyCodexReasoningFlag`**: Test with a non-codex command, a level that doesn't need reasoning, and a successful application of the flag.
    - **`TestIsCodexExecutablePath`**: Test with various executable paths.
    - **`TestIsCodexCommand`**: Mock `selectCommandTemplate` to test error handling, and test with both codex and non-codex command templates.

### `internal/worktree/`

- **File:** `internal/worktree/worktree.go`
- **Opportunity:** This directory is currently empty. If it is intended to have functionality, it needs both implementation and corresponding tests.
