#!/bin/bash

# GitProvider Testing Script
# This script makes it easy to test both GitHub and GitLab providers

set -e

echo "=========================================="
echo "Telefonistka GitProvider Test Suite"
echo "=========================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

run_test() {
    local test_name="$1"
    local test_command="$2"
    
    echo -n "Running: $test_name... "
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if eval "$test_command" > /tmp/test_output_$$ 2>&1; then
        echo -e "${GREEN}✓ PASS${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗ FAIL${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo "Error output:"
        cat /tmp/test_output_$$
        echo ""
    fi
    rm -f /tmp/test_output_$$
}

echo "Step 1: Unit Tests (no credentials needed)"
echo "==========================================\n"

run_test "Factory and types tests" "go test ./internal/pkg/gitprovider -v -short"
run_test "GitHub provider tests" "go test ./internal/pkg/gitprovider/github -v -short"
run_test "GitLab provider tests" "go test ./internal/pkg/gitprovider/gitlab -v -short"

echo ""
echo "Step 2: Build Test"
echo "==========================================\n"

run_test "Build all packages" "go build ./..."

echo ""
echo "Step 3: Manual Integration Tests"
echo "==========================================\n"

if [ -n "$GITHUB_OAUTH_TOKEN" ]; then
    echo -e "${GREEN}GitHub token detected${NC}"
    run_test "GitHub provider integration" "go run cmd/test-providers/main.go github"
else
    echo -e "${YELLOW}⊘ GitHub token not set (skip integration test)${NC}"
    echo "  To test: export GITHUB_OAUTH_TOKEN=your_token"
fi

echo ""

if [ -n "$GITLAB_TOKEN" ]; then
    echo -e "${GREEN}GitLab token detected${NC}"
    run_test "GitLab provider integration" "go run cmd/test-providers/main.go gitlab"
else
    echo -e "${YELLOW}⊘ GitLab token not set (skip integration test)${NC}"
    echo "  To test: export GITLAB_TOKEN=your_token"
fi

echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo "Total tests run: $TESTS_RUN"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
if [ $TESTS_FAILED -gt 0 ]; then
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
else
    echo -e "Failed: 0"
fi
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
