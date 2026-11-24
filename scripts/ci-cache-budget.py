#!/usr/bin/env python3
"""Keep the repository's Actions caches inside their budget.

    scripts/ci-cache-budget.py prune     delete superseded caches on main
    scripts/ci-cache-budget.py check     fail when the total passes the budget
    scripts/ci-cache-budget.py report    print the cache families and sizes
    scripts/ci-cache-budget.py --self-test

GitHub keeps 10 GiB of caches per repository and evicts by age past that, so
an eviction can take the cache every pull request restores. Most of the space
went to copies nothing reads: a cache keyed `<family>-<commit sha>` is saved
anew by every merge to main, and a restore by prefix only ever picks the
newest one.

prune deletes two kinds of cache nobody will read:

  * on refs/heads/main, every cache of a family keyed by a commit sha or a
    content hash (40 or 64 hex digits) except the newest;
  * every cache on any other ref. Only main saves caches
    (scripts/check-ci-caches.py), so one elsewhere is a leftover.

It needs a token with `actions: write` and runs from main.

check fails when the ordinary caches pass ORDINARY_BYTES (7.5 GiB) or the
total passes BUDGET_BYTES (9 GiB). The warm release caches (go-release-*,
icu-static-*, about 1.8 GiB) are read only at release time, so GitHub's
least-recently-used eviction past 10 GiB takes them first; the ordinary
budget leaves them room. The scheduled cache-budget workflow runs check, and
a failure there opens an issue through scheduled-failure-issue.yml.

The listing comes from `gh api repos/{repo}/actions/caches`; GH_REPO names the
repository.
"""

from __future__ import annotations

import collections
import json
import os
import re
import subprocess
import sys

BUDGET_BYTES = 9 * 1024**3
ORDINARY_BYTES = int(7.5 * 1024**3)
LIMIT_BYTES = 10 * 1024**3
MAIN = "refs/heads/main"
SHA_SUFFIX = re.compile(r"^(?P<family>.+)-(?:[0-9a-f]{40}|[0-9a-f]{64})$")
RELEASE_FAMILIES = ("go-release-", "icu-static-")


def is_release(key: str) -> bool:
    return key.startswith(RELEASE_FAMILIES)


def gh(*args: str) -> str:
    return subprocess.run(["gh", *args], check=True, capture_output=True, text=True).stdout


def list_caches(repo: str) -> list[dict]:
    out = gh("api", "--paginate", f"repos/{repo}/actions/caches?per_page=100",
             "--jq", ".actions_caches[]")
    return [json.loads(line) for line in out.splitlines() if line.strip()]


def superseded(caches: list[dict]) -> list[dict]:
    """Return the caches on main that a newer cache of their family replaces."""
    newest: dict[str, dict] = {}
    for c in caches:
        if c["ref"] != MAIN:
            continue
        m = SHA_SUFFIX.match(c["key"])
        if not m:
            continue
        fam = m.group("family")
        if fam not in newest or c["created_at"] > newest[fam]["created_at"]:
            newest[fam] = c
    keep = {c["id"] for c in newest.values()}
    out = []
    for c in caches:
        m = SHA_SUFFIX.match(c["key"]) if c["ref"] == MAIN else None
        if m and c["id"] not in keep:
            out.append(c)
    return out


def off_main(caches: list[dict]) -> list[dict]:
    """Return every cache not on main: only main saves, so these are leftovers."""
    return [c for c in caches if c["ref"] != MAIN]


def family_of(key: str) -> str:
    key = re.sub(r"sha256:[0-9a-f]+", "sha256:*", key)
    key = re.sub(r"-[0-9a-f]{40}$", "-<sha>", key)
    key = re.sub(r"-[0-9a-f]{64}$", "-<hash>", key)
    return key


def report(caches: list[dict]) -> str:
    fams: dict[tuple[str, str], list[int]] = collections.defaultdict(list)
    for c in caches:
        scope = "main" if c["ref"] == MAIN else c["ref"]
        fams[(family_of(c["key"]), scope)].append(c["size_in_bytes"])
    lines = []
    for (fam, scope), sizes in sorted(fams.items(), key=lambda kv: -sum(kv[1])):
        lines.append(f"{sum(sizes) / 1024**2:9.0f} MiB  {len(sizes):3d}  {scope}  {fam}")
    total = sum(c["size_in_bytes"] for c in caches)
    lines.append(f"{total / 1024**3:9.2f} GiB total in {len(caches)} caches "
                 f"(budget {BUDGET_BYTES / 1024**3:.0f} GiB, limit {LIMIT_BYTES / 1024**3:.0f} GiB)")
    return "\n".join(lines)


def main_prune(repo: str) -> int:
    caches = list_caches(repo)
    doomed = superseded(caches) + off_main(caches)
    failed = 0
    for c in doomed:
        try:
            gh("api", "-X", "DELETE", f"repos/{repo}/actions/caches/{c['id']}")
            print(f"deleted {c['key']} ({c['size_in_bytes'] / 1024**2:.0f} MiB)")
        except subprocess.CalledProcessError as err:
            # A cache another run deleted first is gone already.
            if "404" in (err.stderr or ""):
                continue
            print(f"::error::could not delete {c['key']}: {err.stderr.strip()}", file=sys.stderr)
            failed += 1
    print(f"ci-cache-budget: {len(doomed)} superseded or off-main caches")
    return 1 if failed else 0


def over_budget(caches: list[dict]) -> list[str]:
    """The budget lines a cache listing breaks; empty when it keeps both."""
    total = sum(c["size_in_bytes"] for c in caches)
    ordinary = sum(c["size_in_bytes"] for c in caches if not is_release(c["key"]))
    out = []
    if ordinary > ORDINARY_BYTES:
        out.append(f"the ordinary caches hold {ordinary / 1024**3:.2f} GiB, past the {ORDINARY_BYTES / 1024**3:.1f} GiB "
                   f"that leaves the warm release caches room below GitHub's {LIMIT_BYTES / 1024**3:.0f} GiB")
    if total > BUDGET_BYTES:
        out.append(f"Actions caches hold {total / 1024**3:.2f} GiB, past the {BUDGET_BYTES / 1024**3:.0f} GiB budget; "
                   f"GitHub evicts past {LIMIT_BYTES / 1024**3:.0f} GiB")
    return out


def main_check(repo: str) -> int:
    caches = list_caches(repo)
    print(report(caches))
    ordinary = sum(c["size_in_bytes"] for c in caches if not is_release(c["key"]))
    print(f"{ordinary / 1024**3:9.2f} GiB ordinary, {(sum(c['size_in_bytes'] for c in caches) - ordinary) / 1024**3:.2f} GiB warm release caches")
    problems = over_budget(caches)
    for problem in problems:
        print(f"::error::{problem}. Find the family that grew above.")
    return 1 if problems else 0


def self_test() -> int:
    def c(i, key, ref=MAIN, created="2026-11-22T10:00:00Z", size=10, accessed=None):
        return {"id": i, "key": key, "ref": ref, "created_at": created, "size_in_bytes": size,
                "last_accessed_at": accessed or created}

    sha = lambda n: f"{n:040x}"
    caches = [
        c(1, f"go-build-Linux-pr-trimpath-{sha(1)}", created="2026-11-22T09:00:00Z"),
        c(2, f"go-build-Linux-pr-trimpath-{sha(2)}", created="2026-11-22T11:00:00Z"),
        c(3, f"go-build-Linux-pr-trimpath-{sha(3)}", created="2026-11-22T10:00:00Z"),
        c(4, f"go-build-Linux-race-trimpath-{sha(4)}"),
        c(5, f"go-build-Linux-pr-trimpath-{sha(5)}", ref="refs/pull/7/merge", created="2026-11-22T08:00:00Z"),
        c(6, "go-mod-Linux-" + "a" * 64),
        c(7, "buildkit-blob-1-sha256:" + "b" * 64),
    ]
    got = sorted(x["id"] for x in superseded(caches))
    failures = []
    if got != [1, 3]:
        failures.append(f"superseded should be [1, 3] (older pr copies on main), got {got}")
    # A family keyed by a 64-digit content hash keeps its newest copy too;
    # buildkit blobs ("...sha256:<hash>") are not a family of copies.
    hashed = caches + [c(11, "vite-plus-Linux-x64-pnpm-" + "e" * 64, created="2026-11-22T12:00:00Z"),
                       c(12, "vite-plus-Linux-x64-pnpm-" + "f" * 64, created="2026-11-22T13:00:00Z"),
                       c(13, "buildkit-blob-1-sha256:" + "9" * 64)]
    got = sorted(x["id"] for x in superseded(hashed))
    if got != [1, 3, 11]:
        failures.append(f"superseded should be [1, 3, 11] (the older vite-plus copy too), got {got}")
    branch = [
        c(8, "playwright-Linux-" + "c" * 64, ref="refs/pull/9/merge", accessed="2026-11-21T12:00:00Z"),
        c(9, "playwright-Linux-" + "c" * 64, ref="refs/heads/x", accessed="2026-11-23T11:00:00Z"),
        c(10, "go-mod-Linux-" + "d" * 64, accessed="2026-11-01T00:00:00Z"),
    ]
    gone = sorted(x["id"] for x in off_main(caches + branch))
    if gone != [5, 8, 9]:
        failures.append(f"every cache off main should go, read recently or not: [5, 8, 9], got {gone}")
    gib = 1024**3
    ordinary_full = [c(20, "go-build-x-" + sha(20), size=int(7.6 * gib))]
    if not any("ordinary" in x for x in over_budget(ordinary_full)):
        failures.append("ordinary caches past 7.5 GiB must fail")
    with_release = [c(21, "go-build-x-" + sha(21), size=int(7.0 * gib)),
                    c(22, "go-release-Linux-X64-" + sha(22), size=int(1.8 * gib))]
    if over_budget(with_release):
        failures.append(f"7.0 GiB ordinary plus 1.8 GiB warm release caches is within budget: {over_budget(with_release)}")
    total_full = with_release + [c(23, "go-release-macOS-ARM64-" + sha(23), size=int(0.5 * gib))]
    if not any("past the 9 GiB budget" in x for x in over_budget(total_full)):
        failures.append("a total past 9 GiB must fail")
    if "-<sha>" not in report(caches):
        failures.append("report should fold sha suffixes into one family")
    if failures:
        print("ci-cache-budget self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("ci-cache-budget self-test: OK")
    return 0


if __name__ == "__main__":
    args = sys.argv[1:]
    if args == ["--self-test"]:
        sys.exit(self_test())
    repo = os.environ.get("GH_REPO", "neokapi/neokapi")
    if args == ["prune"]:
        sys.exit(main_prune(repo))
    if args == ["check"]:
        sys.exit(main_check(repo))
    if args == ["report"]:
        print(report(list_caches(repo)))
        sys.exit(0)
    print(__doc__)
    sys.exit(2)
