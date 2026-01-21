# Distribution Packaging Design

This document captures the build, signing, and release workflow for Governator v2 packages on Homebrew and apt-managed systems.

## Homebrew Packaging Workflow

1. **Build the binary.**
   - Use the canonical Go toolchain with reproducible settings.
     ```bash
     go build -trimpath -ldflags "-s -w" -o governator ./cmd/governator
     ```
   - Optionally wrap this call with `goreleaser` to produce archives for multiple platforms simultaneously:
     ```bash
     goreleaser release --rm --snapshot --snapshot-time "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
     ```
2. **Create a signed GitHub release.**
   - Tag the commit that produced the artifacts and upload the `governator` binary (or tarball) for the macOS target.
   - Homebrew relies on the archive URLs and SHA256 hashes published with the release. Use `shasum -a 256` to capture the digest.
3. **Publish the formula.**
   - Submit a PR to the tap repository (usually `github.com/<org>/homebrew-governator`) integrating the new `url`, `sha256`, and `version`.
   - Example of `brew bump-formula-pr`:
     ```bash
     brew bump-formula-pr governator --url=https://github.com/<org>/governator/releases/download/vX.Y.Z/governator.tar.gz --sha256=$(shasum -a 256 governator.tar.gz | cut -d' ' -f1)
     ```
4. **Happy-path verification.**
   - Once the formula lands, run:
     ```bash
     brew install --build-from-source governator
     brew test --retry governator
     ```
5. **Publishing notes.**
   - Reuse GitHub Actions (or similar) to drive `brew test-bot` and the release build.
   - Keep the formula minimal: install only the `governator` binary and supporting shell completions if generated.

## apt (deb) Packaging Workflow

1. **Build Go binary(s).**
   - Build the Linux binary as above, optionally cross-compiling via `goreleaser` or `GOOS=linux GOARCH=amd64 go build ...`.
2. **Produce a Debian package.**
   - Assemble a `debian/` directory with `control`, `changelog`, `compat`, and maintainer scripts.
   - Use `dpkg-deb` to build and `lintian` to validate: 
     ```bash
     go build -trimpath -ldflags "-s -w" -o _build/governator ./cmd/governator
     mkdir -p _build/usr/local/bin
     mv _build/governator _build/usr/local/bin/governator
     dpkg-deb --build _build governator_vX.Y.Z_amd64.deb
     lintian governator_vX.Y.Z_amd64.deb
     ```
3. **Sign prerequisites (sad path).**
   - Debian requires signed `.changes`/`.dsc` files before uploading to an apt repo. These artifacts must be signed by a maintainer key:
     ```bash
     gpg --list-secret-keys
     dch --create --package governator
     debsign --re-sign --keyid <KEYID> governator_VX.Y.Z_amd64.changes
     ```
   - Ensure the signing key is uploaded to a keyserver and trusted by downstream maintainers before the first release.
4. **Publish to the apt repo.**
   - Push the `.deb` to a repository manager (e.g., `reprepro`, `aptly`, or Launchpad) with `reprepro includedeb` or the provider’s upload API.
   - Document repository metadata so operators can add `deb https://repo.example.com/governator stable main`.
5. **Happy-path verification.**
   - Install from the repo on Ubuntu:
     ```bash
     sudo apt-get update
     sudo apt-get install governator
     governator --version
     ```

## First Release Checklist

1. Build binaries for macOS and Ubuntu using `goreleaser` or `go build`.
2. Publish a signed GitHub release with the macOS archive and Linux binaries.
3. Bump the Homebrew formula with updated `url`, `sha256`, and `version` via `brew bump-formula-pr`.
4. Build and lint the Debian package, then sign the release artifacts with a released GPG key.
5. Upload the `.deb` to the apt repository and refresh `Packages`/`Release` files.
6. Verify installs: `brew install governator` and `sudo apt-get install governator`.
7. Update `docs/v2-upgrade-notes.md` to note the packaging paths and any manual steps.
8. Confirm the release checklist in this document has been completed before tagging future patches.
