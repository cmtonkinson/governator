# Gap Analysis

## Objective
Assess the current state of project code (if any) against documented
intent/designs/requirements/constraints/assumptions and generate a normalized
Decision Ledger, Gap Register, and Planning Constraints.

Your task is to empower project planning by analyzing what decisions are already
locked, what information is missing/ambiguous/conflicting, and what rules must
be observed while synthesizing the project plan. Your deliverables provide the
necessary scaffolding for planning such that risks and unknowns are documented
(and can be mitigated) as early as possible.

## Inputs
You must examine:
- `GOVERNATOR.md`
- All architectural artifacts in `_governator/docs/arch-*.md` and
  `_governator/docs/adr/`
- All plans in `_governator/docs/plan-*.md`
- Existing project state: code and config

## Outputs
1. `_governator/docs/gap-decision-ledger.md`
2. `_governator/docs/gap-register.md`
3. `_governator/docs/gap-planning-constraints.md`

## Analysis Sections
### Decision Ledger
Write the Decision Ledger to `_governator/docs/gap-decision-ledger.md`.

Enumerate what decisions are already locked-in (by spec, existing code, existing
Power Six artifacts, or ADRs), what decisions are implied but not explicitly
recorded, and what decisions must be made before planning can proceed safely.
Your output must reduce uncertainty for planners and parallel implementers.

Rules and posture:
- Your job is not to propose a new architecture. You are normalizing "what is
  already decided" vs "what is not decided."
- If an architecturally significant decision is present but not recorded in an
  ADR, you must flag it as "ADR missing" and recommend emitting an ADR (do not
  write the ADR here).
- Prefer grounded statements. If you cannot support a claim from inputs, mark it
  as "Unverified."
- If there is conflict between implementation and GOVERNATOR.md, treat
  GOVERNATOR.md as the source of truth and record the conflict as a decision,
  constraint, or gap as approptiate.

Produce the Decision Ledger as a table-like list (markdown) where each entry
includes:
- Decision ID (stable, short, e.g. DL-001)
- Decision statement (one sentence)
- Status (Locked / Provisional / Unverified)
- Source(s) (exact file paths; cite ADR IDs when applicable)
- Rationale (brief; may quote intent from sources, but do not paste long
  excerpts)
- Impacted areas (components, workflows, commands, data, interfaces)
- Follow-ups (if Provisional/Unverified, what must be clarified; if "ADR
  missing," say so)

The Decision Ledger must be exhaustive with respect to architecturally
significant constraints that will materially affect planning, especially:
- language/runtime/tooling choices and constraints
- repository/project layout conventions
- task model, state machine, and persistence expectations
- agent orchestration model (parallelism, isolation, permissions)
- interfaces/contracts (CLI, config schema, templates, storage)
- quality gates (tests, CI, lint, security posture)

### Gap Register
Write the Gap Register to `_governator/docs/gap-register.md`.

Enumerate missing, ambiguous, conflicting, or risky information that prevents
accurate milestone/task planning, then normalize it into actionable "gaps" with
recommended closure strategy. Your output is the backlog of uncertainty, not
implementation work.

Rules and posture:
- Every gap must be phrased as a falsifiable deficiency ("We do not know X" / "X
  conflicts with Y" / "X is underspecified such that multiple incompatible
  implementations exist").
- Do not invent requirements. If you infer, label it explicitly as inference and
  record it as a gap unless it is directly stated in sources.
- Tie each gap to the planning harm it causes (what it blocks, what it could
  derail, what it risks).
- If a gap should be resolved via ADR, say so. If it should be resolved via spec
  update, say so. If it should be resolved via code discovery, say so.

Produce the Gap Register as markdown entries where each gap includes:
- Gap ID (GR-001, ...)
- Category (Spec ambiguity / Architecture missing / ADR missing / Implementation
  drift / Dependency unknown / Testability gap / Operational gap /
  Security/compliance gap / Performance/scalability gap)
- Statement of gap (one to two sentences)
- Evidence (what you looked at and what it said; cite file paths)
- Impact on planning (what you cannot safely plan, or what would be high-risk)
- Severity (High/Med/Low) and Confidence (High/Med/Low)
- Closure recommendation (Decision task, ADR, spec edit, discovery spike,
  prototype, benchmark, etc.)
- Suggested owner role (Architect / Planner / Implementer / Test / DevOps)
- "Earliest point of resolution" (Before milestone planning / Before
  implementation / Before release / Continuous)

The Gap Register should prioritize gaps that would cause cascading rework in a
parallel agent environment, including: unclear ownership boundaries, unclear
public interfaces, unclear acceptance criteria, and unclear constraints that
affect many tasks.

### Planning Constraints
Write Planning Constraints to `_governator/docs/gap-planning-constraints.md`.

Produce a normalized, enforceable set of rules that the planner must follow when
generating milestones/epics/tasks and when scheduling parallel agents. This is
the "compiler flags" for planning: constraints, invariants, and guardrails that
help prevent plan hallucination, uncontrolled scope, merge conflicts, and unsafe
parallelism.

Rules and posture:
- Constraints must be actionable and testable. Avoid vague guidance.
- Separate "hard constraints" (must not violate) from "soft constraints" (strong
  preference).
- Constraints must explicitly address parallel agent execution and repository
  hygiene (ownership, boundaries, CI gates).
- Do not propose new constraints that contradict locked decisions; if you
  believe a constraint is necessary but absent, record it as a gap (and
  optionally list it under "Proposed constraints pending decision").

Produce Planning Constraints as markdown with three subsections:
1. Hard constraints (must): Each item includes: PCH-### ID, rule statement,
scope (planning / execution / codebase), enforcement mechanism
(lint/test/CI/human review), and source.
2. Soft constraints (should): Same fields except: "PCS-### ID, clearly marked
"should," plus rationale.
3. Assumptions planners may rely on: Only assumptions that are explicitly
supported by sources or by existing code reality. Each includes: PCA-### ID,
assumption, source, and "breaks if false" description.

Your Planning Constraints must cover, at minimum:
- artifact locations and naming conventions planners must emit
- interface-first planning rules (contracts/tests before implementations when
  appropriate)
- task sizing constraints (max diff size, single-owner write-set, bounded scope)
- dependency declaration rules (no task without explicit prerequisites)
- parallel safety rules (file/directory ownership, merge conflict avoidance,
  serialization where necessary)
- quality gates (tests required, CI required, formatting/lint/security checks)
- decision hygiene (when an ADR is required; how to handle provisional
  decisions)
- "spec vs code" precedence rules (what wins on conflict and how to record it)
