#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

usage() {
  cat <<'USAGE'
Usage: ./test.sh [options]

Options:
  -a, --all        Run lint, native, and E2E tests (default).
  -n, --native     Run native tests only.
  -e, --e2e        Run E2E tests only.
  -l, --lint       Run lint checks only.
  -q, --quiet      Suppress go test output (failures still surface).
  -v, --verbose    Enable verbose go test output (default).
  -h, --help       Show this help message.
  -e2e-preserve-all     Preserve all E2E test repositories.
  -e2e-clear-all        Clear all E2E test repositories, even on failure.

Examples:
  ./test.sh -a
  ./test.sh --e2e --e2e-preserve-all
USAGE
}

run_native=true
run_e2e=true
run_lint=true
e2e_preserve_all=false
e2e_clear_all=false
quiet=false
go_test_verbosity="-v"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -a|--all)
      run_native=true
      run_e2e=true
      run_lint=true
      shift
      ;;
    -n|--native)
      run_native=true
      run_e2e=false
      run_lint=false
      shift
      ;;
    -e|--e2e)
      run_native=false
      run_e2e=true
      run_lint=false
      shift
      ;;
    -l|--lint)
      run_native=false
      run_e2e=false
      run_lint=true
      shift
      ;;
    -q|--quiet)
      quiet=true
      go_test_verbosity=""
      shift
      ;;
    -v|--verbose)
      quiet=false
      go_test_verbosity="-v"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -e2e-preserve-all)
      e2e_preserve_all=true
      shift
      ;;
    -e2e-clear-all)
      e2e_clear_all=true
      shift
      ;;
    *)
      echo -e "${RED}Unknown option: $1${NC}"
      echo ""
      usage
      exit 2
      ;;
  esac
done

if [[ "$run_lint" == true ]]; then
  echo -e "${YELLOW}=== Running Lint Checks ===${NC}"
  echo ""

  gofmt_cmd=(gofmt -l .)
  if [[ "$quiet" == true ]]; then
    gofmt_output="$("${gofmt_cmd[@]}")"
    if [[ -n "$gofmt_output" ]]; then
      echo ""
      echo "The following files are not formatted:"
      echo "$gofmt_output"
      echo ""
      echo -e "${RED}✗ gofmt check failed${NC}"
      exit 1
    fi
  else
    gofmt_output="$("${gofmt_cmd[@]}")"
    if [[ -n "$gofmt_output" ]]; then
      echo ""
      echo "The following files are not formatted:"
      echo "$gofmt_output"
      echo ""
      echo -e "${RED}✗ gofmt check failed${NC}"
      exit 1
    fi
  fi

  lint_cmd=(go vet)
  if [[ "$go_test_verbosity" == "-v" ]]; then
    lint_cmd+=("-v")
  fi
  lint_cmd+=("./...")
  if [[ "$quiet" == true ]]; then
    if "${lint_cmd[@]}" >/dev/null; then
      echo ""
      echo -e "${GREEN}✓ Lint checks passed${NC}"
      echo ""
    else
      echo ""
      echo -e "${RED}✗ Lint checks failed${NC}"
      exit 1
    fi
  elif "${lint_cmd[@]}"; then
    echo ""
    echo -e "${GREEN}✓ Lint checks passed${NC}"
    echo ""
  else
    echo ""
    echo -e "${RED}✗ Lint checks failed${NC}"
    exit 1
  fi
fi

if [[ "$run_native" == true ]]; then
  echo -e "${YELLOW}=== Running Native Go Test Suite ===${NC}"
  echo ""

# Run all tests except the e2e test directory
  native_cmd=(go test)
  if [[ -n "$go_test_verbosity" ]]; then
    native_cmd+=("$go_test_verbosity")
  fi
  native_cmd+=($(go list ./... | grep -v '/tests/e2e$'))
  if [[ "$quiet" == true ]]; then
    if "${native_cmd[@]}" >/dev/null; then
      echo ""
      echo -e "${GREEN}✓ Native test suite passed${NC}"
      echo ""
    else
      echo ""
      echo -e "${RED}✗ Native test suite failed${NC}"
      exit 1
    fi
  elif "${native_cmd[@]}"; then
    echo ""
    echo -e "${GREEN}✓ Native test suite passed${NC}"
    echo ""
  else
    echo ""
    echo -e "${RED}✗ Native test suite failed${NC}"
    exit 1
  fi
fi

if [[ "$run_e2e" == true ]]; then
  echo -e "${YELLOW}=== Running E2E Test Suite ===${NC}"
  echo ""

# Run the e2e tests
  e2e_cmd=(go test)
  if [[ -n "$go_test_verbosity" ]]; then
    e2e_cmd+=("$go_test_verbosity")
  fi
  e2e_cmd+=("./tests/e2e")
  if [[ "$e2e_preserve_all" == true ]]; then
    e2e_cmd+=("-e2e-preserve-all")
  fi
  if [[ "$e2e_clear_all" == true ]]; then
    e2e_cmd+=("-e2e-clear-all")
  fi
  if [[ "$quiet" == true ]]; then
    if "${e2e_cmd[@]}" >/dev/null; then
      echo ""
      echo -e "${GREEN}✓ E2E test suite passed${NC}"
      echo ""
    else
      echo ""
      echo -e "${RED}✗ E2E test suite failed${NC}"
      exit 1
    fi
  elif "${e2e_cmd[@]}"; then
    echo ""
    echo -e "${GREEN}✓ E2E test suite passed${NC}"
    echo ""
  else
    echo ""
    echo -e "${RED}✗ E2E test suite failed${NC}"
    exit 1
  fi
fi

echo -e "${GREEN}=== All Tests Passed ===${NC}"
