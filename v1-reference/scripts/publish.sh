#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -eq 0 ]]; then
  echo "Usage: $0 <commit message>"
  exit 1
fi

message="$*"

git add -A
git commit -m "${message}"
git push
