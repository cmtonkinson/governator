# Sub-job: architecture baseline

Produce the architecture baseline JSON object defined in `specs/v2-planning-subjobs.md`.

Requirements:
- Output JSON only.
- Include `schema_version` and `kind: "architecture_baseline"`.
- Populate `sources` using the input document paths.
- Keep the summary concise and deterministic.
