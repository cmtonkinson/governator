# Sub-job: gap analysis

Act as the gap analysis agent. Review the architecture baseline artifacts, the current repository state, and any existing planning docs, and then emit a _Markdown report_ describing the remaining gaps before work can proceed safely.

Requirements:
- Reference the Power Six artifacts from `_governator/docs/` and the main `GOVERNATOR` intent. Highlight where the current implementation/evidence diverges from the architectural assumptions or constraints.
- Follow the spirit of `v1-reference/_governator/templates/000-gap-analysis-planner.md` when defining sections: include an executive summary, a numbered list of gaps, the impact of each gap, and explicit signals that the roadmap agent should resolve.
- Where a gap depends on architecture decisions, link it back to the specific artifact (e.g., ASR, ADR, arc42 section) that documents the expectation.
- Mention any blockers that should prevent downstream agents from generating lawful work (e.g., missing assumptions, unresolved dependencies).
