# Codex Chat Mode Implementation Plan

This document summarizes the strategy, architecture, design, and implementation decisions for adding an interactive planning chat mode that pauses after architecture bootstrap and resumes normal planning only after the chat completes. It is written to guide a clean-context agent to implement the feature in the main repo.

## Strategy
- Add an interactive, operator-driven chat phase that runs after the architecture bootstrap completes but before gap-analysis planning tasks are created.
- Keep Governator deterministic and auditable by persisting chat transcripts and state to disk and git.
- Make chat opt-in via a flag on `init` or `run` (first run), and use a new `chat` subcommand to run the interactive session.
- Maintain the existing “no shared memory” principle by using prompt files and stored artifacts only; do not keep conversational state in memory.

## Architecture
- Introduce a planning chat gate that blocks `assign_pending_tasks` after bootstrap completion.
- Add a new interactive command (`governator.sh chat`) that runs a provider-specific chat session with a TTY, logs the transcript, and marks the gate complete.
- Store all chat state in `.governator/config.json` under `planning` to keep it versioned, deterministic, and auditable.

## Design Decisions
### 1) Config additions
- Extend `_governator/templates/config.json`:
  - `planning.chat_mode`: string, values `off` (default), `bootstrap` (gate enabled), `complete` (chat completed).
  - `planning.chat_completed_at`: string timestamp; empty when chat not completed.
- Add per-provider chat arguments:
  - `agents.providers.<provider>.chat_args` array for interactive mode.
  - Keep `args` for non-interactive worker execution.

### 2) Gate behavior
- Gate triggers when:
  - Architecture bootstrap is complete AND
  - `planning.chat_mode == "bootstrap"`.
- Gate blocks all task assignment (including gap-analysis planner creation).
- Gate is only lifted after `governator.sh chat` completes successfully and updates config.

### 3) Chat prompt and context
- Provide a dedicated chat prompt template at `_governator/templates/chat-architecture.md`.
- Construct the chat prompt as a list of files to read, similar to worker prompts:
  - `chat-architecture.md`
  - `_governator/worker-contract.md`
  - `_governator/roles/architect.md`
  - `_governator/custom-prompts/_global.md`
  - `_governator/custom-prompts/architect.md`
  - `GOVERNATOR.md`
  - All `_governator/docs/*.md` except chat logs

### 4) Interactive chat execution
- Use a TTY-allocating wrapper (`script`) to run interactive providers and capture a transcript.
- Store transcripts in `_governator/docs/chat/transcript-<timestamp>.log`.
- If `script` is not available, fall back to `tee` and log a warning (output-only capture).
- Do not rely on network access in tests; use provider CLI binaries as configured in `config.json`.

### 5) CLI integration
- `governator.sh init --chat-on-bootstrap` sets the chat gate in config.
- `governator.sh run --chat-on-bootstrap` sets the chat gate and proceeds with normal run; after bootstrap completes, assignment will pause.
- `governator.sh chat` runs the interactive session and marks the gate complete.

## Implementation Outline (file-level)

### A) New template
- Add `_governator/templates/chat-architecture.md`:
  - Short instructions: ask clarifying questions, resolve conflicts, avoid inventing requirements, end with summary/checklist.

### B) Config utilities (`_governator/lib/config.sh`)
- Add getters/setters:
  - `read_planning_chat_mode`, `write_planning_chat_mode`
  - `read_planning_chat_completed_at`, `write_planning_chat_completed_at`
- Add `read_agent_provider_chat_args` to read `agents.providers.<provider>.chat_args`, and error if missing for provider.
- Extend `init_governator` to parse `--chat-on-bootstrap` and set `planning.chat_mode` to `bootstrap`.

### C) New chat library (`_governator/lib/chat.sh`)
Implement these functions:
- `planning_chat_pending`: returns true when bootstrap complete AND chat_mode is `bootstrap`.
- `enable_planning_chat_on_bootstrap`: sets chat_mode to `bootstrap` and clears completion timestamp.
- `build_chat_prompt`: builds the “read these files in order” prompt, excludes chat logs directory.
- `build_chat_command`: uses `read_agent_provider`, `read_agent_provider_bin`, and `read_agent_provider_chat_args` to create the command; substitute `{REASONING_EFFORT}` like worker commands.
- `run_chat_session`: runs the interactive CLI via `script` with transcript capture.
- `run_planning_chat`: gatekeeper for `governator.sh chat`:
  - Ensure clean git, dependencies, config, and doc presence.
  - Respect system lock (`handle_locked_state`).
  - Ensure bootstrap is complete and chat_mode is `bootstrap`.
  - Run chat session and update config (`chat_mode=complete`, `chat_completed_at=timestamp`).
  - Commit transcript and config updates, and push default branch.

### D) Queue gating (`_governator/lib/queues.sh`)
- In `assign_pending_tasks`, after `bootstrap_gate_allows_assignment` and before planner task creation, check `planning_chat_pending` and return early with a verbose log.

### E) CLI wiring
- In `_governator/governator.sh`:
  - Add global `RUN_CHAT_ON_BOOTSTRAP=0`.
  - Add chat constants:
    - `CHAT_ROLE="architect"`
    - `CHAT_PROMPT_TEMPLATE="${TEMPLATES_DIR}/chat-architecture.md"`
    - `CHAT_DOCS_DIR="${STATE_DIR}/docs/chat"`
  - `source "${LIB_DIR}/chat.sh"`.
  - In `main`, if `RUN_CHAT_ON_BOOTSTRAP==1`, call `enable_planning_chat_on_bootstrap` before `handle_locked_state`.
- In `_governator/lib/internal.sh`:
  - Update `parse_run_args` to accept `--chat-on-bootstrap` and set `RUN_CHAT_ON_BOOTSTRAP=1`.
  - Add a `chat` subcommand that calls `run_planning_chat`.
  - Update help text to mention `chat` and `run --chat-on-bootstrap`.

### F) README update
- Add a short paragraph in the existing Configuration section describing:
  - `init --chat-on-bootstrap`
  - `run --chat-on-bootstrap`
  - `chat` command before planning proceeds
- Do not add new README sections; insert within existing text.

## Behavioral Notes
- Chat is opt-in and only blocks planning when explicitly enabled.
- Gate does not affect pre-bootstrap operations; bootstrap tasks are still assigned normally.
- `chat` command is non-detached and runs interactively; it respects the global lock.
- Transcript and config changes are committed and pushed for auditability.

## Testing Guidance
- Run `scripts/all-tests.sh` after changes.
- No existing tests should break; consider adding tests for:
  - `planning_chat_pending` gating behavior.
  - `run --chat-on-bootstrap` toggling config.
  - `chat` command behavior when bootstrap incomplete or chat disabled.
- Avoid invoking real provider CLIs in tests; stub commands if necessary.

## Reasonable Defaults
- Default provider is `codex`.
- `codex.chat_args` should use interactive mode (e.g., `codex chat`) with the same reasoning effort substitution and sandbox flags as desired.
- `claude` and `gemini` can set `chat_args` to empty arrays if their interactive usage is provider-specific or not yet supported.
