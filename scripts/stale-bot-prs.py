#!/usr/bin/env python3
"""Report pull requests a workflow opened or pushed to that nobody is moving.

    scripts/stale-bot-prs.py              check the repository (GH_REPO)
    scripts/stale-bot-prs.py --self-test  prove the rules on fixtures

A workflow that pushes with GITHUB_TOKEN leaves the pull request's workflow
runs waiting for a person to approve them, and main requires `CI passed`, so
such a pull request waits until someone looks. The docs install page waited
sixteen days that way. The daily Bot PR Watch workflow runs this script, and a
failed scheduled run files an issue through scheduled-failure-issue.yml.

It fails on an open pull request that either:

  * a workflow opened (author github-actions, or a head branch under bot/)
    and that has been open more than MAX_OPEN_DAYS; or
  * has a workflow run on its head commit that has waited for approval
    (conclusion action_required) more than MAX_HELD_HOURS. A tidy or relock
    commit pushed to a Dependabot branch lands here.

Exit status: 0 nothing stale, 1 something is, 2 the listing failed.
"""

from __future__ import annotations

import datetime
import json
import os
import subprocess
import sys

MAX_OPEN_DAYS = 3
MAX_HELD_HOURS = 24
BOT_AUTHORS = {"app/github-actions", "github-actions"}


def gh(*args: str) -> str:
    return subprocess.run(["gh", *args], check=True, capture_output=True, text=True).stdout


def ts(s: str) -> datetime.datetime:
    return datetime.datetime.fromisoformat(s.replace("Z", "+00:00"))


def workflow_opened(pr: dict) -> bool:
    return pr["author"] in BOT_AUTHORS or pr["head"].startswith("bot/")


def problems(prs: list[dict], runs: dict[int, list[dict]], now: datetime.datetime) -> list[str]:
    out = []
    for pr in prs:
        age = now - ts(pr["created"])
        if workflow_opened(pr) and age > datetime.timedelta(days=MAX_OPEN_DAYS):
            out.append(f"#{pr['number']} ({pr['head']}) was opened by a workflow {age.days} days ago and is still open")
        held = [r for r in runs.get(pr["number"], [])
                if r["conclusion"] == "action_required"
                and now - ts(r["created"]) > datetime.timedelta(hours=MAX_HELD_HOURS)]
        if held:
            names = ", ".join(sorted({r["name"] for r in held}))
            out.append(f"#{pr['number']} ({pr['head']}) has workflow runs waiting for approval on its head commit: {names}")
    return out


def fetch(repo: str) -> tuple[list[dict], dict[int, list[dict]]]:
    raw = json.loads(gh("pr", "list", "--repo", repo, "--state", "open", "--limit", "200",
                        "--json", "number,headRefName,headRefOid,author,createdAt"))
    prs, runs = [], {}
    for p in raw:
        login = p["author"]["login"]
        prs.append({"number": p["number"], "head": p["headRefName"], "author": login, "created": p["createdAt"]})
        listed = json.loads(gh("api", f"repos/{repo}/actions/runs?head_sha={p['headRefOid']}&per_page=100"))
        runs[p["number"]] = [{"name": r["name"], "conclusion": r.get("conclusion") or "", "created": r["created_at"]}
                             for r in listed.get("workflow_runs", [])]
    return prs, runs


def self_test() -> int:
    now = datetime.datetime(2026, 11, 22, 12, tzinfo=datetime.timezone.utc)
    prs = [
        {"number": 1, "head": "bot/dogfood-sync", "author": "app/github-actions", "created": "2026-11-15T12:00:00Z"},
        {"number": 2, "head": "bot/release-downloads-kapi", "author": "app/github-actions", "created": "2026-11-21T12:00:00Z"},
        {"number": 3, "head": "dependabot/go_modules/x", "author": "app/dependabot", "created": "2026-11-20T12:00:00Z"},
        {"number": 4, "head": "feature", "author": "someone", "created": "2026-10-01T12:00:00Z"},
    ]
    runs = {
        2: [{"name": "CI", "conclusion": "action_required", "created": "2026-11-22T06:00:00Z"}],
        3: [{"name": "CI", "conclusion": "action_required", "created": "2026-11-20T13:00:00Z"},
            {"name": "E2E", "conclusion": "success", "created": "2026-11-20T13:00:00Z"}],
    }
    got = problems(prs, runs, now)
    failures = []
    if not any(p.startswith("#1 ") and "opened by a workflow" in p for p in got):
        failures.append("a workflow-opened PR open 7 days must be reported")
    if any(p.startswith("#2 ") for p in got):
        failures.append("a 1-day-old bot PR held for 6 hours must not be reported yet")
    if not any(p.startswith("#3 ") and "waiting for approval" in p and "E2E" not in p for p in got):
        failures.append("a Dependabot PR whose CI waited 2 days for approval must be reported, naming only the held run")
    if any(p.startswith("#4 ") for p in got):
        failures.append("an old PR a person opened is not this check's business")
    if failures:
        print("stale-bot-prs self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("stale-bot-prs self-test: OK")
    return 0


def main() -> int:
    repo = os.environ.get("GH_REPO", "neokapi/neokapi")
    try:
        prs, runs = fetch(repo)
    except subprocess.CalledProcessError as err:
        print(f"stale-bot-prs: listing failed: {err.stderr.strip()}", file=sys.stderr)
        return 2
    found = problems(prs, runs, datetime.datetime.now(datetime.timezone.utc))
    if found:
        print("::error::Pull requests a workflow opened or pushed to are waiting on nobody:")
        for f in found:
            print("  " + f)
        print("Approve the held runs (Checks tab, 'Approve workflows to run'), merge or close the pull request. "
              "docs/internals/bot-pushes.md says why this happens and how the bot App prevents it.")
        return 1
    print(f"stale-bot-prs: OK ({len(prs)} open pull requests)")
    return 0


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-test"]:
        sys.exit(self_test())
    if sys.argv[1:]:
        print(__doc__)
        sys.exit(2)
    sys.exit(main())
