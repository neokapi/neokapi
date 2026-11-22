#!/usr/bin/env python3
"""Guard: every Go command the repository runs builds with -trimpath.

Without -trimpath, cmd/go puts each package's absolute directory into its
build cache key. Every git worktree then compiles the whole repository into
cache entries of its own: five agents in five worktrees grew the shared cache
by 54 GB in two hours and filled the disk. With the flag, two checkouts of
one commit share every compiled package and test result.

A command that runs without the flag does no harm on its own, but it fills a
second cache population beside the first and its results disagree with
everyone else's. So the flag is set in a few places and this guard keeps them
in step:

  * scripts/lib/goflags.mk appends -trimpath to GOFLAGS and exports it, and
    the root Makefile and bowrain/Makefile include it;
  * .github/actions/setup-go-modules exports it for every job that uses it;
  * a workflow job that installs Go with actions/setup-go directly, or runs
    `go` without installing it, sets GOFLAGS with -trimpath in the
    workflow's or the job's `env`;
  * .claude/settings.json sets it for agent sessions;
  * no Makefile, make fragment, script or workflow assigns GOFLAGS a value
    without -trimpath (an assignment that keeps $GOFLAGS, or appends to it,
    is fine), and none unsets it.

    scripts/check-go-trimpath.py              # check the repository
    scripts/check-go-trimpath.py --self-test  # prove it on a scratch tree

Exit status: 0 every place agrees, 1 one does not, 2 an input is unreadable.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import tempfile

try:
    import yaml
except ImportError:  # pragma: no cover
    print("check-go-trimpath: needs PyYAML (pip install pyyaml)", file=sys.stderr)
    sys.exit(2)

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

FRAGMENT = "scripts/lib/goflags.mk"
MAKEFILES_THAT_INCLUDE = ("Makefile", "bowrain/Makefile")
SETUP_ACTION = ".github/actions/setup-go-modules/action.yml"
SETTINGS = ".claude/settings.json"

# An assignment to GOFLAGS in make, shell or YAML. The value is group 2.
ASSIGN = re.compile(r"(?<![A-Za-z0-9_])GOFLAGS\s*(\+?:?=|:(?=\s|$))\s*(.*)$")
UNSET = re.compile(r"(?:env\s+-u\s*|unset\s+)GOFLAGS\b")
KEEPS = re.compile(r"\$\(GOFLAGS\)|\$\{GOFLAGS|\$GOFLAGS\b")
GO_COMMAND = re.compile(r"(?<![\w./-])go\s+(build|test|run|vet|install|generate|list)\b")


def tracked(root: str) -> list[str]:
    try:
        out = subprocess.run(
            ["git", "ls-files"], cwd=root, check=True, capture_output=True, text=True
        ).stdout
        return out.splitlines()
    except (OSError, subprocess.CalledProcessError):
        files = []
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [d for d in dirnames if d not in (".git", "node_modules")]
            for f in filenames:
                files.append(os.path.relpath(os.path.join(dirpath, f), root))
        return files


def is_checked_file(rel: str) -> bool:
    base = os.path.basename(rel)
    if base == "Makefile" or rel.endswith(".mk"):
        return True
    if rel.startswith("scripts/") and rel.endswith((".sh", ".bash")):
        return True
    if rel.startswith((".github/workflows/", ".github/actions/")) and rel.endswith((".yml", ".yaml")):
        return True
    return False


def has_trimpath(value: str) -> bool:
    return re.search(r"(?<!\w)-trimpath(?![\w-])", value) is not None


def check_assignments(root: str, rel: str) -> list[str]:
    problems = []
    try:
        with open(os.path.join(root, rel), encoding="utf-8") as fh:
            lines = fh.read().splitlines()
    except (OSError, UnicodeDecodeError) as err:
        return [f"{rel}: unreadable: {err}"]
    for n, line in enumerate(lines, 1):
        stripped = line.strip()
        if stripped.startswith("#"):
            continue
        if UNSET.search(line):
            problems.append(f"{rel}:{n}: unsets GOFLAGS, so the command runs without -trimpath")
            continue
        m = ASSIGN.search(line)
        if not m:
            continue
        op, value = m.group(1), m.group(2)
        if op == "+=" or KEEPS.search(value) or has_trimpath(value):
            continue
        problems.append(f"{rel}:{n}: GOFLAGS is set without -trimpath: {stripped}")
    return problems


def check_fragment(root: str) -> list[str]:
    path = os.path.join(root, FRAGMENT)
    try:
        text = open(path, encoding="utf-8").read()
    except OSError:
        return [f"{FRAGMENT}: missing; it is the one definition of the make-side GOFLAGS"]
    problems = []
    if not re.search(r"^GOFLAGS\s*:=\s*\$\(strip \$\(GOFLAGS\) -trimpath\)", text, re.M):
        problems.append(f"{FRAGMENT}: does not append -trimpath to GOFLAGS")
    if not re.search(r"^export GOFLAGS\b", text, re.M):
        problems.append(f"{FRAGMENT}: does not export GOFLAGS")
    for mk in MAKEFILES_THAT_INCLUDE:
        try:
            body = open(os.path.join(root, mk), encoding="utf-8").read()
        except OSError:
            problems.append(f"{mk}: missing")
            continue
        if not re.search(r"^include\s+\S*scripts/lib/goflags\.mk\s*$", body, re.M):
            problems.append(f"{mk}: does not include {FRAGMENT}")
    return problems


def check_settings(root: str) -> list[str]:
    try:
        with open(os.path.join(root, SETTINGS), encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, ValueError) as err:
        return [f"{SETTINGS}: unreadable: {err}"]
    value = (data.get("env") or {}).get("GOFLAGS", "")
    if not has_trimpath(str(value)):
        return [f"{SETTINGS}: env.GOFLAGS must contain -trimpath for agent sessions"]
    return []


def load_yaml(root: str, rel: str):
    with open(os.path.join(root, rel), encoding="utf-8") as fh:
        return yaml.safe_load(fh)


def check_setup_action(root: str) -> list[str]:
    try:
        doc = load_yaml(root, SETUP_ACTION)
    except (OSError, yaml.YAMLError) as err:
        return [f"{SETUP_ACTION}: unreadable: {err}"]
    for step in (doc.get("runs") or {}).get("steps") or []:
        run = str(step.get("run") or "")
        if "GITHUB_ENV" in run and "GOFLAGS" in run and has_trimpath(run):
            return []
    return [f"{SETUP_ACTION}: no step exports GOFLAGS with -trimpath to GITHUB_ENV"]


def env_has_trimpath(env) -> bool:
    return isinstance(env, dict) and has_trimpath(str(env.get("GOFLAGS", "")))


def check_workflow(root: str, rel: str) -> list[str]:
    try:
        doc = load_yaml(root, rel)
    except (OSError, yaml.YAMLError) as err:
        return [f"{rel}: unreadable: {err}"]
    if not isinstance(doc, dict):
        return []
    top = env_has_trimpath(doc.get("env"))
    problems = []
    for name, job in (doc.get("jobs") or {}).items():
        if not isinstance(job, dict) or "steps" not in job:
            continue
        steps = job.get("steps") or []
        uses = [str(s.get("uses", "")) for s in steps if isinstance(s, dict)]
        if any(u.startswith("./.github/actions/setup-go-modules") for u in uses):
            continue
        direct = any(u.startswith("actions/setup-go@") for u in uses)
        runs_go = any(
            GO_COMMAND.search(str(s.get("run", ""))) for s in steps if isinstance(s, dict)
        )
        if not (direct or runs_go):
            continue
        if top or env_has_trimpath(job.get("env")):
            continue
        why = "installs Go with actions/setup-go" if direct else "runs go"
        problems.append(
            f"{rel}: job {name} {why} without setup-go-modules; set GOFLAGS: -trimpath "
            "in the workflow's or the job's env"
        )
    return problems


def check(root: str) -> list[str]:
    problems = check_fragment(root) + check_settings(root) + check_setup_action(root)
    for rel in tracked(root):
        if not is_checked_file(rel) or not os.path.isfile(os.path.join(root, rel)):
            continue
        problems += check_assignments(root, rel)
        if rel.startswith(".github/workflows/"):
            problems += check_workflow(root, rel)
    return problems


GOOD_FRAGMENT = """ifeq ($(filter -trimpath,$(GOFLAGS)),)
GOFLAGS := $(strip $(GOFLAGS) -trimpath)
endif
export GOFLAGS
"""

GOOD_ACTION = """runs:
  using: composite
  steps:
    - name: Set GOFLAGS
      shell: bash
      run: echo "GOFLAGS=${GOFLAGS:+${GOFLAGS} }-trimpath" >> "$GITHUB_ENV"
"""

GOOD_WORKFLOW = """on: pull_request
env:
  GOFLAGS: -trimpath
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-go@v7
      - run: go test ./...
"""

MODULES_WORKFLOW = """on: pull_request
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/setup-go-modules
      - run: go test ./...
  b:
    runs-on: ubuntu-latest
    env:
      GOFLAGS: -mod=mod -trimpath
    steps:
      - run: go vet ./...
  c:
    runs-on: ubuntu-latest
    steps:
      - run: echo no go here
"""


def write(root: str, rel: str, text: str) -> None:
    path = os.path.join(root, rel)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)


def good_tree(root: str) -> None:
    write(root, FRAGMENT, GOOD_FRAGMENT)
    write(root, "Makefile", "GO := go\ninclude scripts/lib/goflags.mk\ntest:\n\tGOFLAGS=\"$(GOFLAGS) -race\" go test ./...\n")
    write(root, "bowrain/Makefile", "include $(ROOT_DIR)/scripts/lib/goflags.mk\nGOFLAGS += -count=1\n")
    write(root, SETUP_ACTION, GOOD_ACTION)
    write(root, SETTINGS, json.dumps({"env": {"GOFLAGS": "-trimpath"}}))
    write(root, ".github/workflows/good.yml", GOOD_WORKFLOW)
    write(root, ".github/workflows/modules.yml", MODULES_WORKFLOW)
    write(root, "scripts/build.sh", "#!/bin/sh\n# GOFLAGS= in a comment is fine\nGOFLAGS=\"-trimpath -v\" go build ./...\n")


def self_test() -> int:
    failures = []
    with tempfile.TemporaryDirectory() as root:
        good_tree(root)
        problems = check(root)
        if problems:
            failures.append("the good tree is reported: " + "; ".join(problems))

    breakages = {
        "a Makefile that does not include the fragment": (
            "bowrain/Makefile", "GO := go\n", "does not include"),
        "a fragment that no longer appends the flag": (
            FRAGMENT, "export GOFLAGS\n", "does not append -trimpath"),
        "a recipe that overrides GOFLAGS": (
            "Makefile", "include scripts/lib/goflags.mk\nx:\n\tGOFLAGS=-race go test ./...\n",
            "GOFLAGS is set without -trimpath"),
        "a script that unsets GOFLAGS": (
            "scripts/build.sh", "#!/bin/sh\nenv -u GOFLAGS go build ./...\n", "unsets GOFLAGS"),
        "a workflow job that installs Go directly with no env": (
            ".github/workflows/good.yml", GOOD_WORKFLOW.replace("env:\n  GOFLAGS: -trimpath\n", ""),
            "installs Go with actions/setup-go"),
        "a workflow job that runs go on the runner's Go": (
            ".github/workflows/bare.yml",
            "on: pull_request\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go build ./...\n",
            "runs go"),
        "a workflow env that sets GOFLAGS without the flag": (
            ".github/workflows/good.yml", GOOD_WORKFLOW.replace("-trimpath", "-mod=mod"),
            "GOFLAGS is set without -trimpath"),
        "the setup action losing its export": (
            SETUP_ACTION, "runs:\n  using: composite\n  steps: []\n", "no step exports GOFLAGS"),
        "agent settings without the flag": (
            SETTINGS, json.dumps({"env": {}}), "env.GOFLAGS must contain -trimpath"),
    }
    for what, (rel, text, expect) in breakages.items():
        with tempfile.TemporaryDirectory() as root:
            good_tree(root)
            write(root, rel, text)
            problems = check(root)
            if not any(expect in p for p in problems):
                failures.append(f"{what} is not reported (got: {problems})")

    if failures:
        for f in failures:
            print(f"check-go-trimpath self-test: {f}", file=sys.stderr)
        return 1
    print("check-go-trimpath: self-test passed")
    return 0


def main() -> int:
    if sys.argv[1:] == ["--self-test"]:
        return self_test()
    if sys.argv[1:]:
        print(__doc__, file=sys.stderr)
        return 2
    problems = check(ROOT)
    for p in problems:
        print(f"check-go-trimpath: {p}", file=sys.stderr)
    if problems:
        print(
            "check-go-trimpath: every Go command builds with -trimpath; see scripts/lib/goflags.mk",
            file=sys.stderr,
        )
        return 1
    print("check-go-trimpath: every Go command builds with -trimpath")
    return 0


if __name__ == "__main__":
    sys.exit(main())
