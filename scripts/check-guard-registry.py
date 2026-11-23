#!/usr/bin/env python3
"""Guard: the guard registry under scripts/guards/ is consistent.

    scripts/check-guard-registry.py              # check this repository
    scripts/check-guard-registry.py --self-test  # prove the checks on fixtures

Each file scripts/guards/<target>.mk registers one guard (scripts/lib/guards.mk
describes the shape). `make lint`, scripts/pre-push-check.sh and the Repo
guards job of .github/workflows/ci.yml all read their guards from it, so a new
guard is one new file. This script fails when:

  * a guard file does not register exactly the target it is named for, names a
    lane other than lint, pre-push and ci, defines GUARD_PATHS with `:=` (which
    expands a `$` away) or with a pattern that does not compile, or has no
    `<target>: ## <description>` rule with a recipe;
  * a recipe runs a repository file that does not exist, or a prerequisite
    names neither a make target nor a file;
  * make's view of a lane differs from the files (a guard file make does not
    load);
  * a guard is wired outside the registry: `lint:` lists anything but
    $(GUARDS_LINT), the root Makefile defines a registered target again,
    pre-push runs a self-test, a scripts/check-* script or a `## Guard:`
    target itself, or a Repo guards step runs anything but
    scripts/run-guards.sh.

Exit status: 0 consistent, 1 not, 2 the input could not be read.
"""

from __future__ import annotations

import os
import re
import subprocess
import sys
import tempfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
GUARD_DIR = "scripts/guards"
LANES = ("lint", "pre-push", "ci")
RUNNER = "./scripts/run-guards.sh ci"
PRE_PUSH = "scripts/pre-push-check.sh"
WORKFLOW = ".github/workflows/ci.yml"
CI_JOB = "docs-guard"
# The one run_check in pre-push that runs registered guards.
PRE_PUSH_LOOP = 'run_check "$desc" make --no-print-directory "$target"'

RULE_RE = re.compile(r"^([A-Za-z0-9_.%/-]+)\s*:(?![:=])(.*)$")
FILE_REF_RE = re.compile(r"(?<![\w$/.-])(?:\./)?((?:scripts|\.claude|packages)/[A-Za-z0-9_./-]+)")


def read(root: str, rel: str) -> str:
    with open(os.path.join(root, rel), encoding="utf-8") as f:
        return f.read()


def logical_lines(text: str) -> list[str]:
    """Join backslash-continued lines, as make reads them."""
    out, cur = [], ""
    for line in text.split("\n"):
        if line.endswith("\\"):
            cur += line[:-1] + " "
            continue
        out.append(cur + line)
        cur = ""
    if cur:
        out.append(cur)
    return out


def parse_guard(rel: str, text: str) -> tuple[dict, list[str]]:
    """Read one guard file. Returns its fields and the problems found."""
    target = os.path.basename(rel)[: -len(".mk")]
    problems = []
    g = {"target": target, "lanes": [], "paths": [], "desc": "", "recipe": [], "prereqs": []}
    registered = []
    rules = []
    lines = logical_lines(text)
    i = 0
    while i < len(lines):
        line = lines[i]
        i += 1
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        m = re.match(r"^GUARDS\s*\+=\s*(.*)$", line)
        if m:
            registered += m.group(1).split()
            continue
        m = re.match(r"^GUARD_LANES\.(\S+)\s*(:?=)\s*(.*)$", line)
        if m:
            if m.group(1) != target:
                problems.append(f"{rel}: sets GUARD_LANES.{m.group(1)}, not GUARD_LANES.{target}")
            g["lanes"] = m.group(3).split()
            continue
        m = re.match(r"^GUARD_PATHS\.(\S+)\s*(:?=)\s*(.*)$", line)
        if m:
            if m.group(1) != target:
                problems.append(f"{rel}: sets GUARD_PATHS.{m.group(1)}, not GUARD_PATHS.{target}")
            if m.group(2) != "=":
                problems.append(f"{rel}: GUARD_PATHS.{target} must be set with '=', so a '$' stays literal")
            g["paths"] = m.group(3).split()
            continue
        m = RULE_RE.match(line)
        if m and not line.startswith("\t"):
            name, rest = m.group(1), m.group(2)
            prereqs, _, desc = rest.partition("##")
            rules.append(name)
            recipe = []
            while i < len(lines) and lines[i].startswith("\t"):
                recipe.append(lines[i][1:])
                i += 1
            if name == target:
                g["desc"] = desc.strip()
                g["prereqs"] = prereqs.split()
                g["recipe"] = recipe
            continue
        if line.startswith("\t"):
            problems.append(f"{rel}: a recipe line outside a rule: {line.strip()}")
            continue
        problems.append(f"{rel}: a line the registry does not read: {line.strip()}")

    if registered != [target]:
        problems.append(f"{rel}: must register exactly 'GUARDS += {target}', found {registered or 'none'}")
    if rules != [target]:
        problems.append(f"{rel}: must define exactly one rule, '{target}:', found {rules or 'none'}")
    if not g["lanes"]:
        problems.append(f"{rel}: GUARD_LANES.{target} names no lane (one or more of {' '.join(LANES)})")
    for lane in g["lanes"]:
        if lane not in LANES:
            problems.append(f"{rel}: unknown lane '{lane}' (lanes: {' '.join(LANES)})")
    if len(set(g["lanes"])) != len(g["lanes"]):
        problems.append(f"{rel}: a lane is named twice")
    for p in g["paths"]:
        try:
            re.compile(p)
        except re.error as e:
            problems.append(f"{rel}: GUARD_PATHS pattern {p!r} does not compile: {e}")
    if rules == [target]:
        if not g["desc"]:
            problems.append(f"{rel}: the rule has no '## <description>'")
        if not g["recipe"]:
            problems.append(f"{rel}: the rule has no recipe")
    return g, problems


def make_database(root: str) -> tuple[set[str], dict[str, list[str]]]:
    """Every target make knows, and each registered lane as make sees it."""
    r = subprocess.run(
        ["make", "-C", root, "-p", "-n", "--no-print-directory", "guard-list", "LANE=lint"],
        capture_output=True, text=True,
    )
    if r.returncode != 0:
        raise RuntimeError(f"make -p failed: {r.stderr.strip()[:500]}")
    # make prints a file that is only ever a prerequisite after the comment
    # "# Not a target:", and such a name has no rule.
    targets = set()
    prev = ""
    for line in r.stdout.split("\n"):
        m = RULE_RE.match(line)
        if m and not line.startswith(("\t", "#")) and prev != "# Not a target:":
            targets.add(m.group(1))
        prev = line
    lanes = {}
    for lane in LANES:
        r = subprocess.run(
            ["make", "-C", root, "-s", "--no-print-directory", "guard-list", f"LANE={lane}"],
            capture_output=True, text=True,
        )
        if r.returncode != 0:
            raise RuntimeError(f"make guard-list LANE={lane} failed: {r.stderr.strip()[:500]}")
        lanes[lane] = [l.split("\t")[0] for l in r.stdout.split("\n") if l.strip()]
    return targets, lanes


def check_guards(root: str) -> tuple[list[dict], list[str]]:
    problems = []
    guards = []
    d = os.path.join(root, GUARD_DIR)
    names = sorted(os.listdir(d)) if os.path.isdir(d) else []
    for name in names:
        rel = f"{GUARD_DIR}/{name}"
        if not name.endswith(".mk"):
            problems.append(f"{rel}: not a guard file (scripts/guards/ holds only <target>.mk)")
            continue
        g, p = parse_guard(rel, read(root, rel))
        g["file"] = rel
        guards.append(g)
        problems += p
    if not guards:
        problems.append(f"{GUARD_DIR}/: no guard files")
    return guards, problems


def check_commands(root: str, guards: list[dict], targets: set[str]) -> list[str]:
    problems = []
    for g in guards:
        for line in g["recipe"]:
            for ref in FILE_REF_RE.findall(line):
                ref = ref.rstrip(".")
                if not os.path.exists(os.path.join(root, ref)):
                    problems.append(f"{g['file']}: the recipe runs {ref}, which does not exist")
        for p in g["prereqs"]:
            if "$" in p:
                continue
            if p not in targets and not os.path.exists(os.path.join(root, p)):
                problems.append(f"{g['file']}: prerequisite '{p}' is neither a make target nor a file")
    return problems


def check_lanes(guards: list[dict], lanes: dict[str, list[str]]) -> list[str]:
    problems = []
    for lane in LANES:
        want = sorted(g["target"] for g in guards if lane in g["lanes"])
        got = sorted(lanes.get(lane, []))
        for t in sorted(set(want) - set(got)):
            problems.append(f"make does not list {t} in the {lane} lane; is scripts/lib/guards.mk included?")
        for t in sorted(set(got) - set(want)):
            problems.append(f"make lists {t} in the {lane} lane, but no guard file puts it there")
    return problems


def check_makefile(root: str, guards: list[dict]) -> list[str]:
    problems = []
    registered = {g["target"] for g in guards}
    lines = logical_lines(read(root, "Makefile"))
    lint = [l for l in lines if re.match(r"^lint\s*:(?![:=])", l)]
    if len(lint) != 1:
        problems.append(f"Makefile: expected one 'lint:' rule, found {len(lint)}")
    else:
        prereqs = lint[0].split(":", 1)[1].split("##")[0].split()
        if prereqs != ["$(GUARDS_LINT)"]:
            extra = [p for p in prereqs if p != "$(GUARDS_LINT)"]
            problems.append(
                "Makefile: 'lint:' must list only $(GUARDS_LINT); "
                f"wire {' '.join(extra) or 'the registry'} through a guard file in scripts/guards/"
            )
    if not any(re.match(r"^include\s+scripts/lib/guards\.mk\s*$", l) for l in lines):
        problems.append("Makefile: does not 'include scripts/lib/guards.mk'")
    for l in lines:
        m = RULE_RE.match(l)
        if m and not l.startswith("\t") and m.group(1) in registered:
            problems.append(f"Makefile: defines {m.group(1)}, which {GUARD_DIR}/{m.group(1)}.mk defines")
    return problems


def guard_help_targets(root: str) -> set[str]:
    """Targets of the root Makefile whose help text calls them a guard."""
    out = set()
    for l in logical_lines(read(root, "Makefile")):
        m = re.match(r"^([A-Za-z0-9_-]+)\s*:[^=].*##\s*Guard:", l)
        if m:
            out.add(m.group(1))
    return out


def check_pre_push(root: str, guards: list[dict]) -> list[str]:
    problems = []
    names = {g["target"] for g in guards} | guard_help_targets(root)
    text = read(root, PRE_PUSH)
    loops = 0
    for n, line in enumerate(text.split("\n"), 1):
        s = line.strip()
        if not s.startswith("run_check "):
            continue
        if s == PRE_PUSH_LOOP:
            loops += 1
            continue
        why = None
        if "--self-test" in s:
            why = "runs a self-test"
        elif re.search(r"(^|[\s/])scripts/check-", s):
            why = "runs a scripts/check-* script"
        elif ".claude/hooks/" in s:
            why = "runs a hook's check"
        else:
            m = re.search(r"\bmake\b(?:\s+-\S+|\s+\w+=\S+)*\s+([A-Za-z0-9_-]+)", s)
            if m and m.group(1) in names:
                why = f"runs the guard {m.group(1)}"
        if why:
            problems.append(f"{PRE_PUSH}:{n}: {why} outside the registry; add a guard file in {GUARD_DIR}/")
    if loops != 1:
        problems.append(f"{PRE_PUSH}: expected the registry loop ('{PRE_PUSH_LOOP}') once, found {loops}")
    return problems


def job_block(text: str, job: str) -> list[str] | None:
    lines = text.split("\n")
    start = None
    for i, l in enumerate(lines):
        if l == f"  {job}:":
            start = i
            break
    if start is None:
        return None
    end = len(lines)
    for j in range(start + 1, len(lines)):
        if re.match(r"^  [A-Za-z0-9_-]+:\s*$", lines[j]) or re.match(r"^\S", lines[j]):
            end = j
            break
    return lines[start:end]


def step_runs(block: list[str]) -> list[str]:
    """Every `run:` of a job, a block scalar joined into one string."""
    runs = []
    i = 0
    while i < len(block):
        m = re.match(r"^(\s*)(?:- )?run:\s*(.*)$", block[i])
        i += 1
        if not m:
            continue
        indent, value = len(m.group(1)), m.group(2).strip()
        if value in ("|", ">", "|-", ">-"):
            body = []
            while i < len(block) and (not block[i].strip() or len(block[i]) - len(block[i].lstrip()) > indent):
                body.append(block[i].strip())
                i += 1
            value = "\n".join(b for b in body if b)
        runs.append(value)
    return runs


def check_workflow(root: str) -> list[str]:
    block = job_block(read(root, WORKFLOW), CI_JOB)
    if block is None:
        return [f"{WORKFLOW}: no job '{CI_JOB}' (Repo guards)"]
    problems = []
    runs = step_runs(block)
    for r in runs:
        if r != RUNNER:
            first = r.split("\n")[0]
            problems.append(
                f"{WORKFLOW}: the Repo guards job runs '{first}' outside the registry; "
                f"add a guard file in {GUARD_DIR}/ with ci in its GUARD_LANES"
            )
    if runs.count(RUNNER) != 1:
        problems.append(f"{WORKFLOW}: the Repo guards job must run '{RUNNER}' once, found {runs.count(RUNNER)}")
    return problems


def check(root: str) -> list[str]:
    guards, problems = check_guards(root)
    try:
        targets, lanes = make_database(root)
    except RuntimeError as e:
        problems.append(str(e))
    else:
        problems += check_commands(root, guards, targets)
        problems += check_lanes(guards, lanes)
    problems += check_makefile(root, guards)
    problems += check_pre_push(root, guards)
    problems += check_workflow(root)
    return problems


# ── Self-test ────────────────────────────────────────────────────────────────

GOOD_GUARD = """# Why it exists.
GUARDS += check-foo
GUARD_LANES.check-foo := lint pre-push ci
GUARD_PATHS.check-foo = ^foo/ \\.foo$
check-foo: prep ## Guard: foo holds
\t@./scripts/check-foo.sh --self-test
\t@./scripts/check-foo.sh
"""

GOOD_MAKEFILE = """.DEFAULT_GOAL := help
help:
\t@true
include scripts/lib/guards.mk
lint: $(GUARDS_LINT) ## Lint
\t@true
prep:
\t@true
check-module-boundaries: ## Check import boundaries
\t@true
"""

GOOD_PRE_PUSH = """#!/bin/bash
while IFS=$'\\t' read -r -u 3 target desc paths; do
    run_check "$desc" make --no-print-directory "$target"
done 3<<< "$GUARD_LIST"
run_check "Module boundaries" make check-module-boundaries
"""

GOOD_WORKFLOW = """on: pull_request
jobs:
  docs-guard:
    name: Repo guards
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - name: Run the repo guards
        run: ./scripts/run-guards.sh ci
  other:
    runs-on: ubuntu-latest
    steps:
      - run: ./scripts/check-foo.sh
"""


def write(root: str, rel: str, text: str) -> None:
    path = os.path.join(root, rel)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)


def good_tree(root: str) -> None:
    write(root, "Makefile", GOOD_MAKEFILE)
    write(root, "scripts/lib/guards.mk", read(ROOT, "scripts/lib/guards.mk"))
    write(root, f"{GUARD_DIR}/check-foo.mk", GOOD_GUARD)
    write(root, "scripts/check-foo.sh", "#!/bin/sh\n")
    write(root, PRE_PUSH, GOOD_PRE_PUSH)
    write(root, WORKFLOW, GOOD_WORKFLOW)


def self_test() -> int:
    failures = []
    with tempfile.TemporaryDirectory() as root:
        good_tree(root)
        problems = check(root)
        if problems:
            failures.append("the good tree is reported: " + "; ".join(problems))

    guard = f"{GUARD_DIR}/check-foo.mk"
    breakages = {
        "a guard file registering another target": (
            guard, GOOD_GUARD.replace("GUARDS += check-foo", "GUARDS += check-bar"), "must register exactly"),
        "an unknown lane": (
            guard, GOOD_GUARD.replace("lint pre-push ci", "lint nightly"), "unknown lane 'nightly'"),
        "no lane": (
            guard, GOOD_GUARD.replace("lint pre-push ci", ""), "names no lane"),
        "GUARD_PATHS set with :=": (
            guard, GOOD_GUARD.replace("GUARD_PATHS.check-foo =", "GUARD_PATHS.check-foo :="), "must be set with '='"),
        "a path pattern that does not compile": (
            guard, GOOD_GUARD.replace("^foo/", "^foo/(["), "does not compile"),
        "a rule with no description": (
            guard, GOOD_GUARD.replace(" ## Guard: foo holds", ""), "no '## <description>'"),
        "a second rule in a guard file": (
            guard, GOOD_GUARD + "check-bar:\n\t@true\n", "exactly one rule"),
        "a recipe running a missing script": (
            guard, GOOD_GUARD.replace("scripts/check-foo.sh\n", "scripts/check-gone.sh\n"),
            "scripts/check-gone.sh, which does not exist"),
        "a missing prerequisite": (
            guard, GOOD_GUARD.replace("check-foo: prep", "check-foo: nothing-here"), "'nothing-here' is neither"),
        "a stray file in the registry": (
            f"{GUARD_DIR}/README", "notes\n", "not a guard file"),
        "a guard wired into lint directly": (
            "Makefile", GOOD_MAKEFILE.replace("lint: $(GUARDS_LINT)", "lint: $(GUARDS_LINT) check-extra"),
            "wire check-extra through a guard file"),
        "a Makefile that does not include the registry": (
            "Makefile", GOOD_MAKEFILE.replace("include scripts/lib/guards.mk\n", "").replace(
                "lint: $(GUARDS_LINT)", "lint:"), "does not 'include scripts/lib/guards.mk'"),
        "a registered guard defined again in the Makefile": (
            "Makefile", GOOD_MAKEFILE + "check-foo:\n\t@true\n", "Makefile: defines check-foo"),
        "a self-test run in pre-push": (
            PRE_PUSH, GOOD_PRE_PUSH + 'run_check "Foo" ./scripts/foo.sh --self-test\n', "runs a self-test"),
        "a scripts/check-* script run in pre-push": (
            PRE_PUSH, GOOD_PRE_PUSH + 'run_check "Foo" ./scripts/check-foo.sh\n', "runs a scripts/check-* script"),
        "a registered guard run in pre-push by name": (
            PRE_PUSH, GOOD_PRE_PUSH + 'run_check "Foo" make --no-print-directory check-foo\n',
            "runs the guard check-foo"),
        "pre-push without the registry loop": (
            PRE_PUSH, GOOD_PRE_PUSH.replace('run_check "$desc"', 'echo "$desc"'), "expected the registry loop"),
        "a guard step in the Repo guards job": (
            WORKFLOW, GOOD_WORKFLOW.replace(
                "      - name: Run the repo guards\n",
                "      - name: Foo\n        run: ./scripts/check-foo.sh\n      - name: Run the repo guards\n"),
            "runs './scripts/check-foo.sh' outside the registry"),
        "a block-scalar guard step in the Repo guards job": (
            WORKFLOW, GOOD_WORKFLOW.replace(
                "      - name: Run the repo guards\n",
                "      - name: Foo\n        run: |\n          python3 scripts/foo.py --self-test\n"
                "      - name: Run the repo guards\n"),
            "runs 'python3 scripts/foo.py --self-test' outside the registry"),
        "a Repo guards job without the runner": (
            WORKFLOW, GOOD_WORKFLOW.replace("./scripts/run-guards.sh ci", "./scripts/run-guards.sh lint"),
            "must run './scripts/run-guards.sh ci' once"),
    }
    for name, (rel, text, expected) in breakages.items():
        with tempfile.TemporaryDirectory() as root:
            good_tree(root)
            write(root, rel, text)
            try:
                problems = check(root)
            except RuntimeError as e:
                problems = [str(e)]
            if not any(expected in p for p in problems):
                failures.append(f"{name}: expected a problem containing {expected!r}, got {problems}")

    if failures:
        for f in failures:
            print(f"✗ self-test: {f}", file=sys.stderr)
        return 1
    print(f"✓ check-guard-registry self-test: the good tree passes and {len(breakages)} breakages are reported")
    return 0


def main() -> int:
    if sys.argv[1:] == ["--self-test"]:
        return self_test()
    if sys.argv[1:]:
        print(__doc__, file=sys.stderr)
        return 2
    try:
        problems = check(ROOT)
    except (OSError, RuntimeError) as e:
        print(f"check-guard-registry: {e}", file=sys.stderr)
        return 2
    if problems:
        print("check-guard-registry: the guard registry is inconsistent:", file=sys.stderr)
        for p in problems:
            print(f"  {p}", file=sys.stderr)
        return 1
    print("✓ check-guard-registry: every guard is registered once, under scripts/guards/")
    return 0


if __name__ == "__main__":
    sys.exit(main())
