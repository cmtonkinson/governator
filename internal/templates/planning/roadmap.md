# Sub-job: roadmap decomposition

Produce the roadmap decomposition JSON object defined in `specs/v2-planning-subjobs.md`.

Requirements:
- Output JSON only.
- Include `schema_version` and `kind: "roadmap_decomposition"`.
- Set `depth_policy` and `width_policy` deterministically.
- Emit ordered `items` with stable identifiers.
