#!/usr/bin/env python3
"""Guard: a build-only run of a release workflow publishes, signs and saves nothing.

    scripts/check-release-build-only.py              check the release workflows
    scripts/check-release-build-only.py --self-test  prove the rules on fixtures

release.yml and release-bowrain.yml take a `workflow_dispatch` that builds
only, so a change to the release builds can be measured on a branch. Its
setup job sets `needs.setup.outputs.build_only` to 'true' on a dispatch. This
guard evaluates every job and step with that output 'true' (every other
context unknown) and fails on any job or step that can still run and:

  * uses an action that publishes, signs or saves: softprops/action-gh-release,
    docker/login-action, sigstore/cosign-installer, actions/cache (not
    actions/cache/restore), actions/cache/save, docker/build-push-action that
    pushes, or setup-go or setup-msys2 with their cache on;
  * reads a secret other than GITHUB_TOKEN, in its env, with or run.

A publishing job is skipped at the job level (`if:` with `build_only !=
'true'`), so its steps are never reached; a build job skips its signing and
publishing steps the same way. A job that needs a skipped job is skipped too,
unless its condition calls always(), failure() or cancelled().

Exit status: 0 nothing a build-only run reaches publishes, 1 something does,
2 a workflow or condition cannot be read.
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
WORKFLOWS = ("release.yml", "release-bowrain.yml")

_spec = importlib.util.spec_from_file_location(
    "check_test_scripts", os.path.join(os.path.dirname(os.path.abspath(__file__)), "check-test-scripts.py"))
cts = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(cts)

PUBLISHING_ACTIONS = ("softprops/action-gh-release", "docker/login-action", "sigstore/cosign-installer",
                      "actions/cache/save")
SECRET_RE = re.compile(r"\$\{\{(.*?)\}\}", re.S)


class Unreadable(Exception):
    pass


@contextlib.contextmanager
def build_only():
    """Evaluate conditions as a build-only dispatch: build_only is 'true'."""
    saved = cts.context

    def context(name: str) -> object:
        lower = name.lower()
        if lower == "needs.setup.outputs.build_only":
            return "true"
        if lower == "github.event_name":
            return "workflow_dispatch"
        if lower.startswith("secrets.") and lower != "secrets.github_token":
            return SECRET
        return cts.UNKNOWN

    cts.context = context
    try:
        yield
    finally:
        cts.context = saved


SECRET = object()  # the value of a secret: present, so true, and flagged when reached


def evaluate(text: str, where: str) -> object:
    try:
        return cts.evaluate_on_pull_request(text)
    except ValueError as err:
        raise Unreadable(f"{where}: {text!r}: {err}") from err


def reads_secret(value: object, where: str) -> bool:
    """Report whether a step value can carry a secret on a build-only run."""
    for expr in SECRET_RE.findall(str(value)):
        if "secrets." not in expr.lower():
            continue
        names = re.findall(r"secrets\.([A-Za-z0-9_]+)", expr)
        if all(n.upper() == "GITHUB_TOKEN" for n in names):
            continue
        orig_truth = cts.truth

        def truth(v: object) -> object:
            return cts.UNKNOWN if v is SECRET else orig_truth(v)

        cts.truth = truth
        try:
            if evaluate("${{ " + expr + " }}", where) is not False:
                return True
        finally:
            cts.truth = orig_truth
    return False


def publishes(step: dict, where: str) -> str | None:
    uses = str(step.get("uses", "")).split("@", 1)[0]
    with_ = step.get("with") or {}
    if uses in PUBLISHING_ACTIONS:
        return f"uses {uses}"
    if uses == "actions/cache":
        return "saves a cache (actions/cache)"
    if uses == "docker/build-push-action" and str(with_.get("push", "false")).lower() not in ("false", ""):
        return "pushes an image"
    if uses in ("actions/setup-go", "msys2/setup-msys2"):
        default = True
        value = with_.get("cache", default)
        if value is True or (isinstance(value, str) and evaluate(value if "${{" in value else ("true" if value.lower() != "false" else "false"), where) is not False):
            return f"{uses} saves its cache"
    for part in ("env", "with"):
        for key, value in (step.get(part) or {}).items():
            if reads_secret(value, f"{where} {part}.{key}"):
                return f"reads a secret ({part}.{key})"
    if reads_secret(step.get("run", ""), f"{where} run"):
        return "reads a secret in its run"
    return None


def check_workflow(rel: str, doc: dict) -> list[str]:
    on = doc.get("on", doc.get(True)) or {}
    dispatch = on.get("workflow_dispatch") if isinstance(on, dict) else None
    inputs = (dispatch or {}).get("inputs") or {}
    problems = []
    if "build_only" not in inputs:
        return [f"{rel}: has no build-only workflow_dispatch (input build_only)"]
    jobs = {k: v for k, v in (doc.get("jobs") or {}).items() if isinstance(v, dict)}
    if "build_only" not in ((jobs.get("setup") or {}).get("outputs") or {}):
        problems.append(f"{rel}: the setup job does not output build_only")
    with build_only():
        try:
            skipped = cts.skipped_jobs(rel, jobs)
        except cts.Unreadable as err:
            raise Unreadable(str(err)) from err
        for job_id, job in jobs.items():
            if job_id in skipped:
                continue
            for n, step in enumerate(job.get("steps") or [], 1):
                if not isinstance(step, dict):
                    continue
                label = f"{rel} job {job_id} step {step.get('name') or step.get('uses') or n}"
                cond = step.get("if")
                if cond is not None and evaluate(cond if "${{" in str(cond) else "${{ " + str(cond) + " }}", label) is False:
                    continue
                why = publishes(step, label)
                if why:
                    problems.append(f"{label}: {why} on a build-only run")
    return problems


def check(root: str) -> list[str]:
    problems = []
    for name in WORKFLOWS:
        path = os.path.join(root, ".github", "workflows", name)
        try:
            with open(path) as fh:
                doc = yaml.safe_load(fh)
        except (OSError, yaml.YAMLError) as err:
            raise Unreadable(f"{path}: {err}") from err
        problems += check_workflow(f".github/workflows/{name}", doc or {})
    return problems


def main() -> int:
    try:
        problems = check(ROOT)
    except Unreadable as err:
        print(f"check-release-build-only: cannot read {err}", file=sys.stderr)
        return 2
    if problems:
        print("check-release-build-only: a build-only release run reaches a step that publishes, signs or saves\n")
        for p in problems:
            print("  " + p)
        print("\nSkip the job (`if: needs.setup.outputs.build_only != 'true'`) or the step, or\n"
              "gate the value on the same output.")
        return 1
    print("check-release-build-only: OK")
    return 0


FIXTURE = """\
name: Release
on:
  push:
    tags: ["v*"]
  workflow_dispatch:
    inputs:
      build_only:
        type: boolean
        default: true
jobs:
  setup:
    runs-on: ubuntu-latest
    outputs:
      build_only: x
    steps:
      - run: echo
  build:
    needs: [setup]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-go@v7
        with:
          cache: ${{ needs.setup.outputs.build_only != 'true' }}
      - name: go cache on
        uses: actions/setup-go@v7
      - name: icu restore
        uses: actions/cache/restore@v6
      - name: icu save
        uses: actions/cache@v6
      - name: signs
        if: runner.os == 'macOS'
        env:
          P12: ${{ secrets.APPLE_P12 }}
        run: sign
      - name: signs on a release only
        if: ${{ (runner.os == 'macOS') && needs.setup.outputs.build_only != 'true' }}
        env:
          P12: ${{ secrets.APPLE_P12 }}
        run: sign
      - name: key gated on build-only
        env:
          KEY: ${{ needs.setup.outputs.build_only != 'true' && secrets.POSTHOG || '' }}
        run: build
      - name: checkout token
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: gh api
  publish:
    needs: [setup, build]
    if: needs.setup.outputs.build_only != 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: softprops/action-gh-release@v3
  after-publish:
    needs: [publish]
    runs-on: ubuntu-latest
    steps:
      - uses: softprops/action-gh-release@v3
  verify:
    needs: [setup, publish]
    if: ${{ always() && needs.setup.result == 'success' }}
    runs-on: ubuntu-latest
    steps:
      - name: verify publishes
        uses: softprops/action-gh-release@v3
"""


def self_test() -> int:
    failures = []
    doc = yaml.safe_load(FIXTURE)
    problems = check_workflow("release.yml", doc)
    want = {"go cache on", "icu save", "step signs:", "verify publishes"}
    for w in want:
        if not any(w in p for p in problems):
            failures.append(f"expected a problem naming {w!r}; got {problems}")
    for quiet in ("icu restore", "signs on a release only", "key gated on build-only", "checkout token",
                  "job publish ", "job after-publish "):
        if any(quiet in p for p in problems):
            failures.append(f"{quiet!r} is not reached or publishes nothing; got {problems}")
    if len(problems) != len(want):
        failures.append(f"expected {len(want)} problems, got {problems}")
    no_input = yaml.safe_load(FIXTURE.replace("      build_only:\n        type: boolean\n        default: true\n", "      other:\n        type: string\n"))
    if not any("no build-only workflow_dispatch" in p for p in check_workflow("release.yml", no_input)):
        failures.append("a release workflow without the build-only input must fail")
    if failures:
        print("check-release-build-only self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("check-release-build-only self-test: OK")
    return 0


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-test"]:
        sys.exit(self_test())
    if sys.argv[1:]:
        print(__doc__)
        sys.exit(2)
    sys.exit(main())
