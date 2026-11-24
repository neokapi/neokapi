#!/usr/bin/env python3
"""Name the last Bowrain release: the tag the weekly image rebuild builds.

    scripts/last-release-tag.py <owner/repo>   print the tag, or fail
    scripts/last-release-tag.py --self-test

The last release is the highest `bowrain-vX.Y.Z` or `bowrain-vX.Y.Z-rcN` tag,
a release candidate below its release. That tag must have a published GitHub
Release (not a draft), and its commit must be reachable from main. A tag that
fails either check stops the run with the reason; the script never falls back
to a lower tag, because a stray higher tag is exactly what it must not build.

It reads the public tag list and two public API endpoints, so it needs no
token. Exit status: 0 the tag is printed, 1 the highest tag fails a check or
none exists, 2 usage.
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
import urllib.error
import urllib.request

TAG = re.compile(r"^bowrain-v(\d+)\.(\d+)\.(\d+)(?:-rc(\d+))?$")


def order(tag: str) -> tuple:
    major, minor, patch, rc = TAG.match(tag).groups()
    # A release sorts above all of its release candidates.
    return (int(major), int(minor), int(patch), int(rc) if rc else 1 << 30)


def highest(tags: list[str]) -> str | None:
    versions = [t for t in tags if TAG.match(t)]
    return max(versions, key=order) if versions else None


def problems(tag: str, release: dict | None, on_main: str) -> list[str]:
    """Why the tag may not be built; empty when it may."""
    out = []
    if release is None:
        out.append(f"{tag} has no GitHub Release")
    elif release.get("draft"):
        out.append(f"{tag}'s GitHub Release is a draft")
    if on_main not in ("ahead", "identical"):
        out.append(f"{tag}'s commit is not reachable from main (compare: {on_main or 'unknown'})")
    return out


def api(repo: str, path: str) -> dict | None:
    req = urllib.request.Request(f"https://api.github.com/repos/{repo}/{path}",
                                 headers={"Accept": "application/vnd.github+json", "User-Agent": "last-release-tag"})
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return json.load(r)
    except urllib.error.HTTPError as err:
        if err.code == 404:
            return None
        raise


def main(repo: str) -> int:
    out = subprocess.run(["git", "ls-remote", "--tags", "--refs", f"https://github.com/{repo}", "refs/tags/bowrain-v*"],
                         capture_output=True, text=True, check=True).stdout
    tag = highest([line.split("refs/tags/", 1)[1] for line in out.splitlines() if "refs/tags/" in line])
    if tag is None:
        print("::error::no bowrain-v* version tag exists")
        return 1
    release = api(repo, f"releases/tags/{tag}")
    compare = api(repo, f"compare/{tag}...main") or {}
    found = problems(tag, release, compare.get("status", ""))
    for p in found:
        print(f"::error::the last release cannot be built: {p}; the rebuild stops rather than build a lower tag")
    if found:
        return 1
    print(tag)
    return 0


def self_test() -> int:
    failures = []
    tags = ["bowrain-v1.3.0", "bowrain-v1.4.0-rc5", "bowrain-v1.4.0-rc10", "v1.9.0", "bowrain-v1.4.0-rc9x"]
    if highest(tags) != "bowrain-v1.4.0-rc10":
        failures.append(f"highest: {highest(tags)}")
    if highest(tags + ["bowrain-v1.4.0"]) != "bowrain-v1.4.0":
        failures.append("a release sorts above its release candidates")
    published = {"draft": False, "prerelease": True}
    cases = [
        ("published release on main", published, "ahead", False),
        ("tag at main's head", published, "identical", False),
        ("a higher tag with no release", None, "ahead", True),
        ("a draft release", {"draft": True}, "ahead", True),
        ("a tag whose commit is not on main", published, "diverged", True),
        ("a tag ahead of main", published, "behind", True),
        ("compare unreadable", published, "", True),
    ]
    for name, release, status, want_error in cases:
        got = problems("bowrain-v9.9.9", release, status)
        if bool(got) != want_error:
            failures.append(f"{name}: {got}")
    if failures:
        print("last-release-tag self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("last-release-tag self-test: OK")
    return 0


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-test"]:
        sys.exit(self_test())
    if len(sys.argv) == 2:
        sys.exit(main(sys.argv[1]))
    print(__doc__)
    sys.exit(2)
