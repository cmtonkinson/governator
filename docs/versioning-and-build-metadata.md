# Versioning and Build Metadata

## Purpose
Define how the Governator CLI reports version and build information for
system-installed releases.

## Version Source
- The release version comes from the packaging pipeline and is injected at
  build time via Go linker flags.
- Build metadata includes the git commit SHA and build timestamp, also injected
  via linker flags.
- Local developer builds default to placeholder values (see Output Defaults).

## Build Flags
Use Go linker flags to set the build metadata:
- `-X <module>/internal/buildinfo.Version=<semver>`
- `-X <module>/internal/buildinfo.Commit=<git-sha>`
- `-X <module>/internal/buildinfo.BuiltAt=<rfc3339>`

## Version Command Output
`governator version` prints a single, stable, script-friendly line:

```
version=<semver> commit=<git-sha> built_at=<rfc3339>
```

Sample released build output:

```
version=1.2.3 commit=8d3f2a1 built_at=2025-02-14T09:30:00Z
```

## Output Defaults
When build metadata is not provided (e.g. local builds):

```
version=dev commit=unknown built_at=unknown
```
