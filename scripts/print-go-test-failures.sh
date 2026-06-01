#!/usr/bin/env bash
#
# Print the failed tests (and their captured output) from a `go test -json`
# result file. The CI test jobs redirect -json to a file for JUnit conversion,
# which means a failure is otherwise invisible in the job log — you'd have to
# download an artifact. Call this on failure to surface what actually broke.
#
# Usage: scripts/print-go-test-failures.sh <results.json>

set -euo pipefail
file="${1:?usage: print-go-test-failures.sh <results.json>}"

if [ ! -s "$file" ]; then
  echo "(no results file at $file, or it is empty)"
  exit 0
fi

python3 - "$file" <<'PY'
import json, sys
from collections import defaultdict

path = sys.argv[1]
fails = []
outputs = defaultdict(list)
build_errors = []

for line in open(path):
    line = line.strip()
    if not line:
        continue
    try:
        e = json.loads(line)
    except json.JSONDecodeError:
        # Non-JSON line (e.g. a compiler/build error printed before json) — surface it.
        build_errors.append(line)
        continue
    action = e.get("Action")
    if action == "output":
        outputs[(e.get("Package"), e.get("Test"))].append(e.get("Output", ""))
    elif action == "fail" and e.get("Test"):
        fails.append((e.get("Package"), e.get("Test")))

if build_errors:
    print("=== non-JSON / build output ===")
    print("\n".join(build_errors[-50:]))

for pkg, test in fails:
    print(f"\n=== FAIL {pkg} :: {test} ===")
    body = "".join(outputs.get((pkg, test), []))
    print(body[-4000:] if body else "(no captured output)")

if not fails and not build_errors:
    # Failure with no per-test fail event (e.g. package-level failure / panic).
    print("No per-test failures parsed; tail of raw results:")
    with open(path) as fh:
        print("".join(fh.readlines()[-60:]))
PY
