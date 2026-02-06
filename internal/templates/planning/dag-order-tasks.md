1. Inspect `_governator/_local_state/index.json` and read the open tasks in
   `backlog` and `triaged`.
2. Create a DAG among those issues according to their inter-dependencies, using
   any existing dependencies as hints.
  Only note TRUE dependencies where:
  - Task Y needs output/results from Task X to proceed
  - Task Y extends or modifies code written by Task X
  - Task Y tests functionality implemented by Task X
  DO NOT create dependencies just because:
  - Tasks touch the same file (parallel edits can be merged)
  - Tasks are conceptually related, but independent
  - Tasks are in the same feature area but don't share code
  - You want a "clean" serial order
  Examples of PARALLEL patterns (NO dependencies):
  - Multiple bug fixes in different components
  - Adding tests for different existing features
  - Documentation updates for different modules
  - Refactoring separate, independent files
  Examples of TRUE dependencies:
  - task-005 "Add tests for new auth system" depends on task-003 "Implement
    auth system"
  - task-008 "Refactor auth to use new logger" depends on task-006 "Implement
    logger"
  - task-012 "Add UI for API endpoint" depends on task-010 "Create API
    endpoint"

3. Emit JSON like `{"task-07": ["task-03", "task-04"], "task-08": []}` where
   each key is a task id and the value is an array of its dependencies. Empty
   arrays indicate independent tasks that can run immediately in parallel.

4. Write the DAG to `_governator/_local-state/dag.json` without markup,
   commentary, code fences, or any other formatting
  - the file should be a fully valid JSON document
