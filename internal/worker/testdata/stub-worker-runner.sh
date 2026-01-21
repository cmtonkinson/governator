#!/usr/bin/env bash
set -euo pipefail

mode="${1:-success}"

case "$mode" in
success)
  echo "stub worker runner success"
  exit 0
  ;;
sleep)
  duration="${2:-60}"
  echo "stub worker runner sleeping for ${duration}s"
  sleep "${duration}"
  exit 0
  ;;
*)
  echo "stub worker runner unknown mode" >&2
  exit 1
  ;;
esac
