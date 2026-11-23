#!/usr/bin/env python3
"""Re-resolve a Dependabot pull request's pnpm-lock.yaml with the pinned pnpm.

    dependabot-relock.py --base-ref <branch>   relock the working tree against
                                               origin/<branch>
    dependabot-relock.py --base <rev>          the same, against a revision
                                               already present
    dependabot-relock.py --self-test           run the fixtures

Dependabot resolves the lockfile from scratch with its own pnpm and can move
packages the update never named. The relock never edits a lockfile itself:

1. It starts from the base branch's lockfile, and `vp install
   --lockfile-only` applies the pull request's package.json changes to it.
2. What the update asked for and the result does not yet hold is updated
   by name with pnpm's own `update --depth Infinity --lockfile-only`. A
   package.json bump asks for the packages whose specifiers changed, at the
   importers that name them. An update that changes no specifier (a security
   fix of a transitive package) asks for every package Dependabot raised.
   Dependabot resolves from scratch, so a bump also raises unrelated
   packages; those are not part of the update and need not be kept.

The result is then checked against both lockfiles, and any failure puts
Dependabot's lockfile back and exits 1 with the list:

- Form: every dependency has an entry whose resolution is an integrity hash
  from the configured registry (scripts/lib/pnpm_lockfile.py, the rules
  check-lockfiles holds every committed lockfile to).
- Kept: every package the update asked for is in the result at
  Dependabot's version or higher, for each dependent Dependabot gave it to.
- Nothing lowered: no dependent the base and the result share holds a
  package below its base version. A version may leave the lockfile when its
  dependents moved above it or went away. pnpm can lower one while resolving
  a bump (a new dependency pins an older version of something shared, and
  pnpm moves the other dependents onto it); that is refused, named, and left
  to a person.
- Integrity: an entry the base also has keeps the base's integrity hash.
- No package.json changed.

The base is fetched without changing the repository's depth: a shallow
checkout stays shallow and a full one stays full.

RELOCK_INSTALL_CMD and RELOCK_UPDATE_CMD stand in for the two pnpm runs in
the self-test.
"""

import os
import shlex
import subprocess
import sys
import tempfile

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "lib"))
from pnpm_lockfile import Lock, LockError, form_problems, integrity_problems, version_key  # noqa: E402

LOCK = "pnpm-lock.yaml"
INSTALL_CMD = "vp install --lockfile-only"
UPDATE_CMD = "vp update -r --no-save {names} -- --depth Infinity --lockfile-only"


def dependent_label(dependent):
    """'importer bowrain/apps/web' stays; a snapshot key loses its peers."""
    return dependent if dependent.startswith("importer ") else dependent.split("(", 1)[0]


def integrity(res):
    return res.get("integrity") if isinstance(res, dict) else None


def asked(base, dep):
    """The packages a package.json bump asked for: those whose specifier
    changed at some importer. Empty for an update that changes none."""
    return {name for key in set(base.specifiers) | set(dep.specifiers)
            for name in [key[1]] if base.specifiers.get(key) != dep.specifiers.get(key)}


def missing_raises(base, dep, result):
    """Problems where the result does not hold what the update asked for, and
    the names they concern."""
    problems, names = [], set()
    bump = asked(base, dep)
    if bump:
        for edge, dv in sorted(dep.edges.items()):
            if not edge[0].startswith("importer ") or edge[1] not in bump:
                continue
            rv = result.edges.get(edge)
            if rv is None or version_key(rv) < version_key(dv):
                problems.append(f"{edge[1]}: the update asks for {dv} at {edge[0]}; the result has {rv or 'none'}")
                names.add(edge[1])
        return problems, names
    for (name, ver) in sorted(set(dep.entries) - set(base.entries)):
        if not any(version_key(v) >= version_key(ver) for v in result.versions(name)):
            have = ", ".join(sorted(result.versions(name), key=version_key)) or "none"
            problems.append(f"{name} {ver}: Dependabot added it; the result has {have}")
            names.add(name)
    for edge, dv in sorted(dep.edges.items()):
        bv, rv = base.edges.get(edge), result.edges.get(edge)
        if bv and rv and version_key(dv) > version_key(bv) and version_key(rv) < version_key(dv):
            problems.append(f"{edge[1]}: Dependabot raised it from {bv} to {dv} for {dependent_label(edge[0])}; the result has {rv}")
            names.add(edge[1])
    return problems, names


def check(base, dep, result):
    problems = integrity_problems(result) + form_problems(result)
    problems += missing_raises(base, dep, result)[0]
    for edge, bv in sorted(base.edges.items()):
        rv = result.edges.get(edge)
        if rv and version_key(rv) < version_key(bv):
            problems.append(f"{edge[1]} went down from {bv} to {rv} for {dependent_label(edge[0])}")
    for key, res in sorted(result.entries.items()):
        if key in base.entries and integrity(res) != integrity(base.entries[key]):
            problems.append(f"{key[0]} {key[1]}: integrity {integrity(res)} differs from the base's {integrity(base.entries[key])}")
    return problems


def git(*args):
    return subprocess.check_output(["git", *args]).decode()


def fetch_base(ref):
    """Fetch origin/<ref> and return its commit, keeping the repository's
    depth: --depth=1 only when the repository is already shallow."""
    shallow = git("rev-parse", "--is-shallow-repository").strip() == "true"
    depth = ["--depth=1"] if shallow else []
    subprocess.run(["git", "fetch", "--no-tags", *depth, "origin", f"refs/heads/{ref}"], check=True)
    return git("rev-parse", "FETCH_HEAD").strip()


def run(cmd):
    print(f"dependabot-relock: {' '.join(cmd)}")
    subprocess.run(cmd, check=True)


def relock(base_rev):
    base_text = git("show", f"{base_rev}:{LOCK}")
    with open(LOCK) as f:
        dep_text = f.read()
    manifests_before = git("diff", "--name-only", "--", "package.json", "**/package.json")

    def put_back(message, problems=()):
        with open(LOCK, "w") as f:
            f.write(dep_text)
        print(f"::error::{message}; nothing is pushed")
        for p in problems:
            print("  " + p)
        return 1

    try:
        base, dep = Lock(base_text, "base"), Lock(dep_text, "Dependabot's lockfile")
        with open(LOCK, "w") as f:
            f.write(base_text)
        run(shlex.split(os.environ.get("RELOCK_INSTALL_CMD", INSTALL_CMD)))
        with open(LOCK) as f:
            result = Lock(f.read(), "result")
        _, names = missing_raises(base, dep, result)
        if names:
            template = os.environ.get("RELOCK_UPDATE_CMD", UPDATE_CMD)
            run([part for t in shlex.split(template)
                 for part in (sorted(names) if t == "{names}" else [t])])
            with open(LOCK) as f:
                result = Lock(f.read(), "result")
    except (LockError, subprocess.CalledProcessError) as e:
        return put_back(f"the relock failed: {e}")
    if git("diff", "--name-only", "--", "package.json", "**/package.json") != manifests_before:
        return put_back("the relock changed a package.json")
    problems = check(base, dep, result)
    if problems:
        return put_back("the relocked lockfile fails its checks (a person decides; "
                        "the lines say what moved, and through which dependent)", problems)
    print("dependabot-relock: the lockfile keeps the update and lowers nothing else")
    return 0


# Fixtures: lockfiles in pnpm's own layout, cut down to what the checks read.
def lock(packages, snapshots=None, importers=None, overrides=None):
    """packages: ['name@ver', ...]; snapshots: {key: {dep: ver}};
    importers: {path: {dep: ver}}; overrides: {key: resolution text}."""
    overrides = overrides or {}
    out = ["---", "lockfileVersion: '9.0'", "", "importers:", "", "  .:", "    configDependencies: {}", "",
           "packages:", "", "  pnpm@12.21.0:", "    resolution: {integrity: sha512-pm}", "",
           "snapshots:", "", "  pnpm@12.21.0: {}", "", "---", "lockfileVersion: '9.0'", "", "importers:", ""]

    def q(k):
        return f"'{k}'" if k.startswith("@") else k

    for path, deps in (importers or {".": {}}).items():
        out.append(f"  {path}:" + ("" if deps else " {}"))
        if deps:
            out.append("    dependencies:")
            for d, v in deps.items():
                out += [f"      {q(d)}:", f"        specifier: ^{v}", f"        version: {v}"]
        out.append("")
    out += ["packages:", ""]
    for key in packages:
        res = overrides.get(key, f"{{integrity: sha512-{key}}}")
        out += [f"  {q(key)}:", f"    resolution: {res}", ""]
    out += ["snapshots:", ""]
    snapshots = snapshots or {}
    for key in packages:
        deps = snapshots.get(key)
        if deps:
            out.append(f"  {q(key)}:")
            out.append("    dependencies:")
            out += [f"      {q(d)}: {v}" for d, v in deps.items()]
        else:
            out.append(f"  {q(key)}: {{}}")
        out.append("")
    return "\n".join(out)


def self_test():
    failures = []

    def expect(label, cond):
        if not cond:
            failures.append(label)

    def problems_of(b, d, r):
        return check(Lock(b, "base"), Lock(d, "dependabot"), Lock(r, "result"))

    pkgs = ["postcss@8.5.30", "postcss@8.5.33", "vite@8.0.0", "app-a@1.0.0", "posthog-js@1.453.2",
            "@posthog/core@0.12.2", "recharts@3.10.1", "reselect@5.3.0", "toolkit@2.0.0"]
    snaps = {"vite@8.0.0": {"postcss": "8.5.30"}, "app-a@1.0.0": {"postcss": "8.5.33"},
             "posthog-js@1.453.2": {"@posthog/core": "0.12.2"}, "toolkit@2.0.0": {"reselect": "5.3.0"}}
    imps = {".": {"vite": "8.0.0", "app-a": "1.0.0", "posthog-js": "1.453.2", "recharts": "3.10.1", "toolkit": "2.0.0"}}
    base = lock(pkgs, snaps, imps)

    # A lockfile-only security update of a transitive package: postcss 8.5.30
    # goes, vite takes 8.5.33.
    sec_pkgs = [p for p in pkgs if p != "postcss@8.5.30"]
    sec_snaps = {**snaps, "vite@8.0.0": {"postcss": "8.5.33"}}
    dep_sec = lock(sec_pkgs, sec_snaps, imps)
    good_sec = lock(sec_pkgs, sec_snaps, imps)
    expect("security update, done by pnpm from the base: passes", problems_of(base, dep_sec, good_sec) == [])
    p = problems_of(base, dep_sec, base)
    expect("security update thrown away: refused, naming postcss and vite",
           any(x.startswith("postcss: Dependabot raised it from 8.5.30 to 8.5.33 for vite@8.0.0") for x in p))
    expect("security update thrown away: pnpm is asked to update postcss",
           missing_raises(Lock(base, "b"), Lock(dep_sec, "d"), Lock(base, "r"))[1] == {"postcss"})

    # A package.json bump, as on 2026-11-23: Dependabot raised recharts and
    # lowered posthog-js.
    bump_pkgs = [p.replace("recharts@3.10.1", "recharts@3.11.0") for p in pkgs]
    bump_imps = {".": {**imps["."], "recharts": "3.11.0"}}
    dep_bump = lock([p.replace("posthog-js@1.453.2", "posthog-js@1.453.1").replace("@posthog/core@0.12.2", "@posthog/core@0.12.1")
                     for p in bump_pkgs],
                    {**snaps, "posthog-js@1.453.1": {"@posthog/core": "0.12.1"}},
                    {".": {**bump_imps["."], "posthog-js": "1.453.1"}})
    good_bump = lock(bump_pkgs, snaps, bump_imps)
    expect("bump from the base: passes", problems_of(base, dep_bump, good_bump) == [])
    p = problems_of(base, dep_bump, dep_bump)
    expect("Dependabot's own lockfile: posthog-js downgrade refused, from and to",
           any(x == "posthog-js went down from 1.453.2 to 1.453.1 for importer ." for x in p))

    # The bump pulls in a dependency that pins reselect 5.2.0, and pnpm moves
    # toolkit onto it: refused, with the package, both versions and the
    # dependent named.
    dedupe = lock(bump_pkgs + ["reselect@5.2.0"], {**snaps, "toolkit@2.0.0": {"reselect": "5.2.0"},
                                                   "recharts@3.11.0": {"reselect": "5.2.0"}}, bump_imps)
    dedupe = dedupe.replace("  reselect@5.3.0:\n    resolution: {integrity: sha512-reselect@5.3.0}\n\n", "").replace("  reselect@5.3.0: {}\n\n", "")
    p = problems_of(base, dep_bump, dedupe)
    expect("a dedupe downgrade: refused as 'reselect went down from 5.3.0 to 5.2.0 for toolkit@2.0.0'",
           "reselect went down from 5.3.0 to 5.2.0 for toolkit@2.0.0" in p)

    # A bump whose Dependabot lockfile also raised an unrelated package (it
    # resolves from scratch): the relock need not keep that raise.
    collateral = lock([p.replace("vite@8.0.0", "vite@8.0.1") for p in bump_pkgs], snaps,
                      {".": {**bump_imps["."], "vite": "8.0.1"}})
    collateral = collateral.replace("specifier: ^8.0.1", "specifier: ^8.0.0")
    expect("a bump's collateral raise: not required", problems_of(base, collateral, good_bump) == [])
    expect("a bump: pnpm update asked for nothing", missing_raises(Lock(base, "b"), Lock(collateral, "d"), Lock(good_bump, "r"))[1] == set())
    p = problems_of(base, dep_bump, base)
    expect("a bump missing from the result: refused, naming recharts and the importer",
           any(x.startswith("recharts: the update asks for 3.11.0 at importer .") for x in p))

    # Entry sets, per dependent: a lower version may leave when its dependent
    # moved above it; a dependent moved below its base version is refused even
    # while every entry stays.
    lost = lock(sec_pkgs, sec_snaps, imps)
    expect("a lower version that leaves because its dependent moved up: passes", problems_of(base, base, lost) == [])
    low = lock(pkgs, {**snaps, "app-a@1.0.0": {"postcss": "8.5.30"}}, imps)
    expect("a dependent lowered while every entry stays: refused",
           "postcss went down from 8.5.33 to 8.5.30 for app-a@1.0.0" in problems_of(base, base, low))

    # Parsing: a duplicate key is refused before any check.
    dup = base.replace("  recharts@3.10.1:\n    resolution:", "  recharts@3.10.1:\n    resolution: {integrity: sha512-x}\n\n  recharts@3.10.1:\n    resolution:", 1)
    try:
        Lock(dup, "dup")
        expect("a duplicate key: refused", False)
    except LockError:
        pass

    # Integrity and form.
    changed = lock(pkgs, snaps, imps, overrides={"vite@8.0.0": "{integrity: sha512-other}"})
    expect("an existing entry with another integrity: refused",
           any(x.startswith("vite 8.0.0: integrity") for x in problems_of(base, base, changed)))
    new = ["left-pad@1.3.0"]
    for label, res in (("a tarball on another host", "{integrity: sha512-lp, tarball: https://elsewhere.example/left-pad.tgz}"),
                       ("a git resolution", "{integrity: sha512-lp, type: git, repo: https://example.com/left-pad, commit: abc}"),
                       ("no integrity", "{tarball: https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz}")):
        r = lock(pkgs + new, snaps, imps, overrides={"left-pad@1.3.0": res})
        d = lock(pkgs + new, snaps, imps)
        expect(f"{label}: refused", any(x.startswith("left-pad@1.3.0") for x in problems_of(base, d, r)))
    r = lock(pkgs + new, snaps, imps)
    expect("a new registry entry: passes", problems_of(base, r, r) == [])
    expect("a package Dependabot added, missing from the result: refused",
           any(x.startswith("left-pad 1.3.0: Dependabot added it") for x in problems_of(base, r, base)))

    with tempfile.TemporaryDirectory() as d:
        env = {**os.environ, "GIT_CONFIG_GLOBAL": os.devnull, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
               "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"}
        remote, work = os.path.join(d, "remote"), os.path.join(d, "work")
        os.makedirs(remote)

        def sh(cwd, *a):
            return subprocess.run(a, cwd=cwd, env=env, check=True, capture_output=True, text=True).stdout

        def write(name, text):
            with open(os.path.join(work, name), "w") as f:
                f.write(text)

        def read(name):
            with open(os.path.join(work, name)) as f:
                return f.read()

        sh(remote, "git", "init", "-q", "-b", "main")
        with open(os.path.join(remote, LOCK), "w") as f:
            f.write(base)
        with open(os.path.join(remote, "package.json"), "w") as f:
            f.write("{}\n")
        sh(remote, "git", "add", ".")
        sh(remote, "git", "commit", "-q", "-m", "one")
        sh(remote, "git", "commit", "-q", "--allow-empty", "-m", "two")
        sh(d, "git", "clone", "-q", remote, work)

        # The base is fetched without making a full repository shallow.
        r = subprocess.run([sys.executable, "-c", f"import sys; sys.path.insert(0, {os.path.dirname(os.path.abspath(__file__))!r}); "
                            "import importlib.util as u; s = u.spec_from_file_location('r', sys.argv[1]); m = u.module_from_spec(s); "
                            "s.loader.exec_module(m); print(m.fetch_base('main'))", os.path.abspath(__file__)],
                           cwd=work, env=env, capture_output=True, text=True)
        expect("fetch_base: fetches the base", r.returncode == 0 and len(r.stdout.strip()) == 40)
        expect("fetch_base: a full repository stays full", sh(work, "git", "rev-parse", "--is-shallow-repository").strip() == "false")
        expect("fetch_base: every commit is still there", sh(work, "git", "rev-list", "--count", "HEAD").strip() == "2")

        write("cp.py", "import shutil,sys\nshutil.copy(sys.argv[1], 'pnpm-lock.yaml')\n"
                       "open('calls','a').write(' '.join(sys.argv[2:]) + '\\n')\n")
        write("good_sec", good_sec)
        write("good_bump", good_bump)
        write("base_copy", base)

        def relock_with(dep_text, install, update):
            write(LOCK, dep_text)
            if os.path.exists(os.path.join(work, "calls")):
                os.remove(os.path.join(work, "calls"))
            py = shlex.quote(sys.executable)
            return subprocess.run([sys.executable, os.path.abspath(__file__), "--base", "origin/main"], cwd=work,
                                  env={**env, "RELOCK_INSTALL_CMD": f"{py} -c pass" if install is None else f"{py} cp.py {install} install",
                                       "RELOCK_UPDATE_CMD": f"{py} cp.py {update} update {{names}}"},
                                  capture_output=True, text=True)

        r = relock_with(dep_sec, None, "good_sec")
        expect("cli security update: passes", r.returncode == 0)
        expect("cli security update: pnpm update for postcss only",
               os.path.exists(os.path.join(work, "calls")) and read("calls").strip() == "update postcss")
        expect("cli security update: result is pnpm's", read(LOCK) == good_sec)
        r = relock_with(dep_bump, "good_bump", "good_bump")
        expect("cli bump: passes", r.returncode == 0)
        expect("cli bump: no pnpm update", read("calls").strip() == "install")
        r = relock_with(dep_bump, None, "base_copy")
        expect("cli: a result that misses the bump exits 1", r.returncode == 1 and "recharts" in r.stdout)
        expect("cli: Dependabot's lockfile is put back", read(LOCK) == dep_bump)

    if failures:
        for f in failures:
            print(f"dependabot-relock self-test: FAILED: {f}")
        return 1
    print("dependabot-relock self-test: OK")
    return 0


def main(argv):
    if argv == ["--self-test"]:
        return self_test()
    if len(argv) == 2 and argv[0] == "--base":
        return relock(argv[1])
    if len(argv) == 2 and argv[0] == "--base-ref":
        return relock(fetch_base(argv[1]))
    print(__doc__)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
