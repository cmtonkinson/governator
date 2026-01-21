# Release Scripts

Provides a reproducible helper for generating Governator v2 release artifacts on macOS (Homebrew) and Ubuntu (apt).

## Purpose
`./scripts/release.sh` builds the CLI twice (darwin/amd64 and linux/amd64), injects opaque version metadata via Go linker flags, and packages the outputs for their respective platforms.

## Usage

```bash
./scripts/release.sh --version 1.2.3 --commit deadbeef --built-at 2025-01-01T00:00:00Z brew apt
```

The `--version` flag is required; commit and timestamp default to `git rev-parse HEAD` and the current UTC time when omitted. `--out-dir` controls where artifacts land (default `./dist`). Supported targets are `brew`, `apt`, and `all`.

## Determinism
- Go builds use `-trimpath` plus `-ldflags "-s -w"` with explicit `internal/buildinfo` overrides.
- Tarballs and DEBs are assembled via `python3` scripts that emit deterministic metadata (static `mtime`, fixed ownership, and sorted paths).
- The apt pipeline builds the `.deb` manually (`debian-binary`, `control.tar.gz`, `data.tar.gz`, and an `ar` archive) to avoid distro-specific tooling dependencies.

## Outputs
- `dist/homebrew/governator-<version>.tar.gz` (macOS binary)
- `dist/apt/governator_<version>_amd64.deb` (Ubuntu package)

## Requirements
`go`, `git`, `python3`, and `ar` must exist in `PATH`. Missing commands yield a clear error.

## Failure Modes
- Running without `--version` exits with a helpful message.
- Refer to the script usage (`--help`) to see accepted options and targets.
