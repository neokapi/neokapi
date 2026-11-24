#!/usr/bin/env python3
"""Guard: a build-only run of a release workflow holds no write and publishes nothing.

    scripts/check-release-build-only.py              check the release workflows
    scripts/check-release-build-only.py --self-test  prove the rules on fixtures

release.yml, release-bowrain.yml and the plugin releases (release-check.yml,
release-sat.yml) take a `workflow_dispatch` that builds
only, so a change to the release builds can be measured on a branch. Its
setup job sets `needs.setup.outputs.build_only` to 'true' on a dispatch. The
setup job's script belongs to the dispatched branch, so the guard asserts the
structure that makes a dispatch safe whatever that script does:

  * The workflow-level permissions are read-only.
  * No job that a build-only run reaches holds a write permission. A job that
    publishes asks for its own grants and is skipped at the job level
    (`if:` with `build_only != 'true'`); a job that needs a skipped job is
    skipped too, unless its condition calls always(), failure() or cancelled().

It then evaluates every reachable job and step as a dispatch on a branch
other than main (build_only 'true', every other context unknown), following
local composite actions with their inputs, and fails on any that:

  * uses an action that publishes or signs: softprops/action-gh-release,
    docker/login-action, sigstore/cosign-installer, or docker/build-push-action
    that pushes;
  * saves a cache, by the rules of scripts/check-ci-caches.py (actions/cache
    and its post step, setup-go, setup-vp, setup-msys2, golangci-lint,
    BuildKit cache-to);
  * runs a command that pushes, uploads or publishes, whatever credential it
    names: git push, gh release, gh pr create, gh api with a write method,
    docker push, npm publish, cosign sign, aws s3 cp, a registry push or one
    of the repository's publish scripts;
  * reads a secret, GITHUB_TOKEN and github.token included, in its env, with
    or run, or in the job's or the workflow's env.

It also holds the meaning of `latest`: the last release, kept patched by the
weekly rebuild. Outside LATEST_PUBLISHERS (release-bowrain.yml and
rebuild-release-images.yml), nothing pushes a `latest` tag to the registry:
no docker/build-push-action tag, no `docker push` of one, no `imagetools
create -t …:latest`, in any workflow, local action, Makefile or script. A build
from main pushes `main` and its commit tag instead.

Exit status: 0 a build-only run holds no write and publishes nothing, 1 it
does, 2 a workflow, action or condition cannot be read.
"""

from __future__ import annotations

import contextlib
import glob
import importlib.util
import os
import re
import sys

import yaml

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
WORKFLOWS = ("release.yml", "release-bowrain.yml", "release-check.yml", "release-sat.yml", "rebuild-release-images.yml")

_spec = importlib.util.spec_from_file_location("check_ci_caches", os.path.join(HERE, "check-ci-caches.py"))
cc = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(cc)
cts = cc.cts  # the condition evaluator check-ci-caches uses; patched below

PUBLISHING_ACTIONS = ("softprops/action-gh-release", "docker/login-action", "sigstore/cosign-installer")
PUBLISHING_RUN = re.compile(
    r"\bgit\s+push\b"
    r"|\bgh\s+release\s+(?:upload|create|edit|delete)\b"
    r"|\bgh\s+pr\s+(?:create|merge|edit)\b"
    r"|\bgh\s+api\b[^\n]*(?:-X|--method)\s*['\"]?(?:POST|PUT|PATCH|DELETE)"
    r"|\bdocker\s+(?:push|manifest\s+push)\b"
    r"|\bnpm\s+publish\b|\bpnpm\s+publish\b"
    r"|\bcosign\s+sign"
    r"|\baws\s+s3\s+(?:cp|sync|mv|rm)\b"
    r"|registry_commit_and_push"
    r"|scripts/publish-[a-z-]+\.sh"
)
EXPR = re.compile(r"\$\{\{(.*?)\}\}", re.S)
SECRET = object()  # a secret's value: true, and flagged wherever a run reaches it


class Unreadable(Exception):
    pass


@contextlib.contextmanager
def build_only():
    """Evaluate conditions as a build-only dispatch on a branch other than main."""
    saved = cts.context, cts.compare, cts.truth
    branch = object()

    def context(name: str) -> object:
        lower = name.lower()
        if lower == "needs.setup.outputs.build_only":
            return "true"
        if lower == "github.event_name":
            return "workflow_dispatch"
        if lower == "github.ref":
            return branch
        if lower.startswith("secrets.") or lower == "github.token":
            return SECRET
        return cts.UNKNOWN

    def compare(op: str, a: object, b: object) -> object:
        if branch in (a, b):
            other = b if a is branch else a
            if op in ("==", "!=") and isinstance(other, str) and other.lower() == "refs/heads/main":
                return op == "!="
            return cts.UNKNOWN
        if SECRET in (a, b):
            return cts.UNKNOWN
        return saved[1](op, a, b)

    def truth(value: object) -> object:
        if value is SECRET:
            return cts.UNKNOWN
        if value is branch:
            return True
        return saved[2](value)

    cts.context, cts.compare, cts.truth = context, compare, truth
    try:
        yield
    finally:
        cts.context, cts.compare, cts.truth = saved


def evaluate(cond: object, where: str) -> object:
    text = str(cond)
    if "${{" not in text:
        text = "${{ " + text + " }}"
    try:
        return cts.evaluate_on_pull_request(text)
    except ValueError as err:
        raise Unreadable(f"{where}: {cond!r}: {err}") from err


def carries_secret(value: object, where: str) -> bool:
    """Whether a value can hold a secret's text on a build-only run."""
    for expr in EXPR.findall(str(value)):
        if "secrets." not in expr.lower() and "github.token" not in expr.lower():
            continue
        if evaluate("${{ " + expr + " }}", where) is not False:
            return True
    return False


def writes(perms: object) -> list[str]:
    if perms in ("write-all",):
        return ["write-all"]
    if isinstance(perms, dict):
        return [f"{k}: {v}" for k, v in perms.items() if str(v) == "write"]
    return []


def check_steps(root: str, steps: list, inputs: dict | None, where: str, problems: list[str], seen: set) -> None:
    for n, step in enumerate(steps or [], 1):
        if not isinstance(step, dict):
            continue
        label = f"{where} step {step.get('name') or step.get('uses') or n}"
        cond = cc.substitute(step.get("if"), inputs or {})
        if cond is not None and evaluate(cond, label) is False:
            continue
        uses = str(step.get("uses", ""))
        name = uses.split("@", 1)[0]
        with_ = step.get("with") or {}
        if uses.startswith("./"):
            path = os.path.join(root, uses[2:], "action.yml")
            if not os.path.isfile(path):
                path = os.path.join(root, uses[2:], "action.yaml")
            try:
                with open(path) as fh:
                    action = yaml.safe_load(fh) or {}
            except (OSError, yaml.YAMLError) as err:
                raise Unreadable(f"{label}: {uses}: {err}") from err
            runs = action.get("runs") or {}
            if runs.get("using") == "composite":
                resolved = {}
                for key, spec in (action.get("inputs") or {}).items():
                    default = spec.get("default") if isinstance(spec, dict) else None
                    value = with_.get(key, default)
                    resolved[key] = cc.substitute(value, inputs or {}) if value is not None else None
                marker = (path, tuple(sorted((k, str(v)) for k, v in resolved.items())))
                if marker not in seen:
                    seen.add(marker)
                    check_steps(root, runs.get("steps"), resolved, f"{label} > {uses}", problems, seen)
            continue
        if name in PUBLISHING_ACTIONS:
            problems.append(f"{label}: uses {name}")
        elif name == "docker/build-push-action" and evaluate(cc.substitute(with_.get("push", False), inputs or {}), label) is not False:
            problems.append(f"{label}: pushes an image")
        if uses:
            try:
                verdict = cc.saves(uses, {k: cc.substitute(v, inputs or {}) for k, v in with_.items()}, inputs or {}, label)
            except cc.Unreadable as err:
                raise Unreadable(str(err)) from err
            if verdict is True or verdict is cts.UNKNOWN:
                problems.append(f"{label}: {name} may save a cache")
        run = str(step.get("run") or "")
        hit = PUBLISHING_RUN.search(run)
        if hit:
            problems.append(f"{label}: runs {hit.group(0)!r}, which publishes")
        for part in ("env", "with"):
            for key, value in (step.get(part) or {}).items():
                if carries_secret(cc.substitute(value, inputs or {}), f"{label} {part}.{key}"):
                    problems.append(f"{label}: reads a secret ({part}.{key})")
        if carries_secret(run, f"{label} run"):
            problems.append(f"{label}: reads a secret in its run")


def check_workflow(root: str, rel: str, doc: dict) -> list[str]:
    on = doc.get("on", doc.get(True)) or {}
    dispatch = on.get("workflow_dispatch") if isinstance(on, dict) else None
    if "build_only" not in ((dispatch or {}).get("inputs") or {}):
        return [f"{rel}: has no build-only workflow_dispatch (input build_only)"]
    problems = []
    wf_perms = doc.get("permissions", "write-all")
    if writes(wf_perms) or wf_perms in ("write-all", None):
        problems.append(f"{rel}: workflow-level permissions grant write ({wf_perms}); make them read-only and grant per publishing job")
    jobs = {k: v for k, v in (doc.get("jobs") or {}).items() if isinstance(v, dict)}
    if "build_only" not in ((jobs.get("setup") or {}).get("outputs") or {}):
        problems.append(f"{rel}: the setup job does not output build_only")
    with build_only():
        for key, value in (doc.get("env") or {}).items():
            if carries_secret(value, f"{rel} env.{key}"):
                problems.append(f"{rel}: the workflow env.{key} reads a secret, which every job sees")
        try:
            skipped = cts.skipped_jobs(rel, jobs)
        except cts.Unreadable as err:
            raise Unreadable(str(err)) from err
        for job_id, job in jobs.items():
            if job_id in skipped:
                continue
            label = f"{rel} job {job_id}"
            granted = writes(job.get("permissions", wf_perms))
            if granted:
                problems.append(f"{label}: reachable on a build-only run and holds {', '.join(granted)}")
            for key, value in (job.get("env") or {}).items():
                if carries_secret(value, f"{label} env.{key}"):
                    problems.append(f"{label}: its env.{key} reads a secret")
            check_steps(root, job.get("steps"), None, label, problems, set())
    return problems


LATEST_PUBLISHERS = (".github/workflows/release-bowrain.yml", ".github/workflows/rebuild-release-images.yml")
LATEST_TAG = re.compile(r":latest\b")
LATEST_RUN = re.compile(r"docker\s+push\s+\S*:latest\b|imagetools\s+create\b[^\n]*-t\s+\S*:latest\b|--tag[= ]\S*:latest\b")


def latest_pushes(root: str) -> list[str]:
    """Places outside LATEST_PUBLISHERS that push a `latest` tag."""
    out = []
    files = sorted(glob.glob(os.path.join(root, ".github", "workflows", "*.yml"))
                   + glob.glob(os.path.join(root, ".github", "actions", "**", "action.yml"), recursive=True))
    for path in files:
        rel = os.path.relpath(path, root).replace(os.sep, "/")
        if rel in LATEST_PUBLISHERS:
            continue
        try:
            with open(path) as fh:
                doc = yaml.safe_load(fh) or {}
        except (OSError, yaml.YAMLError) as err:
            raise Unreadable(f"{rel}: {err}") from err
        steps = list((doc.get("runs") or {}).get("steps") or [])
        for job in (doc.get("jobs") or {}).values():
            if isinstance(job, dict):
                steps += job.get("steps") or []
        for step in steps:
            if not isinstance(step, dict):
                continue
            label = f"{rel} step {step.get('name') or step.get('uses') or '?'}"
            with_ = step.get("with") or {}
            if str(step.get("uses", "")).startswith("docker/build-push-action") and LATEST_TAG.search(str(with_.get("tags", ""))):
                out.append(f"{label}: pushes a latest tag")
            hit = LATEST_RUN.search(str(step.get("run", "")))
            if hit:
                out.append(f"{label}: runs {hit.group(0)!r}")
    others = [os.path.join(root, "Makefile")] + glob.glob(os.path.join(root, "*", "Makefile")) \
        + glob.glob(os.path.join(root, "scripts", "**", "*.sh"), recursive=True)
    for path in sorted(set(others)):
        if not os.path.isfile(path):
            continue
        rel = os.path.relpath(path, root).replace(os.sep, "/")
        with open(path, errors="replace") as fh:
            for n, line in enumerate(fh, 1):
                if line.lstrip().startswith("#"):
                    continue
                hit = LATEST_RUN.search(line)
                if hit:
                    out.append(f"{rel}:{n}: {hit.group(0)!r}")
    return out


def check(root: str) -> list[str]:
    problems = [f"{p}; only {' and '.join(LATEST_PUBLISHERS)} move latest, which means the last release"
                for p in latest_pushes(root)]
    for name in WORKFLOWS:
        path = os.path.join(root, ".github", "workflows", name)
        try:
            with open(path) as fh:
                doc = yaml.safe_load(fh)
        except (OSError, yaml.YAMLError) as err:
            raise Unreadable(f"{path}: {err}") from err
        problems += check_workflow(root, f".github/workflows/{name}", doc or {})
    return problems


def main() -> int:
    try:
        problems = check(ROOT)
    except Unreadable as err:
        print(f"check-release-build-only: cannot read {err}", file=sys.stderr)
        return 2
    if problems:
        print("check-release-build-only: a build-only release run holds write or publishes\n")
        for p in problems:
            print("  " + p)
        print("\nPush `main` and the commit tag from a build of main; `latest` is the release's.\n"
              "Grant write only to a publishing job, skip it on a build-only run\n"
              "(`if: needs.setup.outputs.build_only != 'true'`), and keep secrets and\n"
              "publishing commands out of every job a build-only run reaches.")
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
permissions:
  contents: read
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
      - name: gh release with the built-in token
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: gh release upload v1 out/kapi.tar.gz
      - name: git push with github.token
        run: git push https://x:${{ github.token }}@github.com/neokapi/neokapi HEAD:refs/heads/x
      - name: node via the local action
        uses: ./.github/actions/node
  secret-job-env:
    needs: [setup]
    runs-on: ubuntu-latest
    env:
      TAP: ${{ secrets.HOMEBREW_TAP_TOKEN }}
    steps:
      - run: make build
  writer:
    needs: [setup]
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - run: make build
  publish:
    needs: [setup, build]
    if: needs.setup.outputs.build_only != 'true'
    runs-on: ubuntu-latest
    permissions:
      contents: write
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

FIXTURE_ACTION = """\
name: node
runs:
  using: composite
  steps:
    - name: vp saves off main
      uses: voidzero-dev/setup-vp@v1
      with:
        cache: true
        cache-save: ${{ github.event_name != 'pull_request' }}
    - name: vp saves on main only
      uses: voidzero-dev/setup-vp@v1
      with:
        cache: true
        cache-save: ${{ github.ref == 'refs/heads/main' }}
"""


def self_test() -> int:
    import tempfile
    failures = []
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, ".github", "actions", "node"))
        with open(os.path.join(root, ".github", "actions", "node", "action.yml"), "w") as fh:
            fh.write(FIXTURE_ACTION)
        problems = check_workflow(root, "release.yml", yaml.safe_load(FIXTURE))
        want = ["go cache on", "icu save", "step signs:", "verify publishes",
                "gh release with the built-in token", "git push with github.token",
                "vp saves off main", "secret-job-env: its env.TAP", "job writer: reachable"]
        for w in want:
            if not any(w in p for p in problems):
                failures.append(f"expected a problem naming {w!r}; got {problems}")
        for quiet in ("icu restore", "signs on a release only", "key gated on build-only", "vp saves on main only",
                      "job publish", "job after-publish"):
            if any(quiet in p for p in problems):
                failures.append(f"{quiet!r} is not reached or publishes nothing; got {problems}")
        loose = yaml.safe_load(FIXTURE.replace("permissions:\n  contents: read\njobs:", "permissions:\n  contents: write\njobs:"))
        if not any("workflow-level permissions grant write" in p for p in check_workflow(root, "release.yml", loose)):
            failures.append("workflow-level write must fail")
        unread = yaml.safe_load(FIXTURE.replace("if: runner.os == 'macOS'\n        env:\n          P12", "if: ${{ fromJSON( }}\n        env:\n          P12"))
        try:
            check_workflow(root, "release.yml", unread)
            failures.append("an unreadable condition must fail closed")
        except Unreadable:
            pass
        no_input = yaml.safe_load(FIXTURE.replace("      build_only:\n        type: boolean\n        default: true\n", "      other:\n        type: string\n"))
        if not any("no build-only workflow_dispatch" in p for p in check_workflow(root, "release.yml", no_input)):
            failures.append("a release workflow without the build-only input must fail")
    with tempfile.TemporaryDirectory() as root:
        wf = os.path.join(root, ".github", "workflows")
        os.makedirs(wf)
        os.makedirs(os.path.join(root, "bowrain"))
        with open(os.path.join(wf, "deploy-x.yml"), "w") as fh:
            fh.write("""jobs:
  build:
    steps:
      - name: push main
        uses: docker/build-push-action@v7
        with:
          tags: |
            ghcr.io/neokapi/x:main
            ghcr.io/neokapi/x:abc123
      - name: push latest
        uses: docker/build-push-action@v7
        with:
          tags: ghcr.io/neokapi/x:latest
      - name: retag latest
        run: docker buildx imagetools create -t ghcr.io/neokapi/x:latest ghcr.io/neokapi/x:1.0
      - name: scan latest
        uses: aquasecurity/trivy-action@v1
        with:
          image-ref: ghcr.io/neokapi/x:latest
""")
        with open(os.path.join(wf, "rebuild-release-images.yml"), "w") as fh:
            fh.write("jobs:\n  p:\n    steps:\n      - run: docker buildx imagetools create -t ghcr.io/neokapi/x:latest x\n")
        with open(os.path.join(root, "bowrain", "Makefile"), "w") as fh:
            fh.write("push:\n\tdocker push $(IMAGE):$(VERSION)\n\tdocker push $(IMAGE):latest\n# docker push $(IMAGE):latest in a comment\n")
        found = latest_pushes(root)
        for want in ("step push latest", "step retag latest", "bowrain/Makefile:3"):
            if not any(want in f for f in found):
                failures.append(f"latest push not caught: {want!r} in {found}")
        for quiet in ("push main", "scan latest", "rebuild-release-images", "Makefile:4"):
            if any(quiet in f for f in found):
                failures.append(f"{quiet!r} pushes no latest, or may: {found}")
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
