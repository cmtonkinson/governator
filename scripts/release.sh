#!/usr/bin/env bash
set -euo pipefail

# Release helper for Governator v2. Builds deterministic artifacts for Homebrew and
# apt (deb) packaging and injects version metadata via Go linker flags.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=""
COMMIT=""
BUILT_AT=""
OUT_DIR=""
COMMANDS=()

usage() {
  cat <<'EOF'
Usage: ./scripts/release.sh [options] <targets...>

Targets:
  brew      Build a macOS binary and package it as governator-<version>.tar.gz.
  apt       Build a linux binary and package it as governator_<version>_amd64.deb.
  all       Run both brew and apt targets.

Options:
  --version <semver>   (required) Release version used in build metadata and artifact names.
  --commit <sha>       Git commit SHA used in build metadata. (defaults to HEAD)
  --built-at <time>    Build timestamp in RFC3339. (defaults to current UTC time)
  --out-dir <path>     Destination directory for artifacts (default: ./dist).
  --help               Show this message and exit.
EOF
}

fail() {
  echo "error: $*" >&2
  exit 1
}

info() {
  printf '%s\n' "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

parse_args() {
  if [[ $# -eq 0 ]]; then
    usage
    fail "no targets specified"
  fi

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        [[ $# -ge 2 ]] || fail "--version requires a value"
        VERSION="$2"
        shift 2
        ;;
      --commit)
        [[ $# -ge 2 ]] || fail "--commit requires a value"
        COMMIT="$2"
        shift 2
        ;;
      --built-at)
        [[ $# -ge 2 ]] || fail "--built-at requires a value"
        BUILT_AT="$2"
        shift 2
        ;;
      --out-dir)
        [[ $# -ge 2 ]] || fail "--out-dir requires a value"
        OUT_DIR="$2"
        shift 2
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      brew|homebrew)
        COMMANDS+=("homebrew")
        shift
        ;;
      apt)
        COMMANDS+=("apt")
        shift
        ;;
      all)
        COMMANDS+=("homebrew" "apt")
        shift
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
  done
}

dedupe_commands() {
  if [[ ${#COMMANDS[@]} -eq 0 ]]; then
    fail "no valid targets specified"
  fi

  declare -A seen
  local unique=()
  for tgt in "${COMMANDS[@]}"; do
    if [[ -z "${seen[$tgt]:-}" ]]; then
      seen[$tgt]=1
      unique+=("$tgt")
    fi
  done
  COMMANDS=("${unique[@]}")
}

build_metadata() {
  if [[ -z "$VERSION" ]]; then
    fail "--version <semver> is required"
  fi

  if [[ -z "$COMMIT" ]]; then
    COMMIT="$(git -C "$REPO_ROOT" rev-parse HEAD)"
  fi
  if [[ -z "$BUILT_AT" ]]; then
    BUILT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  fi

  if [[ -z "$OUT_DIR" ]]; then
    OUT_DIR="$REPO_ROOT/dist"
  fi
}

build_binary() {
  local target_os="$1"
  local out_path="$2"
  mkdir -p "$(dirname "$out_path")"
  local ldflags
  ldflags="-s -w -X github.com/cmtonkinson/governator/internal/buildinfo.Version=${VERSION}"
  ldflags+=" -X github.com/cmtonkinson/governator/internal/buildinfo.Commit=${COMMIT}"
  ldflags+=" -X github.com/cmtonkinson/governator/internal/buildinfo.BuiltAt=${BUILT_AT}"

  info "building ${target_os} binary"
  (
    cd "$REPO_ROOT"
    env GOOS="$target_os" GOARCH=amd64 CGO_ENABLED=0 \
      go build -trimpath -ldflags "$ldflags" -o "$out_path" ./cmd/governator
  )
}

create_tarball_for_binary() {
  local dest="$1"
  local src="$2"
  python3 - <<'PY' "$dest" "$src"
import os
import sys
import tarfile

dest, src = sys.argv[1], sys.argv[2]
stat = os.stat(src)
with open(src, "rb") as fp, tarfile.open(dest, "w:gz", format=tarfile.PAX_FORMAT) as tar:
    info = tarfile.TarInfo("governator")
    info.size = stat.st_size
    info.mode = 0o755
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.mtime = 0
    tar.addfile(info, fp)
PY
}

create_data_tarball() {
  local dest="$1"
  local binary="$2"
  python3 - <<'PY' "$dest" "$binary"
import os
import sys
import tarfile

dest, binary = sys.argv[1], sys.argv[2]
with open(binary, "rb") as fp, tarfile.open(dest, "w:gz", format=tarfile.PAX_FORMAT) as tar:
    for directory in ("usr/", "usr/local/", "usr/local/bin/"):
        info = tarfile.TarInfo(directory)
        info.type = tarfile.DIRTYPE
        info.mode = 0o755
        info.uid = info.gid = 0
        info.uname = info.gname = "root"
        info.mtime = 0
        tar.addfile(info)

    info = tarfile.TarInfo("usr/local/bin/governator")
    info.size = os.stat(binary).st_size
    info.mode = 0o755
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.mtime = 0
    tar.addfile(info, fp)
PY
}

create_control_tarball() {
  local dest="$1"
  local control_file="$2"
  python3 - <<'PY' "$dest" "$control_file"
import os
import sys
import tarfile

dest, control_file = sys.argv[1], sys.argv[2]
stat = os.stat(control_file)
with open(control_file, "rb") as fp, tarfile.open(dest, "w:gz", format=tarfile.PAX_FORMAT) as tar:
    info = tarfile.TarInfo("control")
    info.size = stat.st_size
    info.mode = 0o644
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.mtime = 0
    tar.addfile(info, fp)
PY
}

release_homebrew() {
  info "generating Homebrew artifact"
  local target_dir="$OUT_DIR/homebrew"
  mkdir -p "$target_dir"
  local build_dir
  build_dir="$(mktemp -d)"
  local binary_path="$build_dir/governator-darwin"
  build_binary darwin "$binary_path"
  local archive="$target_dir/governator-${VERSION}.tar.gz"
  create_tarball_for_binary "$archive" "$binary_path"
  rm -rf "$build_dir"
  info "Homebrew artifact created: $archive"
}

release_apt() {
  info "generating apt (deb) artifact"
  local target_dir="$OUT_DIR/apt"
  mkdir -p "$target_dir"
  local build_dir
  build_dir="$(mktemp -d)"
  local binary_path="$build_dir/governator-linux"
  build_binary linux "$binary_path"

  local control_file="$build_dir/control"
  cat <<EOF >"$control_file"
Package: governator
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: amd64
Maintainer: Governator Release Team <releases@example.com>
Description: Governator CLI v2 deterministic orchestrator
EOF

  local control_tar="$build_dir/control.tar.gz"
  create_control_tarball "$control_tar" "$control_file"

  local data_tar="$build_dir/data.tar.gz"
  create_data_tarball "$data_tar" "$binary_path"

  local debian_binary="$build_dir/debian-binary"
  printf '2.0\n' >"$debian_binary"

  local output="$target_dir/governator_${VERSION}_amd64.deb"
  ar rcs "$output" "$debian_binary" "$control_tar" "$data_tar"
  rm -rf "$build_dir"
  info "apt artifact created: $output"
}

main() {
  require_command go
  require_command git
  require_command python3
  require_command ar

  parse_args "$@"
  dedupe_commands
  build_metadata

  info "release metadata: version=${VERSION} commit=${COMMIT} built_at=${BUILT_AT}"
  info "writing artifacts to: ${OUT_DIR}"

  for target in "${COMMANDS[@]}"; do
    case "$target" in
      homebrew)
        release_homebrew
        ;;
      apt)
        release_apt
        ;;
      *)
        fail "unsupported target: $target"
        ;;
    esac
  done
}

main "$@"
