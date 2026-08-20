#!/usr/bin/env bash
#
# Guard: a workflow step that `uses:` a local action names a path that exists,
# relative to where that job actually checked the repository out.
#
# A local `uses: ./x` is resolved against $GITHUB_WORKSPACE, not against the
# repository — so a job that checks out with `path: neokapi` must say
# `./neokapi/.github/actions/x`. Get it wrong and the job dies at that step with
#
#   Can't find 'action.yml' … under '/home/runner/work/neokapi/neokapi/.github/
#   actions/apt-install'. Did you forget to run actions/checkout …?
#
# which is a runtime failure in whatever job happens to use it, discovered
# minutes into a run rather than in review. It is exactly the mistake a sweep
# across many workflows makes: most check out at the root, two do not, and the
# two that do not look identical in the diff.
#
# What this checks, per job:
#   * every `uses: ./…` resolves to a directory holding action.yml/action.yaml,
#     after prefixing the job's checkout path when it has one
#   * a job using a local action checks the repository out first
#
# Usage:
#     ./scripts/check-local-actions.sh
#
# Wired into `make lint` and the repo-guards CI job.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

python3 - "$root" <<'PY'
import sys, os, glob, yaml

root = sys.argv[1]
problems = []

def action_dir_ok(root, rel):
    # rel[2:] rather than lstrip('./'): lstrip strips a CHARACTER SET, so it
    # eats the leading dot of '.github' and every path resolves to nothing.
    d = os.path.join(root, rel[2:] if rel.startswith('./') else rel)
    return any(os.path.isfile(os.path.join(d, n)) for n in ('action.yml', 'action.yaml', 'Dockerfile'))

for path in sorted(glob.glob(os.path.join(root, '.github/workflows/*.yml'))):
    name = os.path.relpath(path, root)
    doc = yaml.safe_load(open(path))
    for job_name, job in (doc.get('jobs') or {}).items():
        steps = job.get('steps') or []
        # The checkout paths this job has established, in order.
        checkout_paths = []
        for i, step in enumerate(steps):
            if not isinstance(step, dict):
                continue
            uses = str(step.get('uses', ''))
            if 'actions/checkout' in uses:
                checkout_paths.append((step.get('with') or {}).get('path') or '')
                continue
            if not uses.startswith('./'):
                continue

            if not checkout_paths:
                problems.append(
                    f"{name}: job {job_name}: step {i} uses the local action {uses} "
                    f"before any actions/checkout, so it cannot exist yet")
                continue

            # The path is workspace-relative; this repository is somewhere
            # inside the workspace, at each checkout's `path:` (or at the root).
            # So try the reference against every checkout this job has made, and
            # accept it if any of them puts it on a real action.
            want = uses[2:]
            resolved = False
            for co in checkout_paths:
                rel = want
                if co:
                    if not want.startswith(co.rstrip('/') + '/'):
                        continue
                    rel = want[len(co.rstrip('/')) + 1:]
                if action_dir_ok(root, './' + rel):
                    resolved = True
                    break
            if resolved:
                continue

            # Not reachable as written. Say what it should have been when the
            # job's own checkout makes that obvious.
            suggestion = ''
            for co in checkout_paths:
                if co and action_dir_ok(root, './' + want):
                    suggestion = (f"; this job checks out with path: {co}, "
                                  f"so it should be ./{co.rstrip('/')}/{want}")
                    break
            problems.append(f"{name}: job {job_name}: {uses} names no local action{suggestion}")

if problems:
    print("check-local-actions: a workflow names a local action that will not resolve\n")
    for p in problems:
        print("  " + p)
    print("\nA local `uses: ./x` resolves against the workspace, not the repository.")
    sys.exit(1)

print("check-local-actions: OK")
PY
