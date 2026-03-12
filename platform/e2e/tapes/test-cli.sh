#!/bin/bash
# Test Bowrain CLI commands used in VHS demos
# Run before generating recordings to catch issues early
#
# For kapi CLI tests, see website/tapes/test-cli.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

passed=0
failed=0

# Test helper
test_cmd() {
  local name="$1"
  local cmd="$2"
  local expected="$3"

  printf "  Testing: %-40s" "$name"

  if output=$(eval "$cmd" 2>&1); then
    if [ -n "$expected" ]; then
      if echo "$output" | grep -q "$expected"; then
        echo -e "${GREEN}PASS${NC}"
        passed=$((passed + 1))
        return 0
      else
        echo -e "${RED}FAIL (missing: $expected)${NC}"
        failed=$((failed + 1))
        return 1
      fi
    else
      echo -e "${GREEN}PASS${NC}"
      passed=$((passed + 1))
      return 0
    fi
  else
    echo -e "${RED}FAIL (command failed)${NC}"
    failed=$((failed + 1))
    return 1
  fi
}

echo "============================================"
echo "Bowrain CLI Demo Tests"
echo "============================================"
echo ""

# Build kapi (needed by create-project.tape)
echo "Building kapi..."
(cd ../../.. && cd kapi && go build -o ../bin/kapi ./cmd/kapi) || {
  echo -e "${RED}Failed to build kapi${NC}"
  exit 1
}

# Build bowrain
echo "Building bowrain..."
(cd ../../.. && cd bowrain-cli && go build -o ../bin/bowrain ./cmd/bowrain) || {
  echo -e "${RED}Failed to build bowrain${NC}"
  exit 1
}

export PATH="$SCRIPT_DIR/../../../bin:$PATH"

# Skip plugin loading for faster tests
export KAPI_PLUGIN_DIR=/tmp/kapi-no-plugins
mkdir -p "$KAPI_PLUGIN_DIR"

echo -e "${GREEN}kapi built${NC}"
echo -e "${GREEN}bowrain built${NC}"
echo ""

# Test sample files
echo "Sample files:"
printf "  Checking: %-40s" "samples/landing-page.html"
if [ -f "samples/landing-page.html" ]; then echo -e "${GREEN}PASS${NC}"; passed=$((passed + 1)); else echo -e "${RED}FAIL${NC}"; failed=$((failed + 1)); fi
echo ""

# overview.tape commands
echo "overview.tape commands:"
test_cmd "bowrain --help" "bowrain --help" "Usage:"
test_cmd "bowrain init --help" "bowrain init --help" ""
test_cmd "bowrain status --help" "bowrain status --help" ""
echo ""

# init.tape commands
echo "init.tape commands:"
test_cmd "bowrain init --help" "bowrain init --help" ""
echo ""

# auth.tape commands
echo "auth.tape commands:"
test_cmd "bowrain auth --help" "bowrain auth --help" "authentication"
test_cmd "bowrain auth status" "bowrain auth status" ""
echo ""

# serve.tape commands
echo "serve.tape commands:"
test_cmd "bowrain serve --help" "bowrain serve --help" "Open a local web dashboard"
echo ""

# create-project.tape commands
echo "create-project.tape commands:"
rm -rf out
test_cmd "pseudo-translate html" "kapi pseudo-translate samples/landing-page.html --target-lang fr && cat out/landing-page.html" ""
rm -rf out
echo ""

# workspaces.tape commands (only if server is running)
echo "workspaces.tape commands:"
if curl -sf http://localhost:8080/api/v1/health > /dev/null 2>&1; then
  test_cmd "server health" "curl -sf http://localhost:8080/api/v1/health" '"status":"ok"'
  echo -e "  ${YELLOW}(Server-backed tests ran against live server)${NC}"
else
  echo -e "  ${YELLOW}(Skipped: server not running)${NC}"
fi
echo ""

echo "============================================"
echo -e "Results: ${GREEN}$passed passed${NC}, ${RED}$failed failed${NC}"
echo "============================================"

if [ $failed -gt 0 ]; then
  echo ""
  echo -e "${RED}Some tests failed. Fix issues before recording demos.${NC}"
  exit 1
else
  echo ""
  echo -e "${GREEN}All tests passed! Ready to record demos.${NC}"
  exit 0
fi
