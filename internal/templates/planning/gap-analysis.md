# Sub-job: gap analysis

Produce the gap analysis JSON object defined in `specs/v2-planning-subjobs.md`.

Requirements:
- Output JSON only.
- Include `schema_version` and `kind: "gap_analysis"`.
- Set `is_greenfield` to match the input repo state.
- If gaps exist, include `area`, `current`, `desired`, and `risk`.
