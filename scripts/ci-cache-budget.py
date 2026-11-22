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

  * on refs/heads/main, every cache of a sha-keyed family except the newest;
  * on any other ref (a branch's manual runs, a pull request), every cache
    not read for STALE_HOURS. Only runs on that same ref can restore it.

It needs a token with `actions: write` and runs from main.

check fails when the total size passes BUDGET_BYTES (9 GiB), which leaves
1 GiB before eviction starts. The scheduled cache-budget workflow runs it, and
a failure there opens an issue through scheduled-failure-issue.yml.

The listing comes from `gh api repos/{repo}/actions/caches`; GH_REPO names the
repository.
"""

from __future__ import annotations

import collections
import datetime
import json
import os
import re
import subprocess
import sys

BUDGET_BYTES = 9 * 1024**3
STALE_HOURS = 24
LIMIT_BYTES = 10 * 1024**3
MAIN = "refs/heads/main"
SHA_SUFFIX = re.compile(r"^(?P<family>.+)-[0-9a-f]{40}$")


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


def stale_elsewhere(caches: list[dict], now: datetime.datetime) -> list[dict]:
    """Return the caches off main that nothing read for STALE_HOURS."""
    cutoff = now - datetime.timedelta(hours=STALE_HOURS)
    out = []
    for c in caches:
        if c["ref"] == MAIN:
            continue
        seen = datetime.datetime.fromisoformat(c.get("last_accessed_at") or c["created_at"])
        if seen < cutoff:
            out.append(c)
    return out


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
    doomed = superseded(caches) + stale_elsewhere(caches, datetime.datetime.now(datetime.timezone.utc))
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
    print(f"ci-cache-budget: {len(doomed)} superseded or stale caches")
    return 1 if failed else 0


def main_check(repo: str) -> int:
    caches = list_caches(repo)
    print(report(caches))
    total = sum(c["size_in_bytes"] for c in caches)
    if total > BUDGET_BYTES:
        print(f"::error::Actions caches hold {total / 1024**3:.2f} GiB, past the "
              f"{BUDGET_BYTES / 1024**3:.0f} GiB budget; GitHub evicts past "
              f"{LIMIT_BYTES / 1024**3:.0f} GiB. Find the family that grew above.")
        return 1
    return 0


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
    now = datetime.datetime(2026, 11, 23, 12, tzinfo=datetime.timezone.utc)
    branch = [
        c(8, "playwright-Linux-" + "c" * 64, ref="refs/pull/9/merge", accessed="2026-11-21T12:00:00Z"),
        c(9, "playwright-Linux-" + "c" * 64, ref="refs/heads/x", accessed="2026-11-23T11:00:00Z"),
        c(10, "go-mod-Linux-" + "d" * 64, accessed="2026-11-01T00:00:00Z"),
    ]
    stale = sorted(x["id"] for x in stale_elsewhere(caches + branch, now))
    if stale != [5, 8]:
        failures.append(f"stale off main should be [5, 8] (not read for a day, not on main), got {stale}")
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
