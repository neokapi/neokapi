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

prune deletes the caches nobody will read:

  * on refs/heads/main, every cache of a family keyed by a commit sha or a
    content hash (40 or 64 hex digits) except the newest;
  * every cache on any other ref. Only main saves caches
    (scripts/check-ci-caches.py), so one elsewhere is a leftover;
  * every buildkit layer blob (`buildkit-blob-*`, written by the image
    builds' gha cache) that no build has read for STALE_DAYS (3) days, and
    every buildkit index (`index-<scope>#<n>`) unread as long that is not the
    newest of its scope. Buildkit reads only the newest index of a scope. A
    blob an index still lists is a plain cache miss once deleted: the build
    rebuilds the layer and writes the blob again (checked on main,
    2026-11-28, with the keycloak image). The REST listing cannot show which
    blobs an index lists, so an index is judged by its own reads.

A listing prune cannot read deletes nothing: one whose pages hold a
different number of caches than its total_count, or a cache without an id,
a key, a ref or a time prune can parse. It needs a token with `actions: write` and
runs from main.

check applies two rules and fails when either breaks.

  * Steady state. After what prune removes, the ordinary caches must stay
    within ORDINARY_BYTES (7.5 GiB) and the total within BUDGET_BYTES
    (9 GiB). These are the caches that stay between saves. The warm release
    caches (go-release-*, icu-static-*, about 1.8 GiB) are read only at
    release time, so GitHub's least-recently-used eviction past 10 GiB takes
    them first. The ordinary budget leaves them room.
  * One save in flight. The steady total plus the newest copy of the largest
    family must stay within LIMIT_BYTES (GitHub's 10 GiB). A save adds a new
    copy before prune deletes the old one, and GitHub evicts on the real
    total in that window. The warm release caches were lost that way on
    2026-11-23. A steady state that passes the first rule can still leave no
    room for one save, and this rule catches that. It allows for one save
    in flight only. A push on main saves several families (the race, pr and
    lint warm jobs run side by side), and their new copies can overlap
    before each job prunes. Two large families in flight together can pass
    10 GiB while this rule holds.

The scheduled cache-budget workflow runs check, and a failure there opens an
issue through scheduled-failure-issue.yml.

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
ORDINARY_BYTES = int(7.5 * 1024**3)
LIMIT_BYTES = 10 * 1024**3
MAIN = "refs/heads/main"
SHA_SUFFIX = re.compile(r"^(?P<family>.+)-(?:[0-9a-f]{40}|[0-9a-f]{64})$")
RELEASE_FAMILIES = ("go-release-", "icu-static-")
STALE_DAYS = 3
BUILDKIT_BLOB = "buildkit-blob-"
BUILDKIT_INDEX = re.compile(r"^index-(?P<scope>.+)#(?P<n>[0-9]+)$")


class Unreadable(Exception):
    """A cache listing prune cannot judge."""


def when(stamp: str) -> datetime.datetime:
    return datetime.datetime.fromisoformat(stamp.replace("Z", "+00:00"))


def readable(caches: list[dict]) -> None:
    """Raise Unreadable unless every cache has what prune judges it by."""
    for c in caches:
        try:
            if not (c["id"] and c["key"] and c["ref"]) or int(c["size_in_bytes"]) < 0:
                raise ValueError("empty field")
            when(c["created_at"])
            when(c["last_accessed_at"])
        except (KeyError, TypeError, ValueError) as err:
            raise Unreadable(f"cache {c.get('key') or c.get('id') or c!r}: {err}") from err


def stale_buildkit(caches: list[dict], now: datetime.datetime) -> list[dict]:
    """The buildkit blobs no build has read for STALE_DAYS days, and the
    indexes unread as long that a newer index of their scope replaces."""
    cutoff = now - datetime.timedelta(days=STALE_DAYS)
    newest: dict[str, int] = {}
    for c in caches:
        m = BUILDKIT_INDEX.match(c["key"]) if c["ref"] == MAIN else None
        if m:
            newest[m.group("scope")] = max(newest.get(m.group("scope"), -1), int(m.group("n")))
    out = []
    for c in caches:
        if c["ref"] != MAIN or when(c["last_accessed_at"]) >= cutoff:
            continue
        m = BUILDKIT_INDEX.match(c["key"])
        if c["key"].startswith(BUILDKIT_BLOB) or (m and int(m.group("n")) < newest[m.group("scope")]):
            out.append(c)
    return out


def fold(family: str) -> str:
    """One family across config hashes: a 64-digit hash inside the key (a
    golangci-lint config, say) changes with the config, and the copy under the
    old hash would otherwise never be superseded."""
    return re.sub(r"-[0-9a-f]{64}(?=-|$)", "-<hash>", family)


def is_release(key: str) -> bool:
    return key.startswith(RELEASE_FAMILIES)


def gh(*args: str) -> str:
    return subprocess.run(["gh", *args], check=True, capture_output=True, text=True).stdout


def caches_from_pages(pages: list[dict]) -> list[dict]:
    """The caches on every page of a listing. Raises Unreadable when the
    pages disagree on total_count or hold a different number of caches: a
    page lost while paging would look like caches that are gone."""
    totals = {page.get("total_count") for page in pages}
    caches = [c for page in pages for c in page.get("actions_caches") or []]
    if len(totals) != 1 or not isinstance(next(iter(totals)), int):
        raise Unreadable(f"the listing's pages give total_count {sorted(map(str, totals))}")
    total = next(iter(totals))
    if len(caches) != total:
        raise Unreadable(f"the listing holds {len(caches)} caches, and total_count says {total}")
    return caches


def list_caches(repo: str) -> list[dict]:
    out = gh("api", "--paginate", "--slurp", f"repos/{repo}/actions/caches?per_page=100")
    return caches_from_pages(json.loads(out))


def superseded(caches: list[dict]) -> list[dict]:
    """Return the caches on main that a newer cache of their family replaces."""
    newest: dict[str, dict] = {}
    for c in caches:
        if c["ref"] != MAIN:
            continue
        m = SHA_SUFFIX.match(c["key"])
        if not m:
            continue
        fam = fold(m.group("family"))
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


def doomed_by_prune(caches: list[dict], now: datetime.datetime) -> list[dict]:
    """What prune deletes from a listing; raises Unreadable, and then
    nothing is deleted."""
    readable(caches)
    seen, out = set(), []
    for c in superseded(caches) + off_main(caches) + stale_buildkit(caches, now):
        if c["id"] not in seen:
            seen.add(c["id"])
            out.append(c)
    return out


def main_prune(repo: str) -> int:
    try:
        caches = list_caches(repo)
        doomed = doomed_by_prune(caches, datetime.datetime.now(datetime.timezone.utc))
    except Unreadable as err:
        # A save that lands while the listing is paged changes total_count, so
        # an unreadable listing is common enough on main that failing the job
        # would be noise. Nothing is deleted; the scheduled check, which fails
        # on an unreadable listing, reports one that stays unreadable.
        print(f"::warning::ci-cache-budget: cannot read the cache listing, so nothing is deleted: {err}")
        return 0
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
    print(f"ci-cache-budget: {len(doomed)} superseded, off-main or unread buildkit caches")
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


def after_prune(caches: list[dict], now: datetime.datetime | None = None) -> list[dict]:
    """The caches prune leaves: each family's newest copy on main, the
    buildkit caches a build read lately, nothing elsewhere. A new copy saved a
    moment before the next prune is a transient every save makes; check
    judges the state prune converges on."""
    now = now or datetime.datetime.now(datetime.timezone.utc)
    doomed = {c["id"] for c in superseded(caches) + off_main(caches) + stale_buildkit(caches, now)}
    return [c for c in caches if c["id"] not in doomed]


def in_flight(steady: list[dict]) -> list[str]:
    """The line broken when one save of the largest family lands before the
    prune: its new copy and the old one both count until prune runs."""
    newest = {}
    for c in steady:
        m = SHA_SUFFIX.match(c["key"])
        if m:
            newest[fold(m.group("family"))] = c
    if not newest:
        return []
    largest = max(newest.values(), key=lambda c: c["size_in_bytes"])
    total = sum(c["size_in_bytes"] for c in steady) + largest["size_in_bytes"]
    if total <= LIMIT_BYTES:
        return []
    return [f"the caches prune keeps plus one more copy of {family_of(largest['key'])} "
            f"({largest['size_in_bytes'] / 1024**3:.2f} GiB) hold {total / 1024**3:.2f} GiB, past GitHub's "
            f"{LIMIT_BYTES / 1024**3:.0f} GiB; a save of that family before the prune would evict"]


def check_problems(caches: list[dict]) -> list[str]:
    """Both rules of check, on a raw listing."""
    steady = after_prune(caches)
    return over_budget(steady) + in_flight(steady)


def main_check(repo: str) -> int:
    try:
        caches = list_caches(repo)
    except Unreadable as err:
        print(f"::error::ci-cache-budget: cannot read the cache listing: {err}")
        return 1
    print(report(caches))
    steady = after_prune(caches)
    ordinary = sum(c["size_in_bytes"] for c in steady if not is_release(c["key"]))
    print(f"after prune: {ordinary / 1024**3:.2f} GiB ordinary, "
          f"{(sum(c['size_in_bytes'] for c in steady) - ordinary) / 1024**3:.2f} GiB warm release caches "
          f"({len(caches) - len(steady)} copies prune removes)")
    problems = check_problems(caches)
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
    lint = caches + [c(14, "golangci-lint-Linux-lint-framework-" + "1" * 64 + "-" + sha(14), created="2026-11-22T09:00:00Z"),
                     c(15, "golangci-lint-Linux-lint-framework-" + "2" * 64 + "-" + sha(15), created="2026-11-22T12:00:00Z")]
    got = sorted(x["id"] for x in superseded(lint))
    if got != [1, 3, 14]:
        failures.append(f"a family under an old config hash is superseded by the new one: want [1, 3, 14], got {got}")
    gone = sorted(x["id"] for x in off_main(caches + branch))
    if gone != [5, 8, 9]:
        failures.append(f"every cache off main should go, read recently or not: [5, 8, 9], got {gone}")
    gib = 1024**3
    # Two copies of one family a moment after a save: only the newest counts.
    transient = [c(30, "go-build-race-" + sha(30), size=int(4 * gib), created="2026-11-22T09:00:00Z"),
                 c(31, "go-build-race-" + sha(31), size=int(4 * gib), created="2026-11-22T10:00:00Z"),
                 c(32, "go-build-race-" + sha(32), size=1, ref="refs/pull/1/merge")]
    steady = after_prune(transient)
    if [x["id"] for x in steady] != [31] or over_budget(steady):
        failures.append(f"after prune only the newest copy counts: {[x['id'] for x in steady]}, {over_budget(steady)}")
    if not over_budget(transient):
        failures.append("the raw listing of two 4 GiB copies is over budget; only after_prune excuses it")
    # 7.4 GiB ordinary at rest keeps the steady budget, but a second 2 GiB
    # race copy in flight passes 10 GiB: the window the release caches were
    # evicted in.
    tight = [c(40, "go-build-Linux-race-trimpath-" + sha(40), size=int(2.0 * gib)),
             c(41, "go-build-Linux-pr-trimpath-" + sha(41), size=int(1.9 * gib)),
             c(47, "go-build-Linux-lint-model-trimpath-" + sha(47), size=int(1.75 * gib)),
             c(48, "go-mod-Linux-" + "7" * 64, size=int(1.75 * gib)),
             c(42, "go-release-Linux-X64-" + sha(42), size=int(1.5 * gib))]
    if over_budget(after_prune(tight)):
        failures.append(f"7.4 GiB ordinary and 8.9 GiB total keep the steady budget: {over_budget(after_prune(tight))}")
    if not any("one more copy of go-build-Linux-race-trimpath-<sha>" in x for x in check_problems(tight)):
        failures.append(f"8.9 GiB plus a 2 GiB race save in flight passes 10 GiB and must fail: {check_problems(tight)}")
    roomy = [c(43, "go-build-Linux-race-trimpath-" + sha(43), size=int(2.0 * gib)),
             c(44, "go-build-Linux-pr-trimpath-" + sha(44), size=int(2.0 * gib)),
             c(45, "go-release-Linux-X64-" + sha(45), size=int(1.8 * gib))]
    if check_problems(roomy):
        failures.append(f"5.8 GiB plus a 2 GiB save in flight is within 10 GiB: {check_problems(roomy)}")
    # The in-flight rule counts the steady state, not the older copy prune removes.
    if check_problems(roomy + [c(46, "go-build-Linux-pr-trimpath-" + sha(46), size=int(2.0 * gib),
                                 created="2026-11-21T10:00:00Z")]):
        failures.append("an older copy prune removes is not counted twice")
    # Buildkit blobs and indexes, judged on 2026-11-28 at noon.
    now = when("2026-11-28T12:00:00Z")
    blobs = [c(50, "buildkit-blob-1-sha256:" + "5" * 64, accessed="2026-11-27T12:00:00Z"),
             c(51, "buildkit-blob-1-sha256:" + "6" * 64, accessed="2026-11-24T12:00:00Z"),
             c(52, "index-bowrain-web-1-f921bd05#3", accessed="2026-11-24T12:00:00Z"),
             c(53, "index-bowrain-web-1-f921bd05#4", accessed="2026-11-24T12:00:00Z"),
             c(54, "index-bowrain-web-1-f921bd05#2", accessed="2026-11-27T12:00:00Z"),
             c(55, "buildkit-blob-1-sha256:" + "7" * 64, ref="refs/pull/1/merge", accessed="2026-11-28T11:00:00Z")]
    gone = sorted(x["id"] for x in doomed_by_prune(blobs, now))
    if gone != [51, 52, 55]:
        failures.append("a blob read yesterday stays, one read four days ago goes, an index goes only when a newer "
                        f"index of its scope replaces it and nobody read it for three days: want [51, 52, 55], got {gone}")
    for broken in ({"id": 60, "key": "buildkit-blob-1-sha256:" + "8" * 64, "ref": MAIN, "size_in_bytes": 1,
                    "created_at": "2026-11-01T00:00:00Z"},
                   {"id": 61, "key": "buildkit-blob-1-sha256:" + "9" * 64, "ref": MAIN, "size_in_bytes": 1,
                    "created_at": "2026-11-01T00:00:00Z", "last_accessed_at": "yesterday"}):
        try:
            doomed_by_prune(blobs + caches + [broken], now)
            failures.append(f"a listing with {broken} must delete nothing")
        except Unreadable:
            pass
    page = lambda total, *ids: {"total_count": total, "actions_caches": [c(i, f"k{i}") for i in ids]}
    if [x["id"] for x in caches_from_pages([page(3, 1, 2), page(3, 3)])] != [1, 2, 3]:
        failures.append("two pages that agree with total_count list every cache")
    for name, pages in (("a page short of total_count", [page(3, 1, 2)]),
                        ("pages that disagree on total_count", [page(3, 1, 2), page(4, 3)]),
                        ("a page without total_count", [{"actions_caches": []}])):
        try:
            caches_from_pages(pages)
            failures.append(f"{name} must be unreadable, so prune deletes nothing")
        except Unreadable:
            pass
    if [x["id"] for x in after_prune(blobs[:2], now)] != [50]:
        failures.append("check judges the listing after prune drops the unread blob")
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
