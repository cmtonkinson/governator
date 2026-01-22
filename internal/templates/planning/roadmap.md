# Sub-job: roadmap planning

Act as the roadmap planning agent. Use the architecture baseline and gap analysis Markdown artifacts to produce the high-level planning documents—milestones, epics, and canonical features—as Markdown, following the style of the v1 milestone/epic templates.

Requirements:
- Emit `_governator/docs/milestones.md` (see `v1-reference/_governator/templates/milestones.md`) with numbered milestone IDs (`m1`, `m2`, ...), a concise title, and a description of the value delivered per milestone.
- Emit `_governator/docs/epics.md` using the v1 template as reference: each epic must tie to one milestone, include in-scope/out-of-scope, and define done criteria and constraints.
- Capture feature/intent-level notes that explain how the epics and milestones connect back to the architecture baseline, referencing specific ASRs, ADRs, or gaps when applicable.
- Keep everything deterministic: milestone/epic titles, IDs, and descriptions must not rely on ad-hoc formatting or extraneous prose.
