#!/usr/bin/env python3
"""Guard: no step a pull request runs saves an Actions cache.

A pull request can restore the caches saved on main and on its own ref, and
nothing else. A cache it saves serves only its own later pushes, while it
counts against the repository's 10 GiB and pushes out the caches every other
pull request restores. So on a pull request every cache step restores and
none saves; main saves.

    scripts/check-ci-caches.py              check the repository
    scripts/check-ci-caches.py --self-test  prove it on fixtures

It reads every workflow under .github/workflows that runs on `pull_request`,
every step that can run there (the `if:` conditions are evaluated as
scripts/check-test-scripts.py does), and the steps of the local composite
actions those steps use, with the caller's inputs. A step saves when it is:

  * actions/cache (not actions/cache/restore), or actions/cache/save;
  * actions/setup-go whose `cache` is not false (it defaults to true);
  * voidzero-dev/setup-vp with `cache` true and `cache-save` not false;
  * actions/setup-node with a `cache` input;
  * golangci/golangci-lint-action without `skip-cache` or `skip-save-cache`;
  * docker/build-push-action whose `cache-to` names the gha cache;
  * msys2/setup-msys2 whose `cache` is not false (it defaults to true).

An input or condition the guard cannot decide is an error naming the step.

Exit status: 0 nothing a pull request runs saves a cache, 1 something does,
2 an input is unreadable.
"""

from __future__ import annotations

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


def substitute(value: object, inputs: dict[str, str]) -> object:
    """Replace `inputs.<name>` in an expression with the caller's literal."""
    if not isinstance(value, str) or "inputs." not in value:
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
    if name == "golangci/golangci-lint-action":
        skip = truthy_input(with_, "skip-cache", False, inputs, where)
        skip_save = truthy_input(with_, "skip-save-cache", False, inputs, where)
        return cts.logical_not(cts.logical_or(skip, skip_save))
    if name == "docker/build-push-action":
        to = with_.get("cache-to")
        if not to:
            return False
        if "type=gha" not in str(to):
            return False
        return on_pr(to, inputs, f"{where} input cache-to") if "${{" in str(to) else True
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
        if not isinstance(step, dict) or "uses" not in step:
            continue
        label = f"{where} step {step.get('name') or step.get('id') or n}"
        cond = substitute(step.get("if"), inputs)
        if cts.excludes_pull_requests(cond, label):
            continue
        uses = str(step["uses"])
        with_ = step.get("with") or {}
        if uses.startswith("./"):
            action = os.path.join(root, uses[2:], "action.yml")
            if not os.path.isfile(action):
                action = os.path.join(root, uses[2:], "action.yaml")
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
            problems.append(f"{label}: {uses} saves a cache on a pull request")
        elif verdict is cts.UNKNOWN:
            problems.append(f"{label}: {uses} may save a cache on a pull request (its inputs turn on a value the event does not fix)")


def check(root: str) -> list[str]:
    problems: list[str] = []
    directory = os.path.join(root, ".github", "workflows")
    for name in sorted(os.listdir(directory)):
        if not name.endswith((".yml", ".yaml")):
            continue
        rel = f".github/workflows/{name}"
        doc = load(os.path.join(directory, name))
        if not cts.runs_on_pull_request(doc):
            continue
        jobs = {k: v for k, v in (doc.get("jobs") or {}).items() if isinstance(v, dict)}
        try:
            skipped = cts.skipped_jobs(rel, jobs)
        except cts.Unreadable as err:
            raise Unreadable(str(err)) from err
        for job_id, job in jobs.items():
            if job_id in skipped:
                continue
            try:
                walk_steps(root, job.get("steps"), {}, f"{rel} job {job_id}", problems, set())
            except cts.Unreadable as err:
                raise Unreadable(str(err)) from err
    return problems


def main() -> int:
    try:
        problems = check(ROOT)
    except Unreadable as err:
        print(f"check-ci-caches: cannot read {err}", file=sys.stderr)
        return 2
    if problems:
        print("check-ci-caches: a step a pull request runs saves an Actions cache\n")
        for p in problems:
            print("  " + p)
        print("\nRestore on a pull request and save on main: actions/cache/restore with\n"
              "`if: github.event_name == 'pull_request'` beside actions/cache with the\n"
              "opposite condition, or the action's own switch (setup-go `cache: false`,\n"
              "setup-vp `cache-save: ${{ github.event_name != 'pull_request' }}`).")
        return 1
    print("check-ci-caches: OK")
    return 0


FIXTURE_WORKFLOW = """\
name: CI
on: [pull_request, push]
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
