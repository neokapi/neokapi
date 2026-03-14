#!/bin/bash
# Test kapi CLI commands used in VHS demos
# Run before generating recordings to catch issues early
#
# For Bowrain CLI tests, see bowrain/e2e/tapes/test-cli.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
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

echo "============================================"
echo "Kapi CLI Demo Tests"
echo "============================================"
echo ""

# Build kapi
echo "Building kapi..."
(cd ../.. && make build) || {
  echo -e "${RED}Failed to build kapi${NC}"
  exit 1
}

export PATH="$SCRIPT_DIR/../../bin:$PATH"

# Skip plugin loading for faster tests
export KAPI_PLUGIN_DIR=/tmp/kapi-no-plugins
mkdir -p "$KAPI_PLUGIN_DIR"

echo -e "${GREEN}✓ kapi built${NC}"
echo ""

# Test sample files
echo "Sample files:"
printf "  Checking: %-40s" "samples/messages.json"
if [ -f "samples/messages.json" ]; then echo -e "${GREEN}✓${NC}"; passed=$((passed + 1)); else echo -e "${RED}✗${NC}"; failed=$((failed + 1)); fi
echo ""

# overview.tape commands
echo "overview.tape commands:"
test_cmd "kapi --help" "kapi --help" "Usage:"
test_cmd "kapi formats" "kapi formats" "html"
test_cmd "kapi tools" "kapi tools" "pseudo-translate"
echo ""

# word-count.tape commands
echo "word-count.tape commands:"
test_cmd "word-count messages.json" "kapi word-count samples/messages.json" "WORDS"
echo ""

# pseudo-translate.tape commands
echo "pseudo-translate.tape commands:"
rm -rf out
test_cmd "pseudo-translate json" "kapi pseudo-translate samples/messages.json --target-lang fr && cat out/messages.json" "welcome"
rm -rf out
echo ""

# Use isolated config for termbase/TM tests
export KAPI_CONFIG_DIR="$(mktemp -d)"

# termbase-qa.tape commands
echo "termbase-qa.tape commands:"
printf "  Checking: %-40s" "samples/glossary.csv"
if [ -f "samples/glossary.csv" ]; then echo -e "${GREEN}✓${NC}"; passed=$((passed + 1)); else echo -e "${RED}✗${NC}"; failed=$((failed + 1)); fi
printf "  Checking: %-40s" "samples/messages_en.json"
if [ -f "samples/messages_en.json" ]; then echo -e "${GREEN}✓${NC}"; passed=$((passed + 1)); else echo -e "${RED}✗${NC}"; failed=$((failed + 1)); fi
test_cmd "termbase import csv" "kapi termbase import samples/glossary.csv --name product-terms --format csv -s en -t fr --header" ""
test_cmd "termbase stats" "kapi termbase stats --name product-terms" "7"
test_cmd "termbase lookup" "kapi termbase lookup password --name product-terms -s en -t fr" "mot de passe"
test_cmd "termbase search" "kapi termbase search encrypt --name product-terms -s en" "encryption"
rm -rf out && mkdir -p out
test_cmd "pseudo-translate en→fr" "kapi flow run pseudo-translate -i samples/messages_en.json -o out/pseudo_fr.json --target-lang fr" ""
test_cmd "qa-check with termbase" "kapi flow run qa-check -i out/pseudo_fr.json -o out/qa_report.json --source-lang en --target-lang fr --termbase product-terms" ""
rm -rf out
echo ""

# termbase-pretranslation.tape commands
echo "termbase-pretranslation.tape commands:"
printf "  Checking: %-40s" "samples/project.tmx"
if [ -f "samples/project.tmx" ]; then echo -e "${GREEN}✓${NC}"; passed=$((passed + 1)); else echo -e "${RED}✗${NC}"; failed=$((failed + 1)); fi
test_cmd "tm import tmx" "kapi tm import samples/project.tmx --name project-tm -s en -t fr" ""
rm -rf out && mkdir -p out
test_cmd "tm-leverage" "kapi flow run tm-leverage -i samples/messages_en.json -o out/step1_tm.json --source-lang en --target-lang fr --tm project-tm" ""
test_cmd "pseudo-translate step2" "kapi flow run pseudo-translate -i out/step1_tm.json -o out/step2_translated.json --target-lang fr" ""
test_cmd "qa-check pipeline" "kapi flow run qa-check -i out/step2_translated.json -o out/step3_qa.json --source-lang en --target-lang fr --termbase product-terms" ""
rm -rf out
echo ""

# Clean up
rm -rf "$KAPI_CONFIG_DIR"

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
