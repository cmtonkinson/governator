<!--
File: specs/v2-template-packaging.md
Purpose: Specify how v2 template assets are packaged, located, and loaded.
-->
# Governator v2 Template Packaging

## Decision
Templates are embedded in the Go binary and optionally overridden by repo-local
files under `_governator/templates/`. The binary always ships a complete set so
system installs can bootstrap and plan without external assets.

## Template roots
- Embedded: `internal/templates/` (go:embed target in the binary).
- Repo override: `_governator/templates/` (optional; used when present).

When resolving a template, load from `_governator/templates/` if the file
exists; otherwise load the embedded copy.

## Naming conventions
Template identifiers are relative paths without the root prefix. The path is
stable and acts as the lookup key.

- Bootstrap: `bootstrap/<name>.md` (Power Six artifacts, ADRs, etc.).
- Planning: `planning/<name>.md` (task templates, prompt templates).

Example lookup keys:
- `bootstrap/architecture.md`
- `planning/task.md`

## Initialization
`init` or bootstrap writes missing template files into
`_governator/templates/` by copying from the embedded defaults. Existing local
templates are left untouched to preserve operator customization.

## Rationale
Embedding guarantees availability for system-installed binaries. The repo-local
override preserves determinism and enables project-level customization without
requiring external packaging.
