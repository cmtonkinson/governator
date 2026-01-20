# Governator v2 Config JSON

Concise specification for `config.json` keys, defaults, and fallback behavior.

## Format
- JSON object; unknown keys are ignored.
- Best-effort parsing: invalid or missing values fall back to defaults.

## Keys
### Worker commands
`workers.commands.default` (array of strings)
- Command template used when a role-specific command is not present.
- Must include `{task_path}` to point at the task file.

`workers.commands.roles` (object of role -> array of strings)
- Optional per-role command template overrides.

Supported template tokens:
- `{task_path}`: absolute path to the task markdown file.
- `{repo_root}`: absolute path to the repo root.
- `{role}`: task role name.

### Concurrency caps
`concurrency.global` (number)
- Maximum total in-flight workers.

`concurrency.default_role` (number)
- Default cap when a role entry is missing.

`concurrency.roles` (object of role -> number)
- Optional per-role caps.

### Timeouts and retries
`timeouts.worker_seconds` (number)
- Worker execution timeout in seconds.

`retries.max_attempts` (number)
- Maximum total attempts per task before blocking.

### Auto-rerun guard
`auto_rerun.enabled` (boolean)
- Enables re-entry guard for `run`.

`auto_rerun.cooldown_seconds` (number)
- Minimum seconds between `run` invocations.

## Defaults and fallback behavior
- `workers.commands.default`: `["codex", "exec", "--sandbox=danger-full-access", "{task_path}"]`
- `workers.commands.roles`: `{}` (role falls back to `workers.commands.default`)
- `concurrency.global`: `1`
- `concurrency.default_role`: `1`
- `concurrency.roles`: `{}`
- `timeouts.worker_seconds`: `900`
- `retries.max_attempts`: `2`
- `auto_rerun.enabled`: `false`
- `auto_rerun.cooldown_seconds`: `60`

Missing or invalid values use the defaults above. If both the role command and
`workers.commands.default` are missing or invalid, worker command resolution
returns an error and the task is not started.

## Example config (defaults shown)
```json
{
  "workers": {
    "commands": {
      "default": [
        "codex",
        "exec",
        "--sandbox=danger-full-access",
        "{task_path}"
      ],
      "roles": {
        "architect": [
          "codex",
          "exec",
          "--sandbox=danger-full-access",
          "--role",
          "architect",
          "{task_path}"
        ]
      }
    }
  },
  "concurrency": {
    "global": 1,
    "default_role": 1,
    "roles": {
      "planner": 1
    }
  },
  "timeouts": {
    "worker_seconds": 900
  },
  "retries": {
    "max_attempts": 2
  },
  "auto_rerun": {
    "enabled": false,
    "cooldown_seconds": 60
  }
}
```
