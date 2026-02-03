#!/bin/bash
# Test CLI commands used in VHS demos
# Run before generating recordings to catch issues early

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

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
        echo -e "${GREEN}✓${NC}"
        passed=$((passed + 1))
        return 0
      else
        echo -e "${RED}✗ (missing: $expected)${NC}"
        failed=$((failed + 1))
        return 1
      fi
    else
      echo -e "${GREEN}✓${NC}"
      passed=$((passed + 1))
      return 0
    fi
  else
    echo -e "${RED}✗ (command failed)${NC}"
    failed=$((failed + 1))
    return 1
  fi
}

# Test file exists
test_file() {
  local path="$1"
  printf "  Checking: %-40s" "$path"
  if [ -f "$path" ]; then
    echo -e "${GREEN}✓${NC}"
    passed=$((passed + 1))
  else
    echo -e "${RED}✗ (not found)${NC}"
    failed=$((failed + 1))
  fi
}

echo "============================================"
echo "CLI Demo Tests"
echo "============================================"
echo ""

# Build kapi
echo "Building kapi..."
(cd ../.. && go build -o bin/kapi ./cmd/kapi) || {
  echo -e "${RED}Failed to build kapi${NC}"
  exit 1
}
export PATH="$SCRIPT_DIR/../../bin:$PATH"

# Skip plugin loading for faster tests
export KAPI_PLUGIN_DIR=/tmp/kapi-no-plugins
mkdir -p "$KAPI_PLUGIN_DIR"

echo -e "${GREEN}✓ kapi built${NC}"
echo ""

# Test sample files exist
echo "Sample files:"
test_file "samples/messages.json"
test_file "samples/landing-page.html"
echo ""

# overview.tape tests
echo "overview.tape commands:"
test_cmd "kapi --help" "kapi --help" "Usage:"
test_cmd "kapi formats" "kapi formats" "html"
test_cmd "kapi tools" "kapi tools" "pseudo-translate"
echo ""

# convert.tape tests
echo "convert.tape commands:"
test_cmd "cat messages.json" "cat samples/messages.json" "welcome"
rm -f /tmp/test-output.yaml /tmp/test-output.json
test_cmd "convert json → yaml" "kapi convert -i samples/messages.json -o /tmp/test-output.yaml && cat /tmp/test-output.yaml" "welcome:"
test_cmd "convert yaml → json" "kapi convert -i /tmp/test-output.yaml -o /tmp/test-output.json && cat /tmp/test-output.json" '"welcome"'
rm -f /tmp/test-output.yaml /tmp/test-output.json
echo ""

# word-count.tape tests
echo "word-count.tape commands:"
test_cmd "word-count messages.json" "kapi word-count samples/messages.json" "WORDS"
echo ""

# pseudo-translate.tape tests
echo "pseudo-translate.tape commands:"
rm -rf out
test_cmd "pseudo-translate json" "kapi pseudo-translate samples/messages.json --target-lang fr && cat out/messages.json" "welcome"
rm -rf out
echo ""

# create-project.tape tests
echo "create-project.tape commands:"
test_cmd "cat landing-page.html" "cat samples/landing-page.html" "<html"
test_cmd "word-count html" "kapi word-count samples/landing-page.html" "WORDS"

# Test pack command (creates file)
rm -f /tmp/test-demo.kaz
test_cmd "pack to .kaz" "kapi pack -i samples/landing-page.html --source-lang en --target-lang fr,de -o /tmp/test-demo.kaz" ""
test_file "/tmp/test-demo.kaz"
rm -f /tmp/test-demo.kaz

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
