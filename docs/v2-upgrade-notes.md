# Governator v2 Upgrade Notes

These notes recap what operators and automation must adapt to when moving from
the v1 Governator experience to v2.

## Summary

- Governator v2 is a system-installed CLI (`governator init`, `plan`, `run`,
  `status`, `version`) whose state lives in `_governator/` and is driven by a
  single canonical task index. All orchestration is deterministic and auditable.
- Planning emits `_governator/task-index.<ext>` plus flat task files under
  `_governator/tasks/`, then `run` iterates tasks until completion. There are no
  longer nested state directories or opaque worktrees to inspect.
- Install and config handling now follows a layered model (user defaults,
  repo overrides, CLI flags) and the binary is shipped through Homebrew (macOS)
  or dpkg/apt (Ubuntu). Updates are handled by the package manager.

## Key behavioral differences from v1

1. **Canonical index replaces directories.** The only mutable state is the
   centralized task index (e.g., `_governator/task-index.json`) and the outputs
   under `_governator/_local_state/`. Task files under `_governator/tasks/` are
   read-only intent. You no longer move `work/` folders around to track progress.
2. **Explicit pipeline flow.** v2 always runs bootstrap → planning → execution,
   and every command exits deterministically. Tasks are eligible based on
   explicit dependencies and caps instead of implicit folder order.
3. **Single CLI entry point.** There are five deterministic commands, each
   returning stable exit codes (`0` success, `1` failure, `2` misuse) and plain
   text output for automation.
4. **Bootstrapped repo state.** Running `governator init` scaffolds the
   `_governator/_durable_state/` config, enforces repo discovery, and never
   mutates files outside of `_governator/` (aside from `~/.config/governator/`).
5. **State discovery and retries.** `run` reads the index, logs lifecycle events,
   preserves worktrees on failure, and creates follow-up tasks when it needs to
   resume; there is no automatic background retry loop this version.

## Install and config layering

- **System install**: Acquire the CLI from the platform package manager (Homebrew
  on macOS, dpkg/apt on Ubuntu). The binary lives in the standard `$PATH` and is
  updated via `brew upgrade governator` or `apt upgrade governator`.
- **Config layers**:
  - Operator defaults are stored in `~/.config/governator/`.
  - Per-repo overrides live in `_governator/config/` inside each repository.
  - CLI flags override both layers during invocation.
- **Repository requirements**: The CLI refuses to run outside a git repository.
  Any automation that previously targeted non-git paths must now initialize git
  first so `governator` can resolve the root and emit the durable state folders.

## Migration guidance

1. Install the v2 binary via your package manager and confirm `governator version`
   reports the desired release.
2. From your existing repo, run `governator init` to regenerate `_governator/`
   and related scaffolding. This will not overwrite upstream files but will ensure
   the durable state directories exist.
3. Re-run planning (`governator plan`). Planning writes the new centralized index
   and tasks, so your working tree will contain fresh intent files; discard any
   lingering v1 state directories before committing.
4. Use the new `plan` + `run` + `status` pattern for future work. Scripts that
   inspected old work directories should be updated to read `_governator/task-index.*`
   instead.

## Breaking considerations (sad paths to call out)

- There is no automated migration of v1 state. If you need history of prior
  work, preserve the old directories externally before switching to v2’s index
  model; otherwise the index starts from scratch and only records new lifecycle
  events.
- Any tooling that expected interactive prompts or ANSI-rich output must be
  rewritten because v2 is strictly non-interactive and line-oriented.
- The CLI enforces the git repo guard and index-based scheduling. If a script
  tried to drive Governator on arbitrary paths or relied on implicit retries,
  you must adjust it to call `governator init`, `plan`, and `run` explicitly.

## Additional references

- `docs/v2-cli.md` — the command reference for the deterministic surface.
- `docs/system-install-distribution.md` — platform-specific install and update plan.
- `GOVERNATOR.md` — the authoritative spec for v2’s behavior and audit model.
