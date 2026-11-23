#!/usr/bin/env python3
"""Guard: only main saves an Actions cache.

A run can restore the caches saved on main and on its own ref, and nothing
else. A cache saved on a pull request, or by a run on another branch (a
workflow started by hand there, a push to a release branch), serves only
that ref, while it counts against the repository's 10 GiB and pushes out
the caches every pull request restores. So off main every cache step
restores and none saves.

Each workflow under .github/workflows is evaluated once for every kind of
run off main that its triggers admit:

  * `pull_request`, as scripts/check-test-scripts.py evaluates it;
  * `pull_request_review` and `pull_request_review_comment`, on refs/pull/;
  * `workflow_dispatch`, started by hand on a branch other than main;
  * `push` whose filter admits a branch other than main, or a tag;
  * `release`, on the release's tag.

In the runs after the first, `github.ref` is a ref other than
refs/heads/main under the prefix named, so `github.ref == 'refs/heads/main'`
is false there, `startsWith(github.ref, 'refs/tags/')` is true on a tag, and
a comparison with any other ref under that prefix may be true: a save behind
one is reported. `github.ref_type` is `branch` or `tag` to match. The inputs
of a `workflow_dispatch` are unknown, so a condition on one admits the step.

    scripts/check-ci-caches.py              check the repository
    scripts/check-ci-caches.py --self-test  prove it on fixtures

For each run it reads every job and step whose `if:` admits that run, the
steps of the local composite actions those steps use, and the jobs of the
local reusable workflows a job calls, each with the caller's inputs. A step
saves when it is:

  * actions/cache (not actions/cache/restore), or actions/cache/save;
  * actions/setup-go whose `cache` is not false (it defaults to true);
  * voidzero-dev/setup-vp with `cache` true and `cache-save` not false;
  * actions/setup-node with a `cache` input;
  * golangci/golangci-lint-action without `skip-cache` or `skip-save-cache`;
  * docker/build-push-action whose `cache-to` names the gha cache;
  * msys2/setup-msys2 whose `cache` is not false (it defaults to true);
  * aquasecurity/trivy-action whose `cache` is not false (it defaults to true
    and saves the vulnerability database through actions/cache).

An input or condition the guard cannot decide is an error naming the step,
and so is a workflow on an event it does not model (merge_group,
pull_request_target), so one cannot be added unchecked.

A step that runs `scripts/ci-cache-budget.py prune` counts as a save: it
deletes caches main's runs read, so it too runs on main only.

It also pairs BuildKit scopes: a `cache-from` scope that no `cache-to` in
any workflow writes is a read that never hits, and is reported. Matrix
values in a scope are expanded over the job's `include` rows.

Exit status: 0 nothing off main saves a cache, 1 something does, 2 an input
is unreadable or a condition cannot be decided.
"""

from __future__ import annotations

import contextlib
import importlib.util
import os
import re
import sys
import tempfile

import yaml

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

_spec = importlib.util.spec_from_file_location(
    "check_test_scripts", os.path.join(os.path.dirname(os.path.abspath(__file__)), "check-test-scripts.py"))
cts = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(cts)


class Unreadable(Exception):
    pass


class OffMainRef:
    """github.ref (or ref_name) of a run on a ref other than main."""

    def __init__(self, prefix: str, main: str):
        self.prefix, self.main = prefix, main


@contextlib.contextmanager
def off_main(event: str, prefix: str):
    """Evaluate conditions as a run of `event` on a ref under `prefix`
    (refs/heads/, refs/tags/ or refs/pull/) that is not main. The pull
    request evaluator in check-test-scripts.py reads its contexts and
    operators by name, so they are swapped for the length of the block."""
    ref = OffMainRef(prefix, "refs/heads/main")
    ref_name = OffMainRef("", "main")
    ref_type = {"refs/heads/": "branch", "refs/tags/": "tag"}.get(prefix, cts.UNDECIDED)
    saved = cts.context, cts.compare, cts.call, cts.truth

    def context(name: str) -> object:
        lower = name.lower()
        if lower == "github.event_name":
            return event
        if lower == "github.ref":
            return ref
        if lower == "github.ref_name":
            return ref_name
        if lower == "github.ref_type":
            return ref_type
        if lower in ("github.head_ref", "github.base_ref"):
            return cts.UNDECIDED if prefix == "refs/pull/" else ""
        if lower == "github.event.repository.default_branch":
            return "main"
        if lower == "github.ref_protected":
            return cts.UNDECIDED
        return cts.UNKNOWN

    def compare(op: str, a: object, b: object) -> object:
        for sentinel in (ref, ref_name):
            if sentinel in (a, b):
                other = b if a is sentinel else a
                if other is cts.UNKNOWN and op in ("==", "!="):
                    return cts.UNKNOWN
                if op not in ("==", "!=") or not isinstance(other, str):
                    return cts.UNDECIDED
                other = other.lower()
                if other == sentinel.main:
                    return op == "!="
                # A full ref under another prefix is never this run's ref.
                if sentinel.prefix and other.startswith("refs/") and not other.startswith(sentinel.prefix):
                    return op == "!="
                # Another ref under the same prefix may be the one this run is on.
                return cts.UNKNOWN
        return saved[1](op, a, b)

    def call(name: str, args: list[object]) -> object:
        if name.lower() == "startswith" and len(args) == 2 and args[0] is ref and isinstance(args[1], str):
            want = args[1].lower()
            if ref.prefix.startswith(want):
                return True
            if want.startswith(ref.prefix):
                return cts.UNKNOWN  # depends on which ref
            return False
        if ref in args or ref_name in args:
            return cts.UNDECIDED
        return saved[2](name, args)

    def truth(value: object) -> object:
        if value is ref or value is ref_name:
            return True
        return saved[3](value)

    cts.context, cts.compare, cts.call, cts.truth = context, compare, call, truth
    try:
        yield
    finally:
        cts.context, cts.compare, cts.call, cts.truth = saved


def triggers(doc: dict) -> dict:
    on = doc.get("on", doc.get(True))  # YAML 1.1 reads a bare `on` as True
    if isinstance(on, str):
        return {on: {}}
    if isinstance(on, list):
        return {t: {} for t in on}
    if isinstance(on, dict):
        return {k: (v or {}) for k, v in on.items()}
    return {}


PRUNE_RE = re.compile(r"ci-cache-budget\.py\s+prune")

# Events whose runs are on the default branch (main): nothing to check.
# workflow_call runs as part of its caller, which walk_jobs follows.
ON_MAIN_EVENTS = {"schedule", "workflow_run", "repository_dispatch", "issue_comment", "issues", "workflow_call"}
# Events scenarios() evaluates as a run off main.
OFF_MAIN_EVENTS = {"pull_request", "pull_request_review", "pull_request_review_comment",
                   "workflow_dispatch", "push", "release"}


def scenarios(doc: dict) -> list[tuple[str, str, str]]:
    """Return (label, event, ref prefix) for each kind of off-main run.
    An empty prefix is the pull request evaluator of check-test-scripts.py.
    An event in neither list above (merge_group, pull_request_target, ...)
    raises Unreadable, so a workflow cannot add one unchecked."""
    on = triggers(doc)
    unknown = sorted(set(on) - ON_MAIN_EVENTS - OFF_MAIN_EVENTS, key=str)
    if unknown:
        raise Unreadable(f"runs on {', '.join(map(str, unknown))}, which the guard does not model: "
                         "say in scripts/check-ci-caches.py which ref such a run is on")
    out = []
    if "pull_request" in on:
        out.append(("a pull request", "pull_request", ""))
    for event in ("pull_request_review", "pull_request_review_comment"):
        if event in on:
            out.append((f"a {event.replace('_', ' ')}", event, "refs/pull/"))
    if "workflow_dispatch" in on:
        out.append(("a run started by hand on a branch", "workflow_dispatch", "refs/heads/"))
    if "push" in on:
        push = on["push"] if isinstance(on["push"], dict) else {}
        has_branch_filter = "branches" in push or "branches-ignore" in push
        has_tag_filter = "tags" in push or "tags-ignore" in push
        # A push with a filter for one kind of ref only runs for that kind.
        if has_tag_filter or not has_branch_filter:
            if push.get("tags") != []:
                out.append(("a push of a tag", "push", "refs/tags/"))
        if has_branch_filter or not has_tag_filter:
            branches = push.get("branches")
            if branches is None or any(str(b) != "main" for b in branches):
                out.append(("a push to a branch", "push", "refs/heads/"))
    if "release" in on:
        out.append(("a release", "release", "refs/tags/"))
    return out


def substitute(value: object, inputs: dict[str, str] | None) -> object:
    """Replace `inputs.<name>` in an expression with the caller's literal.
    With inputs None (a run started by hand, whose inputs are unknown) the
    expression keeps its references, which evaluate as unknown."""
    if inputs is None or not isinstance(value, str) or "inputs." not in value:
        return value

    def lit(m: re.Match) -> str:
        v = inputs.get(m.group(1))
        if v is None:
            return "null"
        return "'" + str(v).replace("'", "''") + "'"

    return re.sub(r"\binputs\.([A-Za-z0-9_-]+)", lit, value)


def on_pr(value: object, inputs: dict[str, str], where: str) -> object:
    """Evaluate an input or condition on a pull_request event: True, False or
    cts.UNKNOWN."""
    value = substitute(value, inputs)
    try:
        if isinstance(value, str) and "${{" not in value and not re.search(r"[=!&|(]", value):
            # A plain value, not an expression.
            return value.strip().lower() not in ("false", "0", "")
        return cts.evaluate_on_pull_request(value)
    except ValueError as err:
        raise Unreadable(f"{where}: {value!r}: {err}") from err


def truthy_input(with_: dict, name: str, default: bool, inputs: dict[str, str], where: str) -> object:
    if name not in with_:
        return default
    return on_pr(with_[name], inputs, f"{where} input {name}")


def saves(uses: str, with_: dict, inputs: dict[str, str], where: str) -> object:
    """Report whether a step that uses `uses` saves a cache on a pull request:
    True, False or cts.UNKNOWN."""
    name = uses.split("@", 1)[0]
    if name in ("actions/cache", "actions/cache/save"):
        return True
    if name == "actions/setup-go":
        return truthy_input(with_, "cache", True, inputs, where)
    if name == "voidzero-dev/setup-vp":
        cache = truthy_input(with_, "cache", False, inputs, where)
        if cache is False:
            return False
        save = truthy_input(with_, "cache-save", True, inputs, where)
        return cts.logical_and(cache, save)
    if name == "actions/setup-node":
        return "cache" in with_ and on_pr(with_["cache"], inputs, f"{where} input cache")
    if name == "aquasecurity/trivy-action":
        return truthy_input(with_, "cache", True, inputs, where)
    if name == "golangci/golangci-lint-action":
        skip = truthy_input(with_, "skip-cache", False, inputs, where)
        skip_save = truthy_input(with_, "skip-save-cache", False, inputs, where)
        return cts.logical_not(cts.logical_or(skip, skip_save))
    if name == "docker/build-push-action":
        to = with_.get("cache-to")
        if not to:
            return False
        text = str(to)
        if "type=gha" not in text:
            return False
        if re.fullmatch(r"\s*\$\{\{.*\}\}\s*", text, re.S):
            return on_pr(to, inputs, f"{where} input cache-to")
        # type=gha written outside an expression saves whatever the rest says.
        outside = re.sub(r"\$\{\{.*?\}\}", "", text, flags=re.S)
        return True if "type=gha" in outside else cts.UNKNOWN
    if name == "msys2/setup-msys2":
        return truthy_input(with_, "cache", True, inputs, where)
    return False


def load(path: str) -> dict:
    try:
        with open(path) as fh:
            doc = yaml.safe_load(fh)
    except (OSError, yaml.YAMLError) as err:
        raise Unreadable(f"{path}: {err}") from err
    return doc if isinstance(doc, dict) else {}


def walk_steps(root: str, steps: list, inputs: dict[str, str], where: str, problems: list[str], seen: set) -> None:
    for n, step in enumerate(steps or [], 1):
        if not isinstance(step, dict):
            continue
        label = f"{where} step {step.get('name') or step.get('id') or n}"
        if "uses" not in step:
            # The prune deletes caches main's runs read: it runs on main only,
            # like a save.
            if PRUNE_RE.search(str(step.get("run") or "")):
                cond = substitute(step.get("if"), inputs)
                if not cts.excludes_pull_requests(cond, label):
                    problems.append(f"{label}: runs the cache prune on {SCENARIO[0]}")
            continue
        cond = substitute(step.get("if"), inputs)
        if cts.excludes_pull_requests(cond, label):
            continue
        uses = str(step["uses"])
        with_ = step.get("with") or {}
        if uses.startswith("./"):
            local = uses[2:]
            # A job that checks this repository out into a subdirectory
            # (contract-audit's ./neokapi) names the action under it.
            if not local.startswith(".github/") and "/.github/actions/" in local:
                local = local[local.index(".github/actions/"):]
            action = os.path.join(root, local, "action.yml")
            if not os.path.isfile(action):
                action = os.path.join(root, local, "action.yaml")
            doc = load(action)
            runs = doc.get("runs") or {}
            if runs.get("using") != "composite":
                continue
            resolved = {}
            for name, spec in (doc.get("inputs") or {}).items():
                default = (spec or {}).get("default") if isinstance(spec, dict) else None
                value = with_.get(name, default)
                resolved[name] = substitute(value, inputs) if value is not None else None
            key = (action, tuple(sorted((k, str(v)) for k, v in resolved.items())))
            if key in seen:
                continue
            seen.add(key)
            walk_steps(root, runs.get("steps"), resolved, f"{label} > {uses}", problems, seen)
            continue
        verdict = saves(uses, with_, inputs, label)
        if verdict is True:
            problems.append(f"{label}: {uses} saves a cache on {SCENARIO[0]}")
        elif verdict is cts.UNKNOWN:
            problems.append(f"{label}: {uses} may save a cache on {SCENARIO[0]} (its inputs turn on a value the run does not fix)")


def walk_jobs(root: str, rel: str, doc: dict, inputs: dict[str, str] | None, where: str,
              problems: list[str], called: tuple[str, ...]) -> None:
    """Walk the jobs of a workflow that admit the current run. A job that
    calls a local reusable workflow walks that workflow's jobs with the
    inputs it passes, defaulted from the callee's `workflow_call` inputs."""
    jobs = {}
    for job_id, job in (doc.get("jobs") or {}).items():
        if isinstance(job, dict):
            jobs[job_id] = dict(job, **{"if": substitute(job["if"], inputs)}) if "if" in job else job
    for job_id in sorted(set(jobs) - cts.skipped_jobs(where, jobs), key=list(jobs).index):
        job = jobs[job_id]
        label = f"{where} job {job_id}"
        uses = job.get("uses")
        if isinstance(uses, str) and uses.startswith("./"):
            target = uses[2:].split("@", 1)[0]
            if target in called:
                raise Unreadable(f"{label}: {target} calls itself through {' > '.join(called)}")
            callee = load(os.path.join(root, target))
            spec = triggers(callee).get("workflow_call") or {}
            with_ = job.get("with") or {}
            resolved = {}
            for name, item in (spec.get("inputs") or {}).items():
                default = item.get("default") if isinstance(item, dict) else None
                value = with_.get(name, default)
                resolved[name] = substitute(value, inputs) if value is not None else None
            walk_jobs(root, target, callee, resolved, f"{label} > {target}", problems, called + (target,))
            continue
        walk_steps(root, job.get("steps"), inputs, label, problems, set())


def check(root: str) -> list[str]:
    problems: list[str] = []
    directory = os.path.join(root, ".github", "workflows")
    for name in sorted(os.listdir(directory)):
        if not name.endswith((".yml", ".yaml")):
            continue
        rel = f".github/workflows/{name}"
        doc = load(os.path.join(directory, name))
        try:
            runs = scenarios(doc)
        except Unreadable as err:
            raise Unreadable(f"{rel}: {err}") from err
        for label, event, prefix in runs:
            SCENARIO[0] = label
            scope = off_main(event, prefix) if prefix else contextlib.nullcontext()
            # A run started by hand has inputs nobody fixes; other runs have none.
            inputs = None if event == "workflow_dispatch" else {}
            with scope:
                try:
                    walk_jobs(root, rel, doc, inputs, rel, problems, (rel,))
                except cts.Unreadable as err:
                    raise Unreadable(f"{err} (on {label})") from err
    return problems


# The kind of run being checked, for the messages walk_steps writes.
SCENARIO = ["a pull request"]


SCOPE_RE = re.compile(r"scope=([^,\s'\"]+)")


def matrix_rows(job: dict) -> list[dict]:
    """The matrix rows of a job: the include entries, or one empty row."""
    matrix = (job.get("strategy") or {}).get("matrix") or {}
    rows = [r for r in (matrix.get("include") or []) if isinstance(r, dict)] if isinstance(matrix, dict) else []
    return rows or [{}]


def expand(text: str, row: dict) -> str:
    return re.sub(r"\$\{\{\s*matrix\.([A-Za-z0-9_-]+)\s*\}\}", lambda m: str(row.get(m.group(1), m.group(0))), text)


def check_scopes(root: str) -> list[str]:
    """A BuildKit cache-from scope must be one that a cache-to writes: only
    main writes (check() makes sure of that), and a scope nothing writes is a
    read that can never hit."""
    directory = os.path.join(root, ".github", "workflows")
    written, reads = set(), []
    for name in sorted(os.listdir(directory)):
        if not name.endswith((".yml", ".yaml")):
            continue
        rel = f".github/workflows/{name}"
        doc = load(os.path.join(directory, name))
        for job_id, job in (doc.get("jobs") or {}).items():
            if not isinstance(job, dict):
                continue
            for n, step in enumerate(job.get("steps") or [], 1):
                if not isinstance(step, dict) or not str(step.get("uses", "")).startswith("docker/build-push-action"):
                    continue
                label = f"{rel} job {job_id} step {step.get('name') or n}"
                with_ = step.get("with") or {}
                for row in matrix_rows(job):
                    written.update(SCOPE_RE.findall(expand(str(with_.get("cache-to") or ""), row)))
                    text = expand(str(with_.get("cache-from") or ""), row)
                    if "type=gha" in text or "${{" in text:
                        reads.append((label, text))
    problems = []
    for label, text in reads:
        if "${{" in text:
            problems.append(f"{label}: cache-from {text!r} names a scope the guard cannot resolve")
            continue
        for scope in SCOPE_RE.findall(text):
            if scope not in written:
                problems.append(f"{label}: cache-from reads scope {scope!r}, which no cache-to writes, so it never hits")
    return problems


def main() -> int:
    try:
        problems = check(ROOT) + check_scopes(ROOT)
    except Unreadable as err:
        print(f"check-ci-caches: cannot read {err}", file=sys.stderr)
        return 2
    if problems:
        print("check-ci-caches: a step that runs off main saves an Actions cache\n")
        for p in problems:
            print("  " + p)
        print("\nSave only on main: actions/cache/restore with\n"
              "`if: github.ref != 'refs/heads/main'` beside actions/cache with\n"
              "`if: github.ref == 'refs/heads/main'`, or the action's own switch\n"
              "(setup-go `cache: ${{ github.ref == 'refs/heads/main' }}`, setup-vp\n"
              "`cache-save: ${{ github.ref == 'refs/heads/main' }}`). A release\n"
              "workflow runs on tags, so it restores and saves nothing.")
        return 1
    print("check-ci-caches: OK")
    return 0


FIXTURE_WORKFLOW = """\
name: CI
on:
  pull_request:
  push:
    branches: [main]
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-go@v7
      - uses: actions/setup-go@v7
        with:
          cache: false
      - uses: actions/cache/restore@v6
      - name: saves on PR
        uses: actions/cache@v6
      - name: saves on push only
        if: github.event_name != 'pull_request'
        uses: actions/cache@v6
      - uses: voidzero-dev/setup-vp@v1
        with:
          cache: true
      - uses: voidzero-dev/setup-vp@v1
        with:
          cache: true
          cache-save: ${{ github.event_name != 'pull_request' }}
      - uses: ./.github/actions/local
        with:
          save: "true"
      - uses: ./.github/actions/local
      - name: undecided
        uses: actions/setup-go@v7
        with:
          cache: ${{ inputs.x == github.ref_name }}
"""

FIXTURE_DISPATCH = """\
name: E2E
on:
  workflow_dispatch:
  push:
    branches: [main]
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - name: saves on any branch
        uses: actions/cache@v6
      - name: saves on main only
        if: github.ref == 'refs/heads/main'
        uses: actions/cache@v6
      - name: restore only
        uses: actions/cache/restore@v6
      - name: cache-to on any branch
        uses: docker/build-push-action@v7
        with:
          cache-to: type=gha,mode=max
      - name: cache-to on main
        uses: docker/build-push-action@v7
        with:
          cache-to: ${{ github.ref == 'refs/heads/main' && 'type=gha,mode=max' || '' }}
      - name: saves when a dispatch input asks
        if: inputs.save == 'yes'
        uses: actions/cache@v6
  only-on-push:
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    steps:
      - name: push-only job
        uses: actions/cache@v6
  calls:
    uses: ./.github/workflows/_called.yml
    with:
      save: "no"
"""

FIXTURE_CALLED = """\
name: Called
on:
  workflow_call:
    inputs:
      save:
        type: string
        default: "yes"
jobs:
  inner:
    runs-on: ubuntu-latest
    steps:
      - name: callee saves always
        uses: actions/cache@v6
      - name: callee saves when its input asks
        if: inputs.save == 'yes'
        uses: actions/cache@v6
"""

FIXTURE_PUSH = """\
name: Release
on:
  push:
    branches: [main, "release/**"]
    tags: ["v*"]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: saves on any ref
        uses: actions/cache@v6
      - name: saves on a tag
        if: startsWith(github.ref, 'refs/tags/')
        uses: actions/cache@v6
      - name: saves on a branch
        if: github.ref_type == 'branch'
        uses: actions/cache@v6
      - name: saves on one release branch
        if: github.ref == 'refs/heads/release/1'
        uses: actions/cache@v6
      - name: saves on main only
        if: github.ref == 'refs/heads/main'
        uses: actions/cache@v6
      - name: msys2 on main only
        uses: msys2/setup-msys2@v2
        with:
          cache: ${{ github.ref == 'refs/heads/main' }}
      - name: setup-go on main only
        uses: actions/setup-go@v7
        with:
          cache: ${{ github.ref == 'refs/heads/main' }}
      - name: trivy on any ref
        uses: aquasecurity/trivy-action@0000000000000000000000000000000000000000
      - name: trivy on main only
        uses: aquasecurity/trivy-action@0000000000000000000000000000000000000000
        with:
          cache: ${{ github.ref == 'refs/heads/main' }}
"""

FIXTURE_TAGS_ONLY = """\
name: Plugin release
on:
  push:
    tags: ["sat-v*"]
  release:
    types: [published]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: tag-only save
        uses: actions/cache@v6
"""

FIXTURE_UNDECIDED = """\
  undecided:
    runs-on: ubuntu-latest
    steps:
      - name: release branch
        if: github.ref == 'refs/heads/release'
        uses: actions/cache@v6
"""

FIXTURE_REVIEW = """\
name: Review
on:
  pull_request_review:
  pull_request_review_comment:
jobs:
  answer:
    runs-on: ubuntu-latest
    steps:
      - name: saves on a review
        uses: actions/cache@v6
"""

FIXTURE_SUBDIR = """\
name: Audit
on: [pull_request]
jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - name: action under a checkout subdirectory
        uses: ./neokapi/.github/actions/local
        with:
          save: "true"
"""

FIXTURE_UNMODELLED = """\
name: Queue
on:
  merge_group:
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/cache@v6
"""

FIXTURE_SCOPES = """\
name: Images
on:
  push:
    branches: [main]
jobs:
  main-images:
    runs-on: ubuntu-latest
    steps:
      - name: writes server
        uses: docker/build-push-action@v7
        with:
          cache-from: type=gha,scope=server
          cache-to: ${{ github.ref == 'refs/heads/main' && 'type=gha,mode=max,scope=server' || '' }}
  release-images:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - { image: server, cache_from: "type=gha,scope=server" }
          - { image: web, cache_from: "type=gha,scope=web-arm64" }
          - { image: tool, cache_from: "" }
    steps:
      - name: reads per row
        uses: docker/build-push-action@v7
        with:
          cache-from: ${{ matrix.cache_from }}
"""

FIXTURE_SECOND_BRANCH = """\
name: Release caches
on:
  push:
    branches: [main, probe]
  workflow_dispatch:
jobs:
  warm:
    if: github.ref == 'refs/heads/main' || github.ref == 'refs/heads/probe'
    runs-on: ubuntu-latest
    steps:
      - name: save on a second branch
        uses: actions/cache/save@v6
      - name: prune on a second branch
        run: python3 scripts/ci-cache-budget.py prune
  main-only:
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - name: prune on main
        run: python3 scripts/ci-cache-budget.py prune
"""

FIXTURE_ACTION = """\
name: local
inputs:
  save:
    default: auto
runs:
  using: composite
  steps:
    - name: inner save
      if: inputs.save == 'true' || (inputs.save == 'auto' && github.event_name != 'pull_request')
      uses: actions/cache@v6
"""


def self_test() -> int:
    failures = []
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, ".github", "workflows"))
        os.makedirs(os.path.join(root, ".github", "actions", "local"))
        with open(os.path.join(root, ".github", "workflows", "ci.yml"), "w") as fh:
            fh.write(FIXTURE_WORKFLOW.replace("      - name: undecided\n        uses: actions/setup-go@v7\n        with:\n          cache: ${{ inputs.x == github.ref_name }}\n", ""))
        with open(os.path.join(root, ".github", "actions", "local", "action.yml"), "w") as fh:
            fh.write(FIXTURE_ACTION)
        problems = check(root)
        want = {"step 1:", "saves on PR", "step 6:", "inner save"}
        for w in want:
            if not any(w in p for p in problems):
                failures.append(f"expected a problem naming {w!r}; got {problems}")
        if len(problems) != len(want):
            failures.append(f"expected {len(want)} problems, got {len(problems)}: {problems}")
        with open(os.path.join(root, ".github", "workflows", "ci.yml"), "w") as fh:
            fh.write(FIXTURE_WORKFLOW)
        try:
            check(root)
            failures.append("an input the guard cannot decide passed")
        except Unreadable as err:
            if "undecided" not in str(err):
                failures.append(f"the undecided input was not named: {err}")
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, ".github", "workflows"))
        with open(os.path.join(root, ".github", "workflows", "e2e.yml"), "w") as fh:
            fh.write(FIXTURE_DISPATCH)
        with open(os.path.join(root, ".github", "workflows", "_called.yml"), "w") as fh:
            fh.write(FIXTURE_CALLED)
        problems = check(root)
        if not any("saves on any branch" in p and "by hand on a branch" in p for p in problems):
            failures.append(f"a save on a dispatched branch run must be reported; got {problems}")
        for quiet in ("saves on main only", "push-only job", "restore only", "callee saves when its input asks"):
            if any(quiet in p for p in problems):
                failures.append(f"{quiet!r} saves nothing off main; got {problems}")
        if not any("cache-to on any branch" in p for p in problems):
            failures.append(f"a BuildKit gha cache-to on a branch run must be reported; got {problems}")
        if not any("saves when a dispatch input asks" in p for p in problems):
            failures.append(f"a dispatch input is unknown, so a save behind one must be reported; got {problems}")
        if not any("job calls > .github/workflows/_called.yml job inner step callee saves always" in p
                   and "by hand on a branch" in p for p in problems):
            failures.append(f"a save in a called workflow must be reported on the caller's run; got {problems}")
        with open(os.path.join(root, ".github", "workflows", "e2e.yml"), "w") as fh:
            fh.write(FIXTURE_DISPATCH + FIXTURE_UNDECIDED)
        problems = check(root)
        if not any("release branch" in p for p in problems):
            failures.append(f"a save behind another branch's ref may run on this one and must be reported; got {problems}")
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, ".github", "workflows"))
        with open(os.path.join(root, ".github", "workflows", "release.yml"), "w") as fh:
            fh.write(FIXTURE_PUSH)
        problems = check(root)
        tag, branch = "a push of a tag", "a push to a branch"
        expect = {
            ("saves on any ref", tag): True, ("saves on any ref", branch): True,
            ("saves on a tag", tag): True, ("saves on a tag", branch): False,
            ("saves on a branch", tag): False, ("saves on a branch", branch): True,
            ("saves on one release branch", tag): False, ("saves on one release branch", branch): True,
            ("saves on main only", tag): False, ("saves on main only", branch): False,
            ("msys2 on main only", tag): False, ("msys2 on main only", branch): False,
            ("setup-go on main only", tag): False, ("setup-go on main only", branch): False,
            ("trivy on any ref", tag): True, ("trivy on any ref", branch): True,
            ("trivy on main only", tag): False, ("trivy on main only", branch): False,
        }
        for (step, run), want in expect.items():
            got = any(f"step {step}:" in p and p.endswith(run) for p in problems)
            if got != want:
                failures.append(f"{step!r} on {run}: expected {'a report' if want else 'none'}; got {problems}")
        with open(os.path.join(root, ".github", "workflows", "release.yml"), "w") as fh:
            fh.write(FIXTURE_TAGS_ONLY)
        problems = check(root)
        runs = sorted(p.rsplit(" on ", 1)[1] for p in problems)
        if runs != ["a push of a tag", "a release"]:
            failures.append(f"a tag-only push and a release run on tags, never on a branch; got {problems}")
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, ".github", "workflows"))
        os.makedirs(os.path.join(root, ".github", "actions", "local"))
        with open(os.path.join(root, ".github", "actions", "local", "action.yml"), "w") as fh:
            fh.write(FIXTURE_ACTION)
        with open(os.path.join(root, ".github", "workflows", "review.yml"), "w") as fh:
            fh.write(FIXTURE_REVIEW)
        with open(os.path.join(root, ".github", "workflows", "audit.yml"), "w") as fh:
            fh.write(FIXTURE_SUBDIR)
        problems = check(root)
        for run in ("a pull request review", "a pull request review comment"):
            if not any("saves on a review" in p and p.endswith(run) for p in problems):
                failures.append(f"a save on {run} must be reported; got {problems}")
        if not any("action under a checkout subdirectory > ./neokapi/.github/actions/local step inner save" in p
                   for p in problems):
            failures.append(f"a local action named under a checkout subdirectory must be followed; got {problems}")
        with open(os.path.join(root, ".github", "workflows", "queue.yml"), "w") as fh:
            fh.write(FIXTURE_UNMODELLED)
        try:
            check(root)
            failures.append("a workflow on an event the guard does not model passed")
        except Unreadable as err:
            if "merge_group" not in str(err) or "queue.yml" not in str(err):
                failures.append(f"the unmodelled event and its workflow were not named: {err}")
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, ".github", "workflows"))
        with open(os.path.join(root, ".github", "workflows", "caches.yml"), "w") as fh:
            fh.write(FIXTURE_SECOND_BRANCH)
        problems = check(root)
        for step, run in (("save on a second branch", "a push to a branch"),
                          ("prune on a second branch", "a push to a branch"),
                          ("save on a second branch", "a run started by hand on a branch"),
                          ("prune on a second branch", "a run started by hand on a branch")):
            if not any(f"step {step}:" in p and p.endswith(run) for p in problems):
                failures.append(f"{step!r} on {run} must be reported; got {problems}")
        if any("prune on main" in p for p in problems):
            failures.append(f"a prune gated on main runs nowhere else; got {problems}")
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, ".github", "workflows"))
        with open(os.path.join(root, ".github", "workflows", "images.yml"), "w") as fh:
            fh.write(FIXTURE_SCOPES)
        problems = check_scopes(root)
        if [p for p in problems if "'web-arm64'" in p] == [] or len(problems) != 1:
            failures.append(f"only the scope nothing writes (web-arm64) must be reported; got {problems}")
    if failures:
        print("check-ci-caches self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("check-ci-caches self-test: OK")
    return 0


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-test"]:
        sys.exit(self_test())
    if sys.argv[1:]:
        print(__doc__)
        sys.exit(2)
    sys.exit(main())
