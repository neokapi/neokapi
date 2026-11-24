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
weekly rebuild. Outside LATEST_PUBLISHERS (the Bowrain release, its weekly
rebuild and the kapi CLI release), nothing pushes a `latest` tag: no
docker/build-push-action tag, `docker push`, `docker tag` before a push,
`docker build --push`, `imagetools create -t` (continuation lines included),
`skopeo copy` or `crane`, and no docker/metadata-action without
`latest=false`, in any workflow, local action, Makefile, *.mk or shell script.
A push whose tag is not written out must use one of SAFE_TAGS, or a whole
reference listed in SAFE_REFS. A build from main pushes `main` and its commit
tag instead.

Exit status: 0 a build-only run holds no write and publishes nothing, 1 it
does, 2 a workflow, action or condition cannot be read.
"""

from __future__ import annotations

import contextlib
import glob
import importlib.util
import os
import re
import shlex
import subprocess
import sys
import tempfile

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


# The files that may move `latest`, each with the image whose `latest` it may
# move: None for any image. release.yml (the kapi CLI release) may move only
# the kapi image's; every push there is checked against that image.
LATEST_PUBLISHERS = {
    ".github/workflows/release-bowrain.yml": None,
    ".github/workflows/rebuild-release-images.yml": None,
    ".github/workflows/release.yml": re.compile(r"^ghcr\.io/(\$\{\{ github\.repository_owner \}\}|neokapi)/kapi$"),
}
# A tag written as this cannot be `latest`.
SAFE_TAGS = {"${{ github.sha }}"}
# Tags safe only where the one shared refusal proves them not `latest`: in a
# workflow, an earlier step of the same job that uses REFUSAL_ACTION with this
# exact expression, no `if:` and no `continue-on-error`; in a Makefile, a
# top-level include of REFUSAL_MK. String equality holds only for values a job
# cannot change between the refusal and the push: inputs, the github context
# and step outputs. An `env` expression is never one of these, because a step
# can rewrite it through GITHUB_ENV.
REFUSABLE_TAGS = {"${{ inputs.image-tag }}", "${{ steps.meta.outputs.tag }}", "$(VERSION)"}
# The values each refusal is run against before the guard trusts it. Both must
# refuse every bad probe and accept every good one, so they accept the same
# set: the Docker tag grammar, and not `latest`.
REFUSAL_BAD = ["latest", "latest ", " latest", "", "a b", "a\nlatest", "latest\n", "a,latest",
               ".x", "-x", "a" * 129, "x:latest", "a'b", "a\tb"]
REFUSAL_GOOD = ["v1.2.3", "abc123", "dev", "LATEST", "_x", "a" * 128, "v1.4.0-rc5-12-gabc-dirty"]
REFUSAL_ACTION = "./.github/actions/refuse-latest-tag"
REFUSAL_MK = "refuse-latest-version.mk"
# Whole image references held in a variable, per file, that cannot be `latest`.
SAFE_REFS = {
    ("scripts/mirror-image.sh", "$target"),  # the script refuses tag latest
}
SKIP_DIRS = {".git", "node_modules", ".claude", ".venv", "dist", "build", "target", "vendor"}
WHOLE_EXPR = re.compile(r"\$\{\{.*?\}\}", re.S)


def _refuses_exactly(run_one) -> bool:
    """Whether run_one(value) is False for every bad probe and True for every
    good one."""
    return (not any(run_one(v) for v in REFUSAL_BAD)) and all(run_one(v) for v in REFUSAL_GOOD)


def refusal_implementations(root: str) -> tuple[bool, bool]:
    """Whether the shared refusals refuse what they must. Each is run against
    REFUSAL_BAD and REFUSAL_GOOD rather than read: the action's one step with
    TAG set as its env maps it, and the make fragment with VERSION given on the
    command line."""
    action_ok = mk_ok = False
    try:
        with open(os.path.join(root, ".github", "actions", "refuse-latest-tag", "action.yml")) as fh:
            doc = yaml.safe_load(fh) or {}
        steps = (doc.get("runs") or {}).get("steps") or []
        step = steps[0] if len(steps) == 1 and isinstance(steps[0], dict) else {}
        if (step.get("shell") == "bash" and "continue-on-error" not in step and "if" not in step
                and str((step.get("env") or {}).get("TAG", "")).replace(" ", "") == "${{inputs.tag}}"):
            script = str(step.get("run", ""))
            action_ok = _refuses_exactly(lambda v: subprocess.run(
                ["bash", "-e", "-c", script], env={"TAG": v, "PATH": os.environ.get("PATH", "")},
                capture_output=True).returncode == 0)
    except (OSError, yaml.YAMLError):
        pass
    fragment = os.path.abspath(os.path.join(root, "scripts", "lib", REFUSAL_MK))
    if os.path.exists(fragment):
        with tempfile.TemporaryDirectory() as tmp:
            mk = os.path.join(tmp, "Makefile")
            with open(mk, "w") as fh:
                fh.write(f"include {fragment}\nprobe:\n\t@true\n")
            mk_ok = _refuses_exactly(lambda v: subprocess.run(
                ["make", "-s", "-f", mk, f"VERSION={v}"], capture_output=True, cwd=tmp).returncode == 0)
    return action_ok, mk_ok


def makefile_refuses(text: str) -> bool:
    """Whether the Makefile includes REFUSAL_MK at top level: not in a recipe,
    not commented, not inside a conditional."""
    depth = 0
    for line in text.splitlines():
        if line.startswith("\t") or not line.strip() or line.lstrip().startswith("#"):
            continue
        word = line.split()[0]
        if word in ("ifeq", "ifneq", "ifdef", "ifndef"):
            depth += 1
        elif word == "endif":
            depth -= 1
        elif word == "include" and depth == 0 and any(arg.endswith("/" + REFUSAL_MK) or arg == REFUSAL_MK
                                                      for arg in line.split()[1:]):
            return True
    return False


def ref_problem(ref: str, path: str, refused: frozenset = frozenset()) -> str | None:
    """Why pushing this reference may push `latest`; None when it cannot.
    refused holds the tags the shared refusal proves are not latest here."""
    ref = ref.strip().strip("'\"")
    if not ref:
        return None
    if ":latest" in ref:
        return f"pushes {ref}"
    if WHOLE_EXPR.fullmatch(ref):
        # An expression: each image reference it can yield is a quoted string.
        images = [lit for lit in re.findall(r"'([^']*)'", ref) if re.fullmatch(r"[\w.-]+/\S+:\S+", lit)]
        if any(lit.endswith(":latest") for lit in images):
            return f"pushes {ref}, which can be latest"
        if images:
            return None
        return f"pushes {ref}, a tag that is not written out"
    masked = WHOLE_EXPR.sub(lambda m: "\x00" * len(m.group(0)), ref)
    last_slash = masked.rfind("/")
    colon = masked.rfind(":")
    if colon <= last_slash:
        if (path, ref) in SAFE_REFS:
            return None
        return f"pushes {ref}, whose tag is not written out"
    tag = ref[colon + 1:]
    if tag in SAFE_TAGS or not re.search(r"\$", tag):
        return None
    if tag in REFUSABLE_TAGS:
        if tag in refused:
            return None
        return f"pushes {ref}, and no shared refusal before it proves {tag} is not latest"
    return f"pushes {ref}, whose tag {tag} is not a known safe value"


def _commands(text: str) -> list[list[str]]:
    joined = re.sub(r"\\\s*\n", " ", text)
    out = []
    for line in joined.splitlines():
        if line.lstrip().startswith("#"):
            continue
        for part in re.split(r"&&|\|\||;|\|", line):
            try:
                words = shlex.split(part, comments=True, posix=True)
            except ValueError:
                words = part.split()
            words = [w.lstrip("@-") if i == 0 else w for i, w in enumerate(words)]
            if words:
                out.append(words)
    return out


def _normalise(words: list[str]) -> list[str]:
    """Split `--flag=value` for the flags whose value decides a push."""
    out = []
    for w in words:
        m = re.fullmatch(r"(--output|--set|--tag)=(.*)", w)
        if m:
            out += [m.group(1), m.group(2)]
        else:
            out.append(w)
    return out


def output_pushes(spec: str) -> bool:
    """Whether a buildx output spec (`type=registry`, `type=image,push=true`)
    pushes. Several specs may be given, one per line."""
    for line in str(spec).splitlines():
        pairs = dict(kv.split("=", 1) for kv in line.replace(" ", "").split(",") if "=" in kv)
        if pairs.get("type") == "registry" or pairs.get("push", "").lower() == "true":
            return True
    return False


def bake_file_pushes(root: str, files: list[str]) -> bool:
    for name in files or ["docker-bake.hcl", "docker-bake.json", "docker-bake.override.hcl"]:
        try:
            with open(os.path.join(root, name)) as fh:
                text = fh.read()
        except OSError:
            continue
        if re.search(r'push\s*[=:]\s*"?true', text) or re.search(r"type\s*=\s*registry", text):
            return True
    return False


def build_pushes(w: list[str], root: str) -> bool:
    """Whether a `docker build`, `docker buildx build` or `bake` command pushes."""
    if "--push" in w:
        return True
    for i, x in enumerate(w):
        if x in ("--output", "-o") and i + 1 < len(w) and output_pushes(w[i + 1]):
            return True
        if x == "--set" and i + 1 < len(w):
            key, _, value = w[i + 1].partition("=")
            if key.endswith(".output") and output_pushes(value):
                return True
            if key.endswith(".push") and value.lower() == "true":
                return True
    if "bake" in w:
        files = [w[i + 1] for i, x in enumerate(w) if x in ("-f", "--file") and i + 1 < len(w)]
        return bake_file_pushes(root, files)
    return False


def shell_pushes(text: str, path: str, refused: frozenset = frozenset(), root: str = ".") -> list[str]:
    """Pushes of a possibly-`latest` tag in shell or make recipe text."""
    found = []
    commands = [_normalise(w) for w in _commands(text)]
    pushes_later = any(w[:2] == ["docker", "push"] for w in commands)

    def flag(ref, what):
        problem = ref_problem(ref, path, refused)
        if problem:
            found.append(f"{what}: {problem}")

    for w in commands:
        if w[:2] == ["docker", "push"] and len(w) > 2:
            flag(w[-1], "docker push")
        elif w[:2] == ["docker", "tag"] and len(w) >= 4 and pushes_later:
            flag(w[3], "docker tag, then a push")
        elif "imagetools" in w and "create" in w:
            for i, x in enumerate(w):
                if x in ("-t", "--tag") and i + 1 < len(w):
                    flag(w[i + 1], "imagetools create")
        elif w[0] == "docker" and "bake" in w and build_pushes(w, root):
            found.append("docker buildx bake: pushes the tags its bake file names, which this guard cannot check")
        elif w[0] == "docker" and "build" in w and build_pushes(w, root):
            for i, x in enumerate(w):
                if x in ("-t", "--tag") and i + 1 < len(w):
                    flag(w[i + 1], "docker build, pushing")
        elif w[0] == "skopeo" and "copy" in w:
            dest = [x for x in w if x.startswith("docker://")]
            if dest:
                flag(dest[-1][len("docker://"):], "skopeo copy")
        elif w[0] == "crane" and len(w) >= 4 and w[1] in ("copy", "cp", "push"):
            flag(w[3], f"crane {w[1]}")
        elif w[0] == "crane" and len(w) >= 4 and w[1] == "tag":
            if w[3] == "latest" or "$" in w[3]:
                found.append(f"crane tag: tags {w[2]} as {w[3]}")
    return found


def _allowed_ref(ref: str, allowed, steps_before: list, job_env: dict) -> bool:
    """Whether a pushed reference is the one image this file may move latest
    for: a literal reference whose repository matches, or the output of an
    earlier step that builds every reference from an IMAGE env that matches."""
    if allowed is None:
        return True
    if not allowed:
        return False
    ref = ref.strip()
    m = re.fullmatch(r"\$\{\{\s*steps\.([\w-]+)\.outputs\.[\w-]+\s*\}\}", ref)
    if m:
        for step in steps_before:
            if isinstance(step, dict) and step.get("id") == m.group(1):
                run = str(step.get("run", ""))
                image = str({**job_env, **(step.get("env") or {})}.get("IMAGE", ""))
                literal_images = re.findall(r"[\w.-]+\.[a-z]{2,}/[\w./-]+", run)
                return bool(allowed.search(image)) and "${IMAGE}:" in run and not literal_images
        return False
    repo = ref.rsplit(":", 1)[0] if ":" in ref.rsplit("/", 1)[-1] else ref
    return bool(allowed.search(repo))


def workflow_pushes(rel: str, doc: dict, allowed, action_ok: bool, root: str) -> list[str]:
    out = []
    jobs = [((doc.get("runs") or {}).get("steps") or [], {})]
    for job in (doc.get("jobs") or {}).values():
        if isinstance(job, dict):
            jobs.append((job.get("steps") or [], job.get("env") or {}))
    for steps, job_env in jobs:
        refused: set[str] = set()
        for n, step in enumerate(steps):
            if not isinstance(step, dict):
                continue
            label = f"{rel} step {step.get('name') or step.get('uses') or '?'}"
            uses = str(step.get("uses", ""))
            with_ = step.get("with") or {}
            if uses == REFUSAL_ACTION:
                disarmed = str(step.get("continue-on-error", False)).lower() not in ("false", "")
                if action_ok and "if" not in step and not disarmed:
                    refused.add(str(with_.get("tag", "")).strip())
                elif disarmed:
                    out.append(f"{label}: the refusal has continue-on-error, so it refuses nothing")
                continue
            frozen = frozenset(refused)
            if uses.startswith("docker/build-push-action") and (
                    str(with_.get("push", "")).lower() not in ("false", "") or output_pushes(with_.get("outputs", ""))):
                for ref in re.split(r"[\n,]", str(with_.get("tags", ""))):
                    if ref.strip() and _allowed_ref(ref, allowed, steps[:n], job_env):
                        continue
                    problem = ref_problem(ref, rel, frozen)
                    if problem:
                        out.append(f"{label}: {problem}")
            if uses.startswith("docker/bake-action"):
                words = ["docker", "buildx", "bake"]
                if str(with_.get("push", "")).lower() == "true":
                    words.append("--push")
                for line in str(with_.get("set", "")).splitlines():
                    if line.strip():
                        words += ["--set", line.strip()]
                for f in re.split(r"[\n,]", str(with_.get("files", "") or with_.get("source", ""))):
                    if f.strip() and not f.strip().startswith(("http", "git@", "{{")):
                        words += ["-f", f.strip()]
                if build_pushes(words, root):
                    out.append(f"{label}: docker/bake-action pushes the tags its bake file names, which this guard cannot check")
            if uses.startswith("docker/metadata-action") and "latest=false" not in str(with_.get("flavor", "")).replace(" ", ""):
                out.append(f"{label}: docker/metadata-action adds latest unless its flavor says latest=false")
            run = str(step.get("run", ""))
            if run:
                for problem in shell_pushes(run, rel, frozen, root):
                    refs = re.findall(r"pushes (\S+?)(?:,|$)", problem)
                    if refs and allowed and all(_allowed_ref(r, allowed, steps[:n], job_env) for r in refs):
                        continue
                    out.append(f"{label}: {problem}")
    return out


def latest_pushes(root: str) -> list[str]:
    """Places that push, or may push, a `latest` tag they may not move."""
    out = []
    action_ok, mk_ok = refusal_implementations(root)
    for path in sorted(glob.glob(os.path.join(root, ".github", "workflows", "*.yml"))
                       + glob.glob(os.path.join(root, ".github", "actions", "**", "action.yml"), recursive=True)):
        rel = os.path.relpath(path, root).replace(os.sep, "/")
        allowed = LATEST_PUBLISHERS.get(rel, False)
        if allowed is None:
            continue
        try:
            with open(path) as fh:
                doc = yaml.safe_load(fh) or {}
        except (OSError, yaml.YAMLError) as err:
            raise Unreadable(f"{rel}: {err}") from err
        out += workflow_pushes(rel, doc, allowed, action_ok, root)
    for dirpath, dirs, files in os.walk(root):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for name in files:
            if not (name == "Makefile" or name.endswith((".mk", ".sh"))):
                continue
            path = os.path.join(dirpath, name)
            rel = os.path.relpath(path, root).replace(os.sep, "/")
            try:
                with open(path, errors="replace") as fh:
                    text = fh.read()
            except OSError:
                continue
            is_make = name == "Makefile" or name.endswith(".mk")
            refused = frozenset({"$(VERSION)"}) if is_make and mk_ok and makefile_refuses(text) else frozenset()
            for problem in shell_pushes(text, rel, refused, root):
                out.append(f"{rel}: {problem}")
    return out


def check(root: str) -> list[str]:
    problems = [f"{p}; only the release and rebuild workflows move latest, which means the last release"
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
        # The shared refusals the guard trusts.
        os.makedirs(os.path.join(root, ".github", "actions", "refuse-latest-tag"))
        with open(os.path.join(root, ".github", "actions", "refuse-latest-tag", "action.yml"), "w") as fh:
            fh.write(open(os.path.join(ROOT, ".github", "actions", "refuse-latest-tag", "action.yml")).read())
        os.makedirs(os.path.join(root, "scripts", "lib"), exist_ok=True)
        with open(os.path.join(root, "scripts", "lib", REFUSAL_MK), "w") as fh:
            fh.write(open(os.path.join(ROOT, "scripts", "lib", REFUSAL_MK)).read())
        with open(os.path.join(wf, "deploy-x.yml"), "w") as fh:
            fh.write("""jobs:
  build:
    steps:
      - name: push main
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: |
            ghcr.io/neokapi/x:main
            ghcr.io/neokapi/x:abc123
      - name: push latest
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: ghcr.io/neokapi/x:latest
      - name: build latest locally
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
            fh.write("include scripts/lib/refuse-latest-version.mk\n"
                     "push:\n\tdocker push $(IMAGE):$(VERSION)\n\tdocker push $(IMAGE):latest\n# docker push $(IMAGE):latest in a comment\n")
        os.makedirs(os.path.join(root, "bowrain", "deploy", "sub"))
        with open(os.path.join(root, "bowrain", "deploy", "sub", "Makefile"), "w") as fh:
            fh.write("p:\n\tdocker buildx build -t $(IMG):latest --push .\n")
        with open(os.path.join(root, "bowrain", "images.mk"), "w") as fh:
            fh.write("include scripts/lib/refuse-latest-version.mk\n"
                     "p:\n\tdocker tag $(IMG):$(VERSION) $(IMG):latest\n\tdocker push $(IMG):$(VERSION)\n")
        os.makedirs(os.path.join(root, "tools"))
        with open(os.path.join(root, "tools", "push.sh"), "w") as fh:
            fh.write("#!/bin/sh\n"
                     "docker buildx imagetools create \\\n  -t \"ghcr.io/neokapi/x:latest\" \\\n  ghcr.io/neokapi/x:1.0\n"
                     "skopeo copy oci:dir docker://ghcr.io/neokapi/y:latest\n"
                     "crane copy ghcr.io/neokapi/z:1 ghcr.io/neokapi/z:latest\n"
                     "docker push \"ghcr.io/neokapi/w:$TAG\"\n"
                     "docker push \"$ref\"\n")
        with open(os.path.join(wf, "deploy-y.yml"), "w") as fh:
            fh.write("""jobs:
  build:
    steps:
      - name: meta default
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/neokapi/x
      - name: meta off
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/neokapi/x
          flavor: latest=false
      - uses: ./.github/actions/refuse-latest-tag
        with:
          tag: ${{ inputs.image-tag }}
      - name: main only
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: |
            ghcr.io/neokapi/x:${{ inputs.image-tag }}
            ${{ github.ref == 'refs/heads/main' && 'ghcr.io/neokapi/x:main' || '' }}
      - name: maybe latest
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: ${{ github.ref == 'refs/heads/main' && 'ghcr.io/neokapi/x:latest' || '' }}
      - name: unknown tag
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: ghcr.io/neokapi/x:${{ steps.x.outputs.tag }}
""")
        found = latest_pushes(root)
        for want in ("step push latest", "step retag latest", "bowrain/Makefile", "deploy/sub/Makefile",
                     "images.mk: docker tag, then a push", "tools/push.sh: imagetools create",
                     "tools/push.sh: skopeo copy", "tools/push.sh: crane copy", "w:$TAG, whose tag $TAG",
                     "pushes $ref, whose tag is not written out", "step meta default", "step maybe latest",
                     "step unknown tag"):
            if not any(want in f for f in found):
                failures.append(f"latest push not caught: {want!r} in {found}")
        for quiet in ("push main", "scan latest", "rebuild-release-images", "in a comment", "step meta off",
                      "step main only", "build latest locally"):
            if any(quiet in f for f in found):
                failures.append(f"{quiet!r} pushes no latest, or may: {found}")
        push = """      - name: {name}
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: ghcr.io/neokapi/r:${{{{ inputs.image-tag }}}}
"""
        refuse = """      - uses: ./.github/actions/refuse-latest-tag
        {cond}with:
          tag: ${{{{ inputs.image-tag }}}}
"""
        with open(os.path.join(wf, "deploy-z.yml"), "w") as fh:
            fh.write("jobs:\n"
                     "  refused:\n    steps:\n" + refuse.format(cond="") + push.format(name="refused push") +
                     "  other-job:\n    steps:\n" + push.format(name="refusal in another job") +
                     "  conditional:\n    steps:\n" + refuse.format(cond="if: false\n        ") + push.format(name="refusal behind if") +
                     "  after:\n    steps:\n" + push.format(name="refusal after the push") + refuse.format(cond="") +
                     "  named-only:\n    steps:\n      - name: Refuse latest as the image tag\n        run: echo ok\n" +
                     push.format(name="a step named like the refusal"))
        with open(os.path.join(wf, "release.yml"), "w") as fh:
            fh.write("""jobs:
  publish-cli-image:
    env:
      IMAGE: ghcr.io/${{ github.repository_owner }}/kapi
    steps:
      - name: Compute image tags
        id: tags
        run: echo "base=${IMAGE}:1,${IMAGE}:latest" >> "$GITHUB_OUTPUT"
      - name: kapi latest
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: ${{ steps.tags.outputs.base }}
      - name: kapi literal latest
        run: docker buildx imagetools create -t ghcr.io/neokapi/kapi:latest x
      - name: other image in the kapi job
        run: docker buildx imagetools create -t ghcr.io/neokapi/notkapi:latest x
  other:
    steps:
      - name: other image from release.yml
        run: docker buildx imagetools create -t ghcr.io/neokapi/notkapi:latest x
""")
        with open(os.path.join(wf, "deploy-u.yml"), "w") as fh:
            fh.write("""jobs:
  outputs:
    steps:
      - name: registry output
        uses: docker/build-push-action@v7
        with:
          outputs: type=registry
          tags: ghcr.io/neokapi/o:latest
      - name: output two words
        run: docker buildx build --output type=registry -t ghcr.io/neokapi/a:latest .
      - name: output short flag
        run: docker buildx build -o type=registry -t ghcr.io/neokapi/b:latest .
      - name: output with equals
        run: docker buildx build --output=type=image,push=true -t ghcr.io/neokapi/c:latest .
      - name: output one word
        run: docker buildx build --output=type=registry -t ghcr.io/neokapi/f:latest .
      - name: output local only
        run: docker buildx build --output type=local,dest=out -t ghcr.io/neokapi/d:latest .
      - name: bake set output
        run: docker buildx bake --set '*.output=type=registry'
      - name: bake push
        run: docker buildx bake --push
      - name: bake file push
        run: docker buildx bake -f pushing.hcl
""")
        with open(os.path.join(root, "pushing.hcl"), "w") as fh:
            fh.write('target "x" {\n  tags = ["ghcr.io/neokapi/e:latest"]\n  output = ["type=registry"]\n}\n')
        os.makedirs(os.path.join(root, ".github", "scripts"), exist_ok=True)
        with open(os.path.join(root, ".github", "scripts", "push.sh"), "w") as fh:
            fh.write("docker push ghcr.io/neokapi/g:latest\n")
        with open(os.path.join(root, "bowrain", "included.mk"), "w") as fh:
            fh.write("include $(ROOT)/scripts/lib/refuse-latest-version.mk\np:\n\tdocker push $(IMG):$(VERSION)\n")
        with open(os.path.join(root, "bowrain", "conditional.mk"), "w") as fh:
            fh.write("ifdef CI\ninclude $(ROOT)/scripts/lib/refuse-latest-version.mk\nendif\np:\n\tdocker push $(IMG):$(VERSION)\n")
        with open(os.path.join(root, "bowrain", "commented.mk"), "w") as fh:
            fh.write("# include $(ROOT)/scripts/lib/refuse-latest-version.mk\np:\n\tdocker push $(IMG):$(VERSION)\n")
        with open(os.path.join(root, "bowrain", "nested.mk"), "w") as fh:
            fh.write("ifdef CI\nifeq ($(VERSION),latest)\n$(error no)\nendif\nendif\np:\n\tdocker push $(IMG):$(VERSION)\n")
        with open(os.path.join(root, "bowrain", "noerror.mk"), "w") as fh:
            fh.write("ifeq ($(VERSION),latest)\n$(info VERSION is latest)\nendif\np:\n\tdocker push $(IMG):$(VERSION)\n")
        found = latest_pushes(root)
        for want in ("step refusal in another job", "step refusal behind if", "step refusal after the push",
                     "step a step named like the refusal", "step other image in the kapi job",
                     "step other image from release.yml", "step registry output", "step output two words",
                     "step output short flag", "step output with equals", "step bake set output", "step bake push",
                     "step bake file push", ".github/scripts/push.sh", "bowrain/conditional.mk", "bowrain/commented.mk",
                     "step output one word", "bowrain/nested.mk", "bowrain/noerror.mk"):
            if not any(want in f for f in found):
                failures.append(f"bypass not caught: {want!r} in {found}")
        for quiet in ("step refused push", "step kapi latest", "step kapi literal latest", "step output local only",
                      "bowrain/included.mk"):
            if any(quiet in f for f in found):
                failures.append(f"{quiet!r} is allowed, but was flagged: {found}")
        if sum(f.startswith("bowrain/Makefile") for f in found) != 1:
            failures.append(f"the Makefile's version push and its comment are not flagged: {found}")
    # The four holes in the refusal, and docker/bake-action.
    def tree(action: str, fragment: str) -> str:
        top = tempfile.mkdtemp()
        os.makedirs(os.path.join(top, ".github", "actions", "refuse-latest-tag"))
        os.makedirs(os.path.join(top, ".github", "workflows"))
        os.makedirs(os.path.join(top, "scripts", "lib"))
        with open(os.path.join(top, ".github", "actions", "refuse-latest-tag", "action.yml"), "w") as fh:
            fh.write(action)
        with open(os.path.join(top, "scripts", "lib", REFUSAL_MK), "w") as fh:
            fh.write(fragment)
        return top

    with open(os.path.join(ROOT, ".github", "actions", "refuse-latest-tag", "action.yml")) as fh:
        real_action = fh.read()
    with open(os.path.join(ROOT, "scripts", "lib", REFUSAL_MK)) as fh:
        real_fragment = fh.read()
    # A raw comparison with latest lets `latest ` and a second line through.
    raw_action = real_action.split("      run: |")[0] + """      run: |
        if [ -z "$TAG" ] || [ "$TAG" = latest ]; then
          exit 1
        fi
"""
    raw_fragment = "ifeq ($(VERSION),latest)\n$(error no)\nendif\n"
    refused_push = """jobs:
  j:
    steps:
      - uses: ./.github/actions/refuse-latest-tag
        {extra}with:
          tag: {tag}
      - name: {name}
        uses: docker/build-push-action@v7
        with:
          push: true
          tags: ghcr.io/neokapi/r:{tag}
"""
    import shutil
    good = tree(real_action, real_fragment)
    weak = tree(raw_action, raw_fragment)
    try:
        if refusal_implementations(good) != (True, True):
            failures.append(f"the repository's refusals must refuse every bad probe: {refusal_implementations(good)}")
        if refusal_implementations(weak) != (False, False):
            failures.append(f"a raw comparison with latest lets `latest ` through and must not be trusted: "
                            f"{refusal_implementations(weak)}")
        cases = {
            "raw action": (weak, "", "${{ inputs.image-tag }}", True),
            "env expression": (good, "", "${{ env.TAG }}", True),
            "continue-on-error": (good, "continue-on-error: true\n        ", "${{ inputs.image-tag }}", True),
            "sound refusal": (good, "", "${{ inputs.image-tag }}", False),
        }
        for name, (top, extra, tag, want) in cases.items():
            with open(os.path.join(top, ".github", "workflows", f"{name.replace(' ', '-')}.yml"), "w") as fh:
                fh.write(refused_push.format(extra=extra, tag=tag, name=name))
        for top in (good, weak):
            with open(os.path.join(top, "Makefile"), "w") as fh:
                fh.write("include scripts/lib/refuse-latest-version.mk\np:\n\tdocker push $(IMG):$(VERSION)\n")
        with open(os.path.join(good, ".github", "workflows", "bake.yml"), "w") as fh:
            fh.write("jobs:\n  b:\n    steps:\n      - name: bake action\n        uses: docker/bake-action@v6\n"
                     "        with:\n          push: true\n")
        found_good, found_weak = latest_pushes(good), latest_pushes(weak)
        found = found_good + found_weak
        for name, (top, extra, tag, want) in cases.items():
            flagged = any(f"step {name}:" in f for f in found)
            if flagged != want:
                failures.append(f"{name}: {'must be flagged' if want else 'is sound and was flagged'}: {found}")
        if not any("the refusal has continue-on-error" in f for f in found):
            failures.append(f"a refusal with continue-on-error must be named: {found}")
        if not any(f.startswith("Makefile: docker push") for f in found_weak):
            failures.append(f"a Makefile under a fragment that lets `latest ` through must be flagged: {found_weak}")
        if any(f.startswith("Makefile: docker push") for f in found_good):
            failures.append(f"a Makefile under the repository's fragment is sound and was flagged: {found_good}")
        if not any("step bake action: docker/bake-action" in f for f in found):
            failures.append(f"a pushing docker/bake-action step must be flagged: {found}")
    finally:
        shutil.rmtree(good)
        shutil.rmtree(weak)
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
