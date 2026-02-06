# Release Process

Governator uses [GoReleaser](https://goreleaser.com) for automated releases.

## Quick Release

To create a new release:

```bash
# 1. Ensure you're on main with latest changes
git checkout main
git pull

# 2. Create and push a version tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

That's it! GitHub Actions will automatically:
- Run tests on Ubuntu and macOS
- Build binaries for 4 platforms (darwin/linux × amd64/arm64)
- Create .deb packages for Debian/Ubuntu
- Generate checksums and SBOM
- Create GitHub Release with all artifacts
- Update Homebrew tap at `cmtonkinson/homebrew-tap`

## What Gets Built

Each release produces:

### Archives (tar.gz)
- `governator_<version>_darwin_amd64.tar.gz` - macOS Intel
- `governator_<version>_darwin_arm64.tar.gz` - macOS Apple Silicon
- `governator_<version>_linux_amd64.tar.gz` - Linux Intel/AMD
- `governator_<version>_linux_arm64.tar.gz` - Linux ARM

### Debian Packages
- `governator_<version>_amd64.deb` - Debian/Ubuntu x86_64
- `governator_<version>_arm64.deb` - Debian/Ubuntu ARM

### Metadata
- `checksums.txt` - SHA256 checksums for all artifacts

## Installation Methods

### Homebrew (macOS/Linux)
```bash
brew install cmtonkinson/tap/governator
```

### Debian/Ubuntu
```bash
# Download .deb from GitHub releases
sudo dpkg -i governator_<version>_amd64.deb
```

### Manual
```bash
# Download appropriate tar.gz from GitHub releases
tar xzf governator_<version>_<os>_<arch>.tar.gz
sudo mv governator /usr/local/bin/
```

## Version Metadata

All binaries include embedded build information:

```bash
$ governator version
version=1.0.0 commit=abc123def built_at=2025-02-06T10:30:00Z
```

Metadata is injected at build time via ldflags into:
- `github.com/cmtonkinson/governator/internal/buildinfo.Version`
- `github.com/cmtonkinson/governator/internal/buildinfo.Commit`
- `github.com/cmtonkinson/governator/internal/buildinfo.BuiltAt`

## Pre-releases

To create a pre-release (beta, rc, etc.):

```bash
git tag -a v1.0.0-beta.1 -m "Release v1.0.0-beta.1"
git push origin v1.0.0-beta.1
```

GoReleaser automatically detects pre-release versions and marks them accordingly.

## Rollback

To delete a release:

```bash
# Delete GitHub release and tag
gh release delete v1.0.0 --yes
git push --delete origin v1.0.0
git tag -d v1.0.0
```

Note: This doesn't revert Homebrew tap changes. If needed, manually revert the commit in `cmtonkinson/homebrew-tap`.

## Monitoring

Watch releases at:
- **GitHub Actions**: https://github.com/cmtonkinson/governator/actions
- **Releases**: https://github.com/cmtonkinson/governator/releases
- **Homebrew Tap**: https://github.com/cmtonkinson/homebrew-tap

## Troubleshooting

### Release workflow fails

Check:
1. Tests pass locally: `go test ./...`
2. Code is properly formatted: `gofmt -l .`
3. `TAP_GITHUB_TOKEN` secret is set with `repo` scope

### Homebrew tap not updated

Verify:
1. `TAP_GITHUB_TOKEN` secret exists in repository settings
2. Token has write access to `cmtonkinson/homebrew-tap`
3. Check GoReleaser logs in GitHub Actions for errors

### Binary missing version info

Ensure:
1. Git tag follows semver format (vX.Y.Z)
2. Tag is pushed before workflow runs
3. `internal/buildinfo` package exists

## Configuration

Release configuration is in:
- **`.goreleaser.yml`** - Main GoReleaser configuration
- **`.github/workflows/release.yml`** - GitHub Actions workflow

To test changes locally:
```bash
# Install GoReleaser
brew install goreleaser/tap/goreleaser

# Validate configuration
goreleaser check

# Test build (doesn't publish)
goreleaser release --snapshot --clean --skip=publish
```

See **GORELEASER_MIGRATION.md** for detailed testing instructions.
