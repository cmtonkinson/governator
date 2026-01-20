# System Install Distribution Plan

## Summary
Define how Governator v2 ships as a system-installed CLI on macOS and Ubuntu.

## Config Locations
- User defaults live in `~/.config/governator/`.
- Per-repo overrides live in `_governator/config/` within the repo.
- The CLI does not write outside the repo except for user defaults.

## Platforms
- macOS via Homebrew.
- Ubuntu via dpkg (apt).

## Homebrew (High Level)
- Provide a formula that installs the `governator` binary to the Homebrew prefix.
- Treat the CLI as a self-contained binary; no runtime dependencies required.

## dpkg (High Level)
- Provide a Debian package that installs the `governator` binary in a standard
  system path (e.g., `/usr/bin` or `/usr/local/bin`).
- Treat the CLI as a self-contained binary; no runtime dependencies required.

## Update Flow
- Updates are handled by the package manager (`brew upgrade`, `apt upgrade`).
- The CLI does not self-update or download binaries at runtime.
