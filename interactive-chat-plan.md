# Governator Clarification Chats — Final Combined Plan (Discovery + Refinement)

This document defines the canonical design for a Q&A “clarification” feature in
Governator: an **up-front Discovery command** (forced when no `GOVERNATOR.md`
exists) and an **optional Refinement chat** (requested via `--refinement`) that
occurs **after architecture + milestone/epic planning and before task
creation**.

The design is **file-backed**, **git-tracked**, and **provider-agnostic** via
existing CLI providers — but uses `screen` as the
interactive session runner and transcript capture mechanism.

---

## Goals

- **Discovery**

  - Forced on first run if `GOVERNATOR.md` does not exist.
  - Always begins with the literal question: **“What would you like to build?”**
  - Primary output: a generated `GOVERNATOR.md` file.
  - Additional outputs: full transcript file, plus a summary file.
  - Transcripts and summaries live under `_governator/docs/chat/`; naming
    conventions are left to implementation.

- **Refinement**

  - Optional, requested via `--refinement` passed to **any single command**
    (`init`, `discovery`, or `run`) before tasks are planned.
  - Occurs after:
    1. discovery (optional/forced)
    2. architecture bootstrap
    3. milestone & epic planning
  - And before:
    4. task planning
  - Primary output: alignment and validation of *all produced artifacts*
    (architecture + milestones/epics + constraints).
  - Additional outputs: full transcript file, plus a summary file.
  - Transcripts and summaries live under `_governator/docs/chat/`; naming
    conventions are left to implementation.

- **Workflow refactor prerequisite**

  - Milestone & epic planning must be **disentangled from task planning**.
  - No tasks may be created until refinement (if enabled) is complete.

- **Operator control**

  - Sessions can be ended explicitly by typing `/done`.
  - `screen` controls provide a hard-stop fallback.
  - System prompts instruct the model to emit the **DISCOVERY COMPLETE** or
    **REFINEMENT COMPLETE** markers when the operator has been satisfied.

---

## Non-Goals

- No mid-task interactive interrupts.
- No direct API integration or custom REPL.
- No hidden memory; all context is derived from repo files.
- No task creation before refinement (if enabled).

---

## User-Facing Behavior

### Discovery requirement

- `governator.sh run` **fails** if `GOVERNATOR.md` does not exist.
- The error message instructs the operator to:
  1. create `GOVERNATOR.md` manually, **or**
  2. run `governator.sh discovery`.
- Presence of `GOVERNATOR.md` is the discovery completion marker.

### Discovery command

- `governator.sh discovery`
  - launches an interactive clarification session via `screen`.
  - prints the literal question **“What would you like to build?”** before the
    agent starts.
  - captures a transcript under `_governator/docs/chat/` (naming conventions
    left to implementation).
  - produces a summary file under `_governator/docs/chat/` (naming conventions
    left to implementation).
  - emits `GOVERNATOR.md` from that summary (w/ transcript as context if needed).

### Refinement intent flag

- `--refinement` may be passed to **any one** of:

  - `governator.sh init --refinement`
  - `governator.sh discovery --refinement`
  - `governator.sh run --refinement`

- This flag sets project intent in `.governator/config.json` (sticky and
  idempotent).
- `refinement_requested` may be cleared once refinement is reviewed and
  confirmed.

- When the pipeline reaches the refinement point, Governator pauses
  automatically and instructs:

  - `governator.sh refinement`

- Refinement is allowed to modify architecture & planning docs (so long as it
  does not introduce internal inconsistencies).
- Refinement results are reviewed like other artifacts; if inconsistencies are
  found, refinement is rejected and another refinement session is required.
- Refinement review may be implemented as a special/system `000-` task with a
  predetermined prompt.
- Refinement review should follow the standard review flow, using the refined
  intent, architecture, and planning docs as input.
- Refinement review should be assigned/dispatched immediately after the
  refinement chat completes.
- Refinement review outputs follow existing review conventions (e.g.
  `review.json`).

---

## Canonical Phase Pipeline

1. Discovery (optional / forced if no GOVERNATOR.md)
2. Architecture bootstrap
3. Milestone & Epic planning
4. Refinement (optional, gated)
5. Task planning
6. Task execution

**Invariant:** If refinement is enabled, **no normal (non-000) task files may be
created** before refinement completes. (special/system 000-prefixed tasks are
allowed).

---

## Prerequisite Refactor: Milestone/Epic vs Task Planning

This clarification feature **depends on** an upfront pipeline split. Treat this
as a hard prerequisite. No migration or backward compatibility is required for
existing Governated repos; pretend they don't exist.

### Required changes

- Milestone & Epic planning must be split from task planning.

- Create a new **000- task to implement this refactor.**

- Split the current planner prompt into:

  1. milestone/epic planning prompt
  2. task planning prompt

- Extend the state machine with explicit markers:

  - `architecture_complete`
  - `epics_complete`
  - `refinement_complete` (if enabled)
  - `tasks_planned`

---

## Configuration Schema

```jsonc
{
  "state": {
    "architecture_complete": "",
    "epics_complete": "",
    "refinement_complete": "",
    "tasks_planned": "",
    "refinement_requested": false
  }
}
```

---

## Commands

### `governator.sh discovery`

- Runs a `screen` session with the configured provider CLI.
- Focuses on intent, requirements, constraints, and assumptions.
- Ends when the agent emits **DISCOVERY COMPLETE** or the operator types
  `/done`.
- `/done` detection and the overall operator-termination mechanism remain open
  questions; defer implementation details.

### `governator.sh refinement`

- Runs a `screen` session reacting to:
  - `GOVERNATOR.md`
  - architecture docs
  - milestone & epic plans
- Drives convergence and alignment.
- Ends when the agent emits **REFINEMENT COMPLETE** or the operator types
  `/done`.
- `/done` detection and the overall operator-termination mechanism remain open
  questions; defer implementation details.

---

## Dependencies

- `screen` is a required dependency and must be documented in `README.md`.

---

## Open Questions

- `/done` termination: define whether/how to detect `/done` from `screen`
  output and what the operator termination UX should be if `/done` is not
  supported.

---

## Acceptance Criteria

- On a fresh repo with no `GOVERNATOR.md`, `run` fails with clear guidance.
- `governator.sh discovery` generates a valid `GOVERNATOR.md`.
- Passing `--refinement` blocks task planning until refinement completes.
- No normal tasks are created before refinement when refinement is enabled.
- All chats are transcripted and summarized under `_governator/docs/chat/`.
