#!/usr/bin/env python3
"""Guard: every package `test` script is run by a pull request workflow.

A package.json `test` script that no CI job runs is a suite whose failures
merge. That happened three times: packages/app (#1533), the Storybook mock
adapter (#1801), and two RTL tests in packages/ui that failed on main for
the whole time nothing ran that package.

    scripts/check-test-scripts.py              # check the repository
    scripts/check-test-scripts.py --self-test  # prove it on scratch trees

What it reads:

  * every tracked package.json with a `test` script (node_modules aside);
  * every workflow under .github/workflows that runs on `pull_request`, and in
    it every step whose job and step `if:` do not exclude pull requests;
  * every make target such a step names, followed through prerequisites,
    `$(MAKE) [-C dir] target` lines and `bash script.sh` calls.

A suite counts as run when a command in the package's own directory is one
of `vp test`, `vp run test`, `vpr test`, `pnpm [run] test`, `npm [run] test`,
or the text of the script itself (`$(VPX) tsx --test ...`). A `vp test`
given a file or directory filter runs part of the suite and does not count.
The directory is the step's `working-directory`, the make directory, and
any `cd <dir>` before the command on the same line.

EXEMPT names a suite that cannot run in a pull request job, with the reason.
An entry for a package that has no `test` script, or whose suite is run, is
an error, so the list cannot outlive its reason.

Exit status: 0 every suite is run, 1 a suite is not, 2 an input is unreadable.
"""

from __future__ import annotations

import json
import os
import re
import shlex
import subprocess
import sys
import tempfile

try:
    import yaml
except ImportError:  # pragma: no cover
    print("check-test-scripts: needs PyYAML (pip install pyyaml)", file=sys.stderr)
    sys.exit(2)

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Suites no pull request job runs. Each entry is a package directory and the
# reason it cannot run there. Keep this list as short as it can be.
EXEMPT: dict[str, str] = {
    # Playwright against a running Bowrain server, Keycloak and PostgreSQL.
    # e2e.yml builds four images for that stack and runs the suite nightly
    # and on demand; a pull request does not wait on it.
    "bowrain/e2e/cloud": "needs the full Docker stack; e2e.yml runs it nightly",
}

RUNNERS = (
    ("vp", "test"),
    ("vp", "run", "test"),
    ("vpr", "test"),
    ("pnpm", "test"),
    ("pnpm", "run", "test"),
    ("npm", "test"),
    ("npm", "run", "test"),
)
# Prefixes that run a binary from node_modules/.bin; stripped before the
# command is compared with the script text.
EXEC_PREFIXES = (("vpx",), ("npx",), ("pnpm", "exec"), ("vp", "exec"))
PR_EXCLUDING_TERM = re.compile(
    r"github\.event_name\s*!=\s*'pull_request'"
    r"|github\.event_name\s*==\s*'(push|schedule|workflow_dispatch|release)'"
)


def excludes_pull_requests(condition: object) -> bool:
    """Report whether an `if:` keeps its job or step off pull requests.

    It does when it holds a term such as `github.event_name != 'pull_request'`
    and no `||`. With an `||` the other side can admit a pull request
    (`event_name != 'pull_request' || needs.changes.outputs.cli == 'true'`),
    so the step counts as run.
    """
    text = str(condition or "")
    return bool(PR_EXCLUDING_TERM.search(text)) and "||" not in text


class Unreadable(Exception):
    """An input has a shape the guard does not read."""


# ── Packages ────────────────────────────────────────────────────────────────


def package_jsons(root: str) -> list[str]:
    """Return repo-relative paths of package.json files, tracked ones in a repo."""
    try:
        out = subprocess.run(
            ["git", "-C", root, "ls-files", "-z", "--", "package.json", "*/package.json"],
            check=True, capture_output=True, text=True,
        ).stdout
        files = [f for f in out.split("\0") if f]
    except (subprocess.CalledProcessError, FileNotFoundError):
        files = []
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [d for d in dirnames if d not in ("node_modules", ".git")]
            if "package.json" in filenames:
                files.append(os.path.relpath(os.path.join(dirpath, "package.json"), root))
    return sorted(f for f in files if "node_modules/" not in f)


def test_scripts(root: str) -> dict[str, str]:
    """Map each package directory with a `test` script to the script text."""
    suites = {}
    for rel in package_jsons(root):
        try:
            with open(os.path.join(root, rel)) as fh:
                pkg = json.load(fh)
        except (OSError, ValueError) as err:
            raise Unreadable(f"{rel}: {err}") from err
        script = (pkg.get("scripts") or {}).get("test")
        if script:
            suites[os.path.dirname(rel) or "."] = script
    return suites


# ── Commands ────────────────────────────────────────────────────────────────


def norm(path: str) -> str:
    path = os.path.normpath(path)
    return "." if path in ("", ".") else path


def split_commands(line: str) -> list[str]:
    """Split one shell line into the commands joined by &&, || and ;."""
    return [c.strip() for c in re.split(r"&&|\|\||;", line) if c.strip()]


def words(command: str) -> list[str]:
    try:
        return shlex.split(command, comments=True)
    except ValueError:
        return command.split()


def runs_suite(argv: list[str], script: str) -> bool:
    """Report whether argv runs the whole of a package's test script."""
    for runner in RUNNERS:
        n = len(runner)
        if tuple(argv[:n]) != runner:
            continue
        rest = argv[n:]
        if runner == ("vp", "test") and rest[:1] == ["run"]:
            rest = rest[1:]
        # Anything after `--` goes to the runner as flags; a bare word before it
        # is a file or directory filter.
        positional = []
        for word in rest:
            if word == "--":
                break
            if not word.startswith("-"):
                positional.append(word)
        return not positional
    want = words(script)
    for prefix in EXEC_PREFIXES:
        if tuple(argv[: len(prefix)]) == prefix:
            argv = argv[len(prefix):]
            break
    if not want or argv[: len(want)] != want:
        return False
    # Extra flags and forwarded arguments keep the whole suite; an extra bare
    # word is a filter.
    return all(w.startswith("-") or w in ('$@', '"$@"') for w in argv[len(want):])


class Walker:
    """Collects the directories a test runner is started in."""

    def __init__(self, root: str, suites: dict[str, str]):
        self.root = root
        self.suites = suites
        self.covered: dict[str, str] = {}
        self.makefiles: dict[str, dict] = {}
        self.seen_targets: set[tuple[str, str]] = set()
        self.seen_scripts: set[str] = set()

    # A command is run in cwd, a repo-relative directory.
    def command(self, command: str, cwd: str, where: str) -> str:
        argv = words(command)
        if not argv:
            return cwd
        if argv[0] == "cd" and len(argv) >= 2:
            return norm(os.path.join(cwd, argv[1]))
        if argv[0] in ("make", "$(MAKE)", "${MAKE}"):
            self.make(argv[1:], cwd, where)
            return cwd
        if argv[0] in ("bash", "sh") and len(argv) >= 2 and argv[1].endswith(".sh"):
            self.script(norm(os.path.join(cwd, argv[1])), cwd, where)
            return cwd
        if argv[0].endswith(".sh") and "/" in argv[0]:
            self.script(norm(os.path.join(cwd, argv[0])), cwd, where)
            return cwd
        script = self.suites.get(cwd)
        if script is not None and runs_suite(argv, script):
            self.covered.setdefault(cwd, where)
        return cwd

    def line(self, line: str, cwd: str, where: str) -> str:
        """Run one line from cwd and return the directory it ends in."""
        for command in split_commands(line):
            cwd = self.command(command, cwd, where)
        return cwd

    def script(self, path: str, cwd: str, where: str) -> None:
        full = os.path.join(self.root, path)
        if path in self.seen_scripts or not os.path.isfile(full):
            return
        self.seen_scripts.add(path)
        here = os.path.dirname(path) or "."
        with open(full) as fh:
            text = fh.read().replace("\\\n", " ")
        for raw in text.splitlines():
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            # The usual "run from the script's own directory" idioms.
            line = re.sub(r'cd\s+"?\$\(dirname\s+"?\$0"?\)"?', f"cd {here}", line)
            line = re.sub(r'cd\s+"?\$\{?SCRIPT_DIR\}?"?', f"cd {here}", line)
            # A script is one shell: a `cd` holds for the lines after it.
            cwd = self.line(line, cwd, f"{where} > {path}")

    # ── make ──

    def makefile(self, directory: str) -> dict:
        if directory in self.makefiles:
            return self.makefiles[directory]
        path = os.path.join(self.root, directory, "Makefile")
        parsed = {"vars": {}, "targets": {}}
        if os.path.isfile(path):
            with open(path) as fh:
                parsed = parse_makefile(fh.read())
        self.makefiles[directory] = parsed
        return parsed

    def make(self, args: list[str], cwd: str, where: str) -> None:
        directory, targets, i = cwd, [], 0
        while i < len(args):
            arg = args[i]
            if arg == "-C" and i + 1 < len(args):
                directory = norm(os.path.join(directory, args[i + 1]))
                i += 2
                continue
            if arg.startswith("-C") and len(arg) > 2:
                directory = norm(os.path.join(directory, arg[2:]))
            elif arg.startswith("--directory="):
                directory = norm(os.path.join(directory, arg.split("=", 1)[1]))
            elif arg in ("-j", "-f", "-l") and i + 1 < len(args):
                i += 1
            elif not arg.startswith("-") and "=" not in arg:
                targets.append(arg)
            i += 1
        for target in targets:
            self.target(directory, target, where)

    def target(self, directory: str, name: str, where: str) -> None:
        key = (directory, name)
        if key in self.seen_targets:
            return
        self.seen_targets.add(key)
        mk = self.makefile(directory)
        rule = mk["targets"].get(name)
        if rule is None:
            return
        label = f"make -C {directory} {name}" if directory != "." else f"make {name}"
        for prereq in rule["prereqs"]:
            self.target(directory, expand(prereq, mk["vars"], name), where)
        for raw in rule["recipe"]:
            line = expand(raw, mk["vars"], name).lstrip("@-+ \t")
            self.line(line, directory, f"{where} > {label}")


def expand(text: str, variables: dict[str, str], target: str, depth: int = 0) -> str:
    text = text.replace("$@", target).replace("$$", "$")
    if depth > 8:
        return text

    def sub(m: re.Match) -> str:
        name = m.group(1) or m.group(2)
        if name == "MAKE":
            return "$(MAKE)"
        if name in variables:
            return expand(variables[name], variables, target, depth + 1)
        return m.group(0)

    return re.sub(r"\$\(([A-Za-z0-9_]+)\)|\$\{([A-Za-z0-9_]+)\}", sub, text)


def parse_makefile(text: str) -> dict:
    """Read variable assignments, rules, prerequisites and recipes."""
    variables: dict[str, str] = {}
    targets: dict[str, dict] = {}
    current: list[dict] = []
    lines = text.replace("\\\n", " ").splitlines()
    for line in lines:
        if line.startswith("\t"):
            for rule in current:
                stripped = line.strip()
                if stripped and not stripped.startswith("#"):
                    rule["recipe"].append(stripped)
            continue
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        assign = re.match(r"^(?:export\s+)?([A-Za-z0-9_]+)\s*(::=|:=|\?=|\+=|=)\s*(.*)$", line)
        if assign:
            name, op, value = assign.group(1), assign.group(2), assign.group(3).split(" #")[0].strip()
            if op == "+=" and name in variables:
                variables[name] += " " + value
            elif op != "?=" or name not in variables:
                variables[name] = value
            current = []
            continue
        rule = re.match(r"^([^:=#\t][^:=#]*?)\s*::?(?!=)\s*(.*)$", line)
        if rule:
            body = rule.group(2).split("##")[0].split("#")[0]
            if re.match(r"^\s*[A-Za-z0-9_]+\s*[:?+]?=", body):
                current = []  # a target-specific variable
                continue
            body, _, inline = body.partition(";")
            prereqs = [p for p in body.replace("|", " ").split() if p]
            current = []
            for name in rule.group(1).split():
                entry = targets.setdefault(name, {"prereqs": [], "recipe": []})
                entry["prereqs"].extend(prereqs)
                if inline.strip():
                    entry["recipe"].append(inline.strip())
                current.append(entry)
            continue
        current = []
    return {"vars": variables, "targets": targets}


# ── Workflows ───────────────────────────────────────────────────────────────


def runs_on_pull_request(doc: dict) -> bool:
    on = doc.get("on", doc.get(True))  # YAML 1.1 reads a bare `on` as True
    if isinstance(on, str):
        return on == "pull_request"
    if isinstance(on, list):
        return "pull_request" in on
    if isinstance(on, dict):
        return "pull_request" in on
    return False


def walk_workflows(root: str, walker: Walker) -> None:
    directory = os.path.join(root, ".github", "workflows")
    if not os.path.isdir(directory):
        return
    for name in sorted(os.listdir(directory)):
        if not name.endswith((".yml", ".yaml")):
            continue
        rel = f".github/workflows/{name}"
        try:
            with open(os.path.join(directory, name)) as fh:
                doc = yaml.safe_load(fh)
        except yaml.YAMLError as err:
            raise Unreadable(f"{rel}: {err}") from err
        if not isinstance(doc, dict) or not runs_on_pull_request(doc):
            continue
        for job_id, job in (doc.get("jobs") or {}).items():
            if not isinstance(job, dict) or excludes_pull_requests(job.get("if")):
                continue
            defaults = ((job.get("defaults") or {}).get("run") or {}).get("working-directory", ".")
            for step in job.get("steps") or []:
                if not isinstance(step, dict) or "run" not in step:
                    continue
                if excludes_pull_requests(step.get("if")):
                    continue
                cwd = norm(step.get("working-directory", defaults))
                where = f"{rel} job {job_id}"
                text = str(step["run"]).replace("\\\n", " ")
                # A step's run block is one shell, so a `cd` carries forward.
                for line in text.splitlines():
                    if line.strip() and not line.strip().startswith("#"):
                        cwd = walker.line(line.strip(), cwd, where)


# ── Check ───────────────────────────────────────────────────────────────────


def check(root: str, exempt: dict[str, str]) -> tuple[list[str], dict[str, str]]:
    """Return the problems and the map of covered suite to what runs it."""
    suites = test_scripts(root)
    walker = Walker(root, suites)
    walk_workflows(root, walker)
    problems = []
    for directory in sorted(suites):
        if directory in walker.covered:
            if directory in exempt:
                problems.append(f"{directory}: exempt, but {walker.covered[directory]} runs it; drop the entry")
            continue
        if directory in exempt:
            continue
        problems.append(
            f"{directory}: its test script ({suites[directory]}) runs in no pull request job"
        )
    for directory in sorted(exempt):
        if directory not in suites:
            problems.append(f"{directory}: exempt, but it has no test script; drop the entry")
    return problems, walker.covered


def main_check(root: str) -> int:
    try:
        problems, covered = check(root, EXEMPT)
    except Unreadable as err:
        print(f"check-test-scripts: cannot read {err}", file=sys.stderr)
        return 2
    if problems:
        print("check-test-scripts: a package test suite runs in no pull request job\n")
        for p in problems:
            print("  " + p)
        print(
            "\nRun it from a make target a ci.yml job calls (ci-frontend-packages is the\n"
            "home for workspace packages), in the job whose path filter covers what\n"
            "the suite imports. EXEMPT in scripts/check-test-scripts.py is only for a\n"
            "suite that cannot run in CI, with the reason."
        )
        return 1
    print(f"check-test-scripts: OK ({len(covered)} suites run, {len(EXEMPT)} exempt)")
    return 0


# ── Self-test ───────────────────────────────────────────────────────────────


def write(root: str, rel: str, text: str) -> None:
    path = os.path.join(root, rel)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as fh:
        fh.write(text)


def pkg(test: str | None) -> str:
    return json.dumps({"name": "x", "scripts": {"test": test} if test else {"build": "x"}})


SELF_TEST_MAKEFILE = """\
DESK_DIR := apps/desk
VPX ?= vp exec

ci-web: deps ## the job body
\tcd packages/a && vp test
\t@$(MAKE) --no-print-directory ci-more
\tcd web && vp test run src/pages/one

ci-more:
\tcd $(DESK_DIR)/frontend && vp run test

deps:
\tcd packages/via-prereq && vpr test

ci-sub:
\t$(MAKE) -C tool check

ci-script:
\tbash e2e/run.sh

unrun:
\tcd packages/orphan && vp test
"""

SELF_TEST_TOOL_MAKEFILE = """\
VPX ?= vp exec
check: test
test:
\t$(VPX) tsx --test "src/**/*.test.ts"
"""

SELF_TEST_CI = """\
name: CI
on:
  pull_request:
jobs:
  web:
    runs-on: ubuntu-latest
    steps:
      - run: make ci-web ci-sub
      - run: make ci-script
  docs:
    runs-on: ubuntu-latest
    steps:
      - name: in a working directory
        working-directory: packages/wd
        run: |
          vp check
          vp test -- --run --passWithNoTests
      - name: only on push
        if: github.event_name != 'pull_request'
        run: cd packages/push-only && vp test
  either:
    if: ${{ !cancelled() && (github.event_name != 'pull_request' || needs.changes.outputs.x == 'true') }}
    runs-on: ubuntu-latest
    steps:
      - run: |
          cd packages/either
          vp test
  pushjob:
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    steps:
      - run: cd packages/push-job && vp test
"""

SELF_TEST_NIGHTLY = """\
name: Nightly
on:
  schedule:
    - cron: "0 2 * * *"
jobs:
  n:
    runs-on: ubuntu-latest
    steps:
      - run: cd packages/nightly && vp test
"""

SELF_TEST_RUN_SH = """\
#!/usr/bin/env bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"
vpx playwright test "$@"
"""


def self_test() -> int:
    failures = []
    with tempfile.TemporaryDirectory() as root:
        covered_dirs = {
            "packages/a": "vp test",
            "apps/desk/frontend": "vp test",
            "packages/via-prereq": "vp test",
            "packages/wd": "vp test",
            "packages/either": "vp test",  # a cd on the line before
            "tool": 'tsx --test "src/**/*.test.ts"',
            "e2e": "playwright test",
        }
        uncovered_dirs = {
            "web": "vp test",  # only a filtered run
            "packages/orphan": "vp test",  # in a target no job calls
            "packages/push-only": "vp test",
            "packages/push-job": "vp test",
            "packages/nightly": "vp test",
        }
        for d, t in {**covered_dirs, **uncovered_dirs}.items():
            write(root, f"{d}/package.json", pkg(t))
        write(root, "packages/no-test/package.json", pkg(None))
        write(root, "packages/a/node_modules/dep/package.json", pkg("vp test"))
        write(root, "Makefile", SELF_TEST_MAKEFILE)
        write(root, "tool/Makefile", SELF_TEST_TOOL_MAKEFILE)
        write(root, "e2e/run.sh", SELF_TEST_RUN_SH)
        write(root, ".github/workflows/ci.yml", SELF_TEST_CI)
        write(root, ".github/workflows/nightly.yml", SELF_TEST_NIGHTLY)

        problems, covered = check(root, {})
        for d in covered_dirs:
            if d not in covered:
                failures.append(f"{d} should count as run")
        for d in uncovered_dirs:
            if not any(p.startswith(f"{d}:") for p in problems):
                failures.append(f"{d} should be reported")
        if any("node_modules" in p for p in problems):
            failures.append("a package under node_modules was read")
        if any(p.startswith("packages/no-test:") for p in problems):
            failures.append("a package without a test script was reported")

        exempt = {"packages/orphan": "reason", "packages/a": "stale", "packages/gone": "stale"}
        problems, _ = check(root, exempt)
        if any(p.startswith("packages/orphan:") for p in problems):
            failures.append("an exempt suite was reported")
        if not any(p.startswith("packages/a: exempt, but") for p in problems):
            failures.append("an exemption for a run suite was not reported")
        if not any(p.startswith("packages/gone: exempt, but") for p in problems):
            failures.append("an exemption for a missing suite was not reported")

    for argv, script, want in (
        (["vp", "test"], "vp test", True),
        (["vp", "test", "run"], "vp test", True),
        (["vp", "test", "--", "--run"], "vp test", True),
        (["vp", "test", "run", "src/x"], "vp test", False),
        (["vp", "check"], "vp test", False),
        (["vpx", "jest"], "jest", True),
        (["vpx", "jest", "src/one.test.ts"], "jest", False),
    ):
        if runs_suite(argv, script) != want:
            failures.append(f"runs_suite({argv}, {script!r}) should be {want}")

    if failures:
        print("check-test-scripts self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("check-test-scripts self-test: OK")
    return 0


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-test"]:
        sys.exit(self_test())
    if sys.argv[1:]:
        print(__doc__)
        sys.exit(2)
    sys.exit(main_check(ROOT))
