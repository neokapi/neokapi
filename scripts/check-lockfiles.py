#!/usr/bin/env python3
"""Hold every committed pnpm lockfile to one form.

    check-lockfiles.py             check every tracked pnpm-lock.yaml
    check-lockfiles.py --self-test run the fixtures

pnpm writes a package's tarball URL into the lockfile only when the package
comes from a registry other than the configured one. A developer whose own
~/.npmrc names a mirror therefore writes a lockfile no other machine writes.
The repository's .npmrc names the registry, so every machine resolves from
it, and each project with a lockfile of its own carries an .npmrc naming the
same one (pnpm reads no .npmrc above a project outside the workspace).

Every lockfile must parse as pnpm reads it, resolve every dependency of an
importer or a package to an entry with an integrity hash, and hold no
resolution other than that hash.
"""

import os
import re
import subprocess
import sys
import tempfile

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "lib"))
from pnpm_lockfile import Lock, LockError, form_problems, integrity_problems  # noqa: E402


def registry(npmrc_path):
    """The `registry=` an .npmrc names, or None."""
    try:
        with open(npmrc_path) as f:
            for line in f:
                m = re.match(r"^\s*registry\s*=\s*(\S+)\s*$", line)
                if m:
                    return m.group(1).rstrip("/") + "/"
    except FileNotFoundError:
        pass
    return None


def check(root, lockfiles):
    problems = []
    want = registry(os.path.join(root, ".npmrc"))
    if not want:
        problems.append(".npmrc: names no registry")
    for path in lockfiles:
        d = os.path.dirname(path)
        if d:
            got = registry(os.path.join(root, d, ".npmrc"))
            if got != want:
                problems.append(f"{d}/.npmrc: registry {got} (the repository's is {want})")
        try:
            with open(os.path.join(root, path)) as f:
                lock = Lock(f.read(), path)
        except LockError as e:
            problems.append(str(e))
            continue
        base = os.path.realpath(os.path.join(root, d))

        def in_repo(rel, base=base):
            target = os.path.realpath(os.path.join(base, rel))
            return target.startswith(os.path.realpath(root) + os.sep) and os.path.isdir(target)

        problems += [f"{path}: {p}" for p in integrity_problems(lock) + form_problems(lock, in_repo)]
    return problems


def tracked_lockfiles(root):
    out = subprocess.check_output(["git", "-C", root, "ls-files", "pnpm-lock.yaml", "**/pnpm-lock.yaml"]).decode()
    return sorted(out.split())


GOOD = """---
lockfileVersion: '9.0'

importers:

  .:
    configDependencies: {}

packages:

  pnpm@12.21.0:
    resolution: {integrity: sha512-pm}

snapshots:

  pnpm@12.21.0: {}

---
lockfileVersion: '9.0'

importers:

  .:
    dependencies:
      left-pad:
        specifier: ^1.3.0
        version: 1.3.0

packages:

  left-pad@1.3.0:
    resolution: {integrity: sha512-lp}

  '@scope/util@2.0.0':
    resolution: {integrity: sha512-su}

snapshots:

  left-pad@1.3.0:
    dependencies:
      '@scope/util': 2.0.0

  '@scope/util@2.0.0': {}
"""


def self_test():
    failures = []
    cases = {
        "the good lockfile": (GOOD, None),
        "a tarball field": (GOOD.replace("{integrity: sha512-lp}", "{integrity: sha512-lp, tarball: https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz}"), "not an integrity hash alone"),
        "a tarball in the package manager's document": (GOOD.replace("{integrity: sha512-pm}", "{integrity: sha512-pm, tarball: https://registry.npmjs.org/pnpm/-/pnpm-12.21.0.tgz}"), "pnpm@12.21.0"),
        "a git resolution": (GOOD.replace("{integrity: sha512-lp}", "{type: git, repo: https://example.com/left-pad, commit: abc}"), "not an integrity hash alone"),
        "no integrity": (GOOD.replace("{integrity: sha512-su}", "{tarball: https://registry.npmjs.org/x.tgz}"), "no integrity hash"),
        "a dependency with no entry": (GOOD.replace("'@scope/util': 2.0.0", "'@scope/util': 2.0.1"), "no packages entry"),
        "an importer version with no entry": (GOOD.replace("version: 1.3.0", "version: 1.3.1"), "importer . -> left-pad@1.3.1: no packages entry"),
        "a directory outside the repository": (GOOD.replace("  '@scope/util@2.0.0':\n    resolution: {integrity: sha512-su}", "  '@scope/util@file:../../elsewhere':\n    resolution: {directory: ../../elsewhere, type: directory}").replace("'@scope/util': 2.0.0", "'@scope/util': file:../../elsewhere").replace("  '@scope/util@2.0.0': {}", "  '@scope/util@file:../../elsewhere': {}"), "not an integrity hash alone"),
        "a directory inside the repository": (GOOD.replace("  '@scope/util@2.0.0':\n    resolution: {integrity: sha512-su}", "  '@scope/util@file:sub':\n    resolution: {directory: sub, type: directory}").replace("'@scope/util': 2.0.0", "'@scope/util': file:sub").replace("  '@scope/util@2.0.0': {}", "  '@scope/util@file:sub': {}"), None),
        "a duplicate key": (GOOD.replace("  '@scope/util@2.0.0':\n    resolution", "  left-pad@1.3.0:\n    resolution: {integrity: sha512-x}\n\n  '@scope/util@2.0.0':\n    resolution"), "keys by line"),
    }
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, "sub"))
        with open(os.path.join(root, ".npmrc"), "w") as f:
            f.write("registry=https://registry.npmjs.org/\n")
        for label, (text, want) in cases.items():
            with open(os.path.join(root, "pnpm-lock.yaml"), "w") as f:
                f.write(text)
            got = check(root, ["pnpm-lock.yaml"])
            if want is None and got:
                failures.append(f"{label}: expected no problems, got {got}")
            if want is not None and not any(want in p for p in got):
                failures.append(f"{label}: expected {want!r}, got {got}")
        # A project with a lockfile of its own and no .npmrc, then a
        # different registry.
        with open(os.path.join(root, "sub", "pnpm-lock.yaml"), "w") as f:
            f.write(GOOD)
        if not any("sub/.npmrc: registry None" in p for p in check(root, ["sub/pnpm-lock.yaml"])):
            failures.append("a project lockfile without an .npmrc: not reported")
        with open(os.path.join(root, "sub", ".npmrc"), "w") as f:
            f.write("registry=https://registry.yarnpkg.com/\n")
        if not any("registry.yarnpkg.com" in p for p in check(root, ["sub/pnpm-lock.yaml"])):
            failures.append("a project .npmrc naming another registry: not reported")
        os.remove(os.path.join(root, ".npmrc"))
        if not any("names no registry" in p for p in check(root, [])):
            failures.append("a repository .npmrc without a registry: not reported")
    for f in failures:
        print(f"check-lockfiles self-test: FAILED: {f}")
    if not failures:
        print("check-lockfiles self-test: OK")
    return 1 if failures else 0


def main():
    if sys.argv[1:] == ["--self-test"]:
        return self_test()
    root = subprocess.check_output(["git", "rev-parse", "--show-toplevel"]).decode().strip()
    lockfiles = tracked_lockfiles(root)
    problems = check(root, lockfiles)
    for p in problems:
        print(f"::error::{p}")
    if problems:
        print("Each lockfile is written with the registry the repository's .npmrc names. "
              "Re-resolve it with the pinned pnpm (vp install) from a checkout of this branch.")
        return 1
    print(f"check-lockfiles: {len(lockfiles)} lockfiles, every resolution an integrity hash from the configured registry")
    return 0


if __name__ == "__main__":
    sys.exit(main())
