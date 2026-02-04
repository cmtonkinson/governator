1. Inspect `_governator/index.json` and read the open tasks in `backlog` and
   `triaged`.
2. Create a DAG among those issues according to their inter-dependencies, using
   any existing dependencies as hints
3. Emit JSON like `{"task-07": ["task-03", "task-04"], "task-08": ["task-03",
   "task-06", "task-07"]}` where each key is a task id and the value is an
   array of its dependencies.
4. Write the DAG to `_governator/_local-state/dag.json` without markup,
   commentary, code fences, or any other formatting - the file should be a
   fully valid JSON document.
