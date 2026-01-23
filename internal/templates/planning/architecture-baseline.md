# Sub-job: architecture baseline

Act as the architecture baseline agent. Emit the Power Six architecture artifacts as _Markdown documents_ so the subsequent agents (gap analysis, roadmap, tasking) have stable inputs they can read, reference, and build upon.

Requirements:
- Each artifact must be written under `_governator/docs/` using the templates from `_governator/templates/` (`asr.md`, `arc42.md`, `personas.md`, `wardley.md`, `c4.md`, `adr.md`) as the structural starting point. See `v1-reference/_governator/templates/000-architecture-bootstrap.md` for the intent behind the Power Six.
- Preserve non-empty sections and explain why any optional/top-level entry is intentionally omitted.
- Keep commentary architectural - focus on constraints, risks, assumptions, and the reasoning that makes implementation decisions safe.
- Treat this stage as the definitive design snapshot: every statement should either be traceable to the repo context or clearly marked as an assumption.
