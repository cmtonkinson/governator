#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== Running Native Go Test Suite ===${NC}"
echo ""

# Run all tests except the e2e test directory
# The ! -path "./test/*" excludes the test directory
if go test -v $(go list ./... | grep -v '/test$'); then
    echo ""
    echo -e "${GREEN}✓ Native test suite passed${NC}"
    echo ""
else
    echo ""
    echo -e "${RED}✗ Native test suite failed${NC}"
    exit 1
fi

echo -e "${YELLOW}=== Running E2E Test Suite ===${NC}"
echo ""

# Run the e2e tests
if go test -v ./test; then
    echo ""
    echo -e "${GREEN}✓ E2E test suite passed${NC}"
    echo ""
    echo -e "${GREEN}=== All Tests Passed ===${NC}"
    exit 0
else
    echo ""
    echo -e "${RED}✗ E2E test suite failed${NC}"
    exit 1
fi
