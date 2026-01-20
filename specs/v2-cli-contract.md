<!--
CLI contract for Governator v2 commands, flags, output, and exit codes.
-->
# Governator v2 CLI Contract

## Goals
- Provide a deterministic, script-friendly command surface.
- Define exit codes for success, misuse, and failure.
- Describe minimal output expectations for automation and operators.

## Conventions
- All commands are non-interactive and must not prompt.
- Output is line-oriented, stable, and free of ANSI styling.
- Errors are concise and actionable.
- Paths are printed relative to the repo root where practical.

## Exit Codes
- `0`: success.
- `1`: execution failure (runtime error, failed task, failed bootstrap, etc.).
- `2`: misuse (unknown command, invalid args/flags, missing repo).

## Commands

### `governator init`
Initializes repo-local config and state directories.

**Synopsis**
```
governator init
```

**Behavior**
- Creates required `_governator/` durable state directories.
- Idempotent; re-running is safe.
- Fails with exit code `1` if filesystem operations fail.

**Output (example)**
```
init ok
```

### `governator plan`
Generates a full task index for the project.

**Synopsis**
```
governator plan
```

**Behavior**
- Ensures required bootstrap artifacts exist; if missing, runs bootstrap first.
- Emits a deterministic task index and task files.
- Fails with exit code `1` on planner/IO failures.

**Output (example)**
```
bootstrap ok
plan ok
```

### `governator run`
Executes eligible tasks until completion or blocking failure.

**Synopsis**
```
governator run
```

**Behavior**
- Resumes from the current index state; no separate resume command.
- Logs task starts, completions, failures, and timeouts to stdout.
- Uses deterministic task ordering per scheduler rules.
- Fails with exit code `1` on task failure or timeout.

**Output (example)**
```
task start id=T-014 role=tester
task timeout id=T-014 role=tester after=10m
run failed
```

### `governator status`
Prints a summary of current task state.

**Synopsis**
```
governator status
```

**Behavior**
- Reads the task index and prints a compact summary.
- Does not modify repo state.

**Output (example)**
```
tasks total=82 done=16 open=66 blocked=0
```

### `governator version`
Prints version and build metadata.

**Synopsis**
```
governator version
```

**Behavior**
- Prints version string and build metadata in a single line.

**Output (example)**
```
governator v2.0.0+build.123
```

## Unsupported or Missing Commands
- Missing command or unknown command is treated as misuse.
- The CLI prints a brief usage line and exits with code `2`.

**Output (example)**
```
usage: governator <init|plan|run|status|version>
```
