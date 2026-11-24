#!/usr/bin/env python3
"""Report the gardening that is due: the upkeep a person runs locally.

    scripts/gardening-due.py                 print what is due (exit 0)
    scripts/gardening-due.py --json          the same, as JSON
    scripts/gardening-due.py --sync-issues   keep one `gardening` issue per item
    scripts/gardening-due.py --release-gate <id> [--report-only]
                                             refuse a release when <id> is older
                                             than its release_gate_days
    scripts/gardening-due.py --self-test     prove the rules on fixtures

Reads docs/internals/gardening.yaml. Each item says where its "last done"
date is read from; the format-ops item reads every ritual's cadence_days and
last_run from docs/internals/format-ops-ledger.json, with the rule
.skills/format-ops/scripts/due.mjs applies to cadence. A missing file, field
or date makes an item due with the reason, never skipped. No network is
needed, except by --sync-issues, which calls gh.

--sync-issues, for the Gardening workflow: an item that is due gets an issue
titled "Gardening due: <id>", opened with what the item is for, the command
and how overdue it is, or a short comment with the new count when one is
open. An item that is done has its open issue closed, with a comment saying
what was read; an open issue for an item no longer listed is closed too.
Issues are matched by a hidden marker in the body, and a run comments at
most once an ISO week per issue. A marker counts only when the workflow's own
identity wrote the issue or comment (asked of the API at run time), so a
marker a stranger writes is ignored. A person with write access can still
edit an issue body the workflow wrote; that is accepted. --no-close opens and
comments only. GARDENER names a login to
assign new issues to; empty assigns nobody.

This is a report: it never fails on what is due. --release-gate is the one
exception: release.yml runs it on a kapi release, and it exits 1 when the
item's date is more than its release_gate_days old or cannot be read.
--report-only (a build-only dispatch) prints the same and exits 0.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import re
import subprocess
import sys

import yaml

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LIST = os.path.join("docs", "internals", "gardening.yaml")
LABEL = "gardening"
TITLE = "Gardening due: {}"


def parse_date(text: object) -> dt.date | None:
    if isinstance(text, dt.datetime):
        return text.date()
    if isinstance(text, dt.date):
        return text
    if not isinstance(text, str):
        return None
    try:
        return dt.date.fromisoformat(text[:10])
    except ValueError:
        return None


def judge(item_id: str, last: dt.date | None, cadence: int, today: dt.date, read: str) -> dict:
    """The verdict for one dated item."""
    if last is None:
        return {"due": True, "reason": f"no date: {read}", "overdue_days": None, "read": read}
    age = (today - last).days
    if age < 0:
        return {"due": True, "reason": f"the date is in the future ({last}), so the record is wrong: {read}",
                "overdue_days": None, "read": read}
    if age > cadence:
        return {"due": True, "reason": f"last done {last}, {age} days ago; the cadence is {cadence} days",
                "overdue_days": age - cadence, "read": read}
    return {"due": False, "reason": f"last done {last}, {age} days ago; the cadence is {cadence} days",
            "overdue_days": None, "read": read}


def read_json(path: str) -> tuple[object | None, str | None]:
    try:
        with open(path) as fh:
            return json.load(fh), None
    except FileNotFoundError:
        return None, "the file is missing"
    except (OSError, ValueError) as err:
        return None, f"the file cannot be read ({err})"


def check_item(root: str, item: dict, today: dt.date) -> dict:
    src = item.get("source") or {}
    kind = src.get("kind")
    rel = src.get("file", "")
    path = os.path.join(root, rel)
    if kind == "json-field":
        field = src.get("field", "")
        doc, err = read_json(path)
        if err:
            return judge(item["id"], None, 0, today, f"{rel}: {err}")
        value = doc.get(field) if isinstance(doc, dict) else None
        if value is None:
            return judge(item["id"], None, 0, today, f"{rel} has no `{field}`")
        return judge(item["id"], parse_date(value), int(item["cadence_days"]), today, f"{rel} `{field}` = {value}")
    if kind == "oldest-date":
        try:
            with open(path) as fh:
                text = fh.read()
        except FileNotFoundError:
            return judge(item["id"], None, 0, today, f"{rel}: the file is missing")
        dates = [d for d in (parse_date(m) for m in re.findall(src["pattern"], text)) if d]
        if not dates:
            return judge(item["id"], None, 0, today, f"{rel} has no date matching {src['pattern']}")
        oldest = min(dates)
        return judge(item["id"], oldest, int(item["cadence_days"]), today,
                     f"{rel}: oldest of {len(dates)} dates is {oldest}")
    if kind == "format-ops":
        return check_format_ops(root, rel, today)
    return judge(item["id"], None, 0, today, f"unknown source kind {kind!r}")


def check_format_ops(root: str, rel: str, today: dt.date) -> dict:
    """Cadence due-ness of each ritual, by the rule due.mjs applies.

    cadence_days 0 is never due by time; a ci_owned ritual is watched by CI,
    not run by a person; a ritual blocked on an issue stays due and says so.
    """
    doc, err = read_json(os.path.join(root, rel))
    rituals = doc.get("rituals") if isinstance(doc, dict) else None
    if err or not isinstance(rituals, dict):
        return judge("format-ops", None, 0, today, f"{rel}: {err or 'no rituals'}")
    due, worst = [], None
    for name, r in rituals.items():
        cadence = r.get("cadence_days") or 0
        if r.get("ci_owned") or cadence == 0:
            continue
        last = parse_date(r.get("last_run"))
        if r.get("last_run") is None:
            line, over = f"{name}: never run (cadence {cadence} days)", None
        elif last is None or (today - last).days < 0:
            line, over = f"{name}: last_run {r.get('last_run')!r} is not a past date", None
        elif (today - last).days > cadence:
            age = (today - last).days
            line, over = f"{name}: {age} days since {last}; the cadence is {cadence} days", age - cadence
        else:
            continue
        if r.get("blocked_on"):
            line += f" (blocked on {r['blocked_on']})"
        due.append(line)
        if over is not None:
            worst = over if worst is None else max(worst, over)
    read = f"{rel}: {len(rituals)} rituals"
    if not due:
        return {"due": False, "reason": "no ritual is past its cadence", "overdue_days": None, "read": read}
    return {"due": True, "reason": f"{len(due)} ritual(s) due: " + "; ".join(due), "overdue_days": worst,
            "read": read, "details": due}


def load_items(root: str) -> list[dict]:
    with open(os.path.join(root, LIST)) as fh:
        return (yaml.safe_load(fh) or {}).get("items") or []


def report(root: str, today: dt.date) -> list[dict]:
    out = []
    for item in load_items(root):
        verdict = check_item(root, item, today)
        out.append({"id": item["id"], "purpose": item.get("purpose", ""), "run": item.get("run", ""),
                    "needs": item.get("needs", ""), **verdict})
    return out


def overdue_text(r: dict) -> str:
    if r["overdue_days"] is None:
        return "overdue (no usable date)" if "never" not in r["reason"] else "overdue (never done)"
    return f"{r['overdue_days']} days overdue"


def print_report(results: list[dict], today: dt.date) -> None:
    print(f"gardening due report, {today}")
    due = [r for r in results if r["due"]]
    print(f"\nDUE ({len(due)})")
    for r in due:
        print(f"  {r['id']}: {overdue_text(r)}")
        print(f"    {r['purpose']}")
        print(f"    why: {r['reason']}")
        print(f"    run: {r['run']}")
        if r["needs"]:
            print(f"    needs: {r['needs']}")
    print("\nOK")
    for r in results:
        if not r["due"]:
            print(f"  {r['id']}: {r['reason']}")
    print("\nexit 0: this is a report, not a gate.")


# ── issues ───────────────────────────────────────────────────────────────
#
# An issue is matched by the hidden marker in its body, which carries the
# item's id, within the `gardening` label; the title is for people and may be
# edited. A reminder comment carries the ISO week it was written in, so a run
# comments at most once a week per issue. An open issue whose marker names an
# item no longer in the list is closed by the scheduled run.

ITEM_MARK = re.compile(r"<!-- gardening-item: ([A-Za-z0-9_.-]+) -->")
WEEK_MARK = re.compile(r"<!-- gardening-week: (\d{4}-W\d{2}) -->")


def iso_week(day: dt.date) -> str:
    year, week, _ = day.isocalendar()
    return f"{year}-W{week:02d}"


def item_mark(item_id: str) -> str:
    return f"<!-- gardening-item: {item_id} -->"


def week_mark(week: str) -> str:
    return f"<!-- gardening-week: {week} -->"


def gh(*args: str, check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(["gh", *args], capture_output=True, text=True, check=check)


def issue_body(r: dict, week: str, origin: str) -> str:
    lines = [r["purpose"], "", f"**Run:** {r['run']}", "", f"**Status:** {overdue_text(r)}. {r['reason']}."]
    for d in r.get("details") or []:
        lines.append(f"- {d}")
    if r["needs"]:
        lines += ["", f"**Needs:** {r['needs']}."]
    lines += ["", "The work is local; this issue is a reminder. The weekly Gardening workflow comments here while "
              "the item is due and closes the issue when it reads the item as done "
              "(docs/internals/gardening.yaml)." + origin, "", item_mark(r["id"]), week_mark(week)]
    return "\n".join(lines)


def login_key(login: str) -> str:
    """One spelling of a login: gh writes a bot as app/<name> or <name>[bot]."""
    login = (login or "").strip().lower()
    login = login.removeprefix("app/")
    return login.removesuffix("[bot]")


def plan(results: list[dict], issues: list[dict], week: str, close: bool, origin: str = "",
         trusted: str = "") -> list[tuple]:
    """The actions that bring the open `gardening` issues in line with results.

    issues: the open issues with the label, each {number, title, author, body,
    comments: [{author, body}, ...]}. trusted: the workflow's own login; a
    marker in an issue or comment by anyone else is ignored. Returns tuples: ("mark", number, new_body),
    ("open", id, title, body), ("comment", number, body), ("close", number,
    comment), ("note", text).
    """
    actions: list[tuple] = []
    listed = {r["id"] for r in results}
    by_id: dict[str, dict] = {}
    me = login_key(trusted)

    def ours(author: str) -> bool:
        return bool(me) and login_key(author) == me

    for issue in issues:
        if not ours(issue.get("author", "")):
            continue
        m = ITEM_MARK.search(issue.get("body") or "")
        if m:
            by_id.setdefault(m.group(1), issue)
            continue
        # Migration, once: an issue opened before markers existed is matched
        # by its exact original title and given the marker.
        for item_id in listed:
            if issue.get("title") == TITLE.format(item_id) and item_id not in by_id:
                body = (issue.get("body") or "") + "\n\n" + item_mark(item_id)
                actions.append(("mark", issue["number"], body))
                by_id[item_id] = {**issue, "body": body}
                break
    for r in results:
        issue = by_id.get(r["id"])
        if r["due"]:
            if issue is None:
                actions.append(("open", r["id"], TITLE.format(r["id"]), issue_body(r, week, origin)))
                continue
            texts = [issue.get("body") or ""] + [c.get("body") or "" for c in issue.get("comments") or []
                                                 if ours(c.get("author", ""))]
            if any(week in WEEK_MARK.findall(t) for t in texts):
                actions.append(("note", f"#{issue['number']} ({r['id']}): already reminded in {week}"))
                continue
            actions.append(("comment", issue["number"],
                            f"Still due: {overdue_text(r)}. {r['reason']}.{origin}\n\n{week_mark(week)}"))
        elif issue is not None:
            if close:
                actions.append(("close", issue["number"], f"Done: read {r['read']} ({r['reason']}). Closing."))
            else:
                actions.append(("note", f"#{issue['number']} ({r['id']}) reads as done; a dispatch does not close it"))
    for item_id, issue in by_id.items():
        if item_id in listed:
            continue
        if close:
            actions.append(("close", issue["number"],
                            f"`{item_id}` was removed from docs/internals/gardening.yaml, so nothing reminds about it any more. Closing."))
        else:
            actions.append(("note", f"#{issue['number']} names `{item_id}`, which is no longer listed; a dispatch does not close it"))
    return actions


def own_login() -> str:
    """The login the token acts as (github-actions[bot] for GITHUB_TOKEN)."""
    got = gh("api", "graphql", "-f", "query=query { viewer { login } }", "--jq", ".data.viewer.login", check=False)
    return got.stdout.strip() if got.returncode == 0 else ""


def sync_issues(results: list[dict], close: bool, gardener: str, origin: str, today: dt.date) -> int:
    trusted = own_login()
    if not trusted:
        # Without it no marker can be trusted, and every run would open a
        # duplicate of each issue.
        print("::error::cannot read the token's own login from the API; no issue changed")
        return 1
    print(f"trusting markers written by {trusted}")
    gh("label", "create", LABEL, "--force", "--color", "0E8A16",
       "--description", "Local upkeep that is due; closed when it is done")
    raw = json.loads(gh("issue", "list", "--label", LABEL, "--state", "open", "--limit", "200",
                        "--json", "number,title,author,body,comments").stdout)
    issues = [{"number": i["number"], "title": i["title"], "author": (i.get("author") or {}).get("login", ""),
               "body": i.get("body") or "",
               "comments": [{"author": (c.get("author") or {}).get("login", ""), "body": c.get("body") or ""}
                            for c in i.get("comments") or []]} for i in raw]
    for action in plan(results, issues, iso_week(today), close, origin, trusted):
        kind = action[0]
        if kind == "mark":
            gh("issue", "edit", str(action[1]), "--body", action[2])
            print(f"marked #{action[1]}")
        elif kind == "open":
            _, item_id, title, body = action
            args = ["issue", "create", "--title", title, "--label", LABEL, "--body", body]
            made = gh(*args, *(["--assignee", gardener] if gardener else []), check=False)
            if made.returncode != 0 and gardener:
                print(f"::warning::could not assign {title} to {gardener}: {made.stderr.strip()}; opening it unassigned")
                made = gh(*args, check=False)
            if made.returncode != 0:
                print(f"::error::could not open {title}: {made.stderr.strip()}")
                return 1
            print(f"opened {made.stdout.strip()} ({item_id})")
        elif kind == "comment":
            gh("issue", "comment", str(action[1]), "--body", action[2])
            print(f"commented on #{action[1]}")
        elif kind == "close":
            gh("issue", "comment", str(action[1]), "--body", action[2])
            gh("issue", "close", str(action[1]))
            print(f"closed #{action[1]}")
        else:
            print(action[1])
    return 0


# ── release gate ─────────────────────────────────────────────────────────


def release_gate(root: str, item_id: str, today: dt.date) -> tuple[bool, str]:
    """Whether a release may proceed, and the line saying why."""
    items = {i["id"]: i for i in load_items(root)}
    item = items.get(item_id)
    if item is None or "release_gate_days" not in item:
        return False, f"{item_id} has no release_gate_days in {LIST}"
    limit = int(item["release_gate_days"])
    verdict = check_item(root, {**item, "cadence_days": limit}, today)
    verdict["reason"] = verdict["reason"].replace(f"; the cadence is {limit} days", "")
    if verdict["due"]:
        return False, (f"{item_id} is too old to release with: {verdict['reason']}; the release gate allows "
                       f"{limit} days. Refresh it with `{item['run']}` and commit the result.")
    return True, f"{item_id}: {verdict['reason']}; within the release gate of {limit} days"


# ── self-test ────────────────────────────────────────────────────────────


def gate_self_test() -> list[str]:
    import tempfile
    failures = []
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, "docs", "internals"))
        with open(os.path.join(root, LIST), "w") as fh:
            fh.write("items:\n  - id: cat\n    run: make update-model-catalog\n    cadence_days: 30\n"
                     "    release_gate_days: 60\n    source:\n      kind: json-field\n      file: m.json\n      field: as_of\n"
                     "  - id: nogate\n    run: x\n    cadence_days: 30\n    source:\n      kind: json-field\n      file: m.json\n      field: as_of\n")
        today = dt.date(2026, 11, 24)
        for days, want in ((59, True), (60, True), (61, False)):
            with open(os.path.join(root, "m.json"), "w") as fh:
                json.dump({"as_of": (today - dt.timedelta(days=days)).isoformat()}, fh)
            ok, line = release_gate(root, "cat", today)
            if ok != want:
                failures.append(f"release gate at {days} days: {ok}, want {want} ({line})")
        ok, line = release_gate(root, "cat", today)
        if "make update-model-catalog" not in line:
            failures.append(f"the refusal names the refresh command: {line}")
        with open(os.path.join(root, "m.json"), "w") as fh:
            json.dump({"note": "no date"}, fh)
        if release_gate(root, "cat", today)[0]:
            failures.append("a missing date must refuse the release")
        if release_gate(root, "nogate", today)[0]:
            failures.append("an item with no release_gate_days must refuse, not pass")
    return failures


def self_test() -> int:
    import tempfile
    today = dt.date(2026, 11, 23)
    failures = []
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, "docs", "internals"))

        def write(rel, text):
            p = os.path.join(root, rel)
            os.makedirs(os.path.dirname(p), exist_ok=True)
            with open(p, "w") as fh:
                fh.write(text)

        write("fresh.json", '{"as_of": "2026-11-15"}')
        write("stale.json", '{"as_of": "2026-07-14"}')
        write("future.json", '{"as_of": "2027-01-01"}')
        write("nofield.json", '{"other": 1}')
        write("dates.yaml", "a: { verified: 2026-10-01 }\nb: { latest_verified: x, verified: 2026-06-01 }\n")
        write("docs/internals/format-ops-ledger.json", json.dumps({"rituals": {
            "fresh": {"cadence_days": 14, "last_run": "2026-11-20"},
            "late": {"cadence_days": 30, "last_run": "2026-05-20"},
            "never": {"cadence_days": 60, "last_run": None},
            "watermark-only": {"cadence_days": 0, "last_run": None},
            "ci": {"cadence_days": 30, "ci_owned": True, "last_run": None},
        }}))

        def item(i, kind, file, **kw):
            return {"id": i, "cadence_days": 30, "source": {"kind": kind, "file": file, **kw}}

        cases = {
            "not due": (item("a", "json-field", "fresh.json", field="as_of"), False, "8 days ago"),
            "due": (item("b", "json-field", "stale.json", field="as_of"), True, "132 days ago"),
            "future date": (item("c", "json-field", "future.json", field="as_of"), True, "in the future"),
            "missing file": (item("d", "json-field", "gone.json", field="as_of"), True, "the file is missing"),
            "missing field": (item("e", "json-field", "nofield.json", field="as_of"), True, "has no `as_of`"),
            "oldest date": (item("f", "oldest-date", "dates.yaml", pattern=r"\bverified: (\d{4}-\d{2}-\d{2})"), True,
                            "oldest of 2 dates is 2026-06-01"),
            "missing pattern file": (item("g", "oldest-date", "gone.yaml", pattern="x"), True, "the file is missing"),
        }
        for name, (it, want_due, want_text) in cases.items():
            got = check_item(root, it, today)
            if got["due"] != want_due or want_text not in got["reason"] + got["read"]:
                failures.append(f"{name}: got {got}")
        b = check_item(root, cases["due"][0], today)
        if b["overdue_days"] != 102:
            failures.append(f"due: overdue_days {b['overdue_days']}, want 102")

        ops = check_format_ops(root, "docs/internals/format-ops-ledger.json", today)
        joined = " ".join(ops.get("details") or [])
        if not ops["due"] or "late: 187 days" not in joined or "never: never run" not in joined:
            failures.append(f"format-ops due rituals: {ops}")
        for quiet in ("fresh:", "watermark-only", "ci:"):
            if quiet in joined:
                failures.append(f"format-ops: {quiet} is not due by cadence; got {joined}")
        gone = check_format_ops(root, "docs/internals/missing-ledger.json", today)
        if not gone["due"] or "missing" not in gone["read"]:
            failures.append(f"format-ops missing ledger must be due: {gone}")
    failures += plan_self_test()
    failures += gate_self_test()
    if failures:
        print("gardening-due self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("gardening-due self-test: OK")
    return 0


def plan_self_test() -> list[str]:
    failures = []
    week = "2026-W48"
    due = {"id": "prices", "due": True, "purpose": "p", "run": "r", "needs": "", "reason": "x",
           "overdue_days": 5, "read": "f"}
    done = {**due, "id": "catalog", "due": False}
    bot = "github-actions[bot]"
    marked = {"number": 1, "title": "Edited by someone", "author": "app/github-actions",
              "body": "text\n" + item_mark("prices"), "comments": []}

    def plan(*args, **kw):
        return globals()["plan"](*args, trusted=bot, **kw)

    def kinds(actions):
        return [(a[0], a[1]) for a in actions]

    # A renamed issue is still found by its marker: a comment, no duplicate.
    got = kinds(plan([due], [marked], week, close=True))
    if got != [("comment", 1)]:
        failures.append(f"renamed issue: {got}")
    # At most once a week: a reminder already written this week is not repeated.
    seen = {**marked, "comments": [{"author": "github-actions", "body": "Still due\n\n" + week_mark(week)}]}
    got = kinds(plan([due], [seen], week, close=True))
    if got[0][0] != "note" or len(got) != 1:
        failures.append(f"second run in a week: {got}")
    opened_now = {**marked, "body": marked["body"] + "\n" + week_mark(week)}
    if kinds(plan([due], [opened_now], week, close=True))[0][0] != "note":
        failures.append("an issue opened this week is not reminded again the same week")
    last_week = {**marked, "comments": [{"author": "github-actions", "body": week_mark("2026-W47")}]}
    if kinds(plan([due], [last_week], week, close=True)) != [("comment", 1)]:
        failures.append("a reminder from last week does not stop this week's")
    # A removed item's issue is closed by the schedule, not by a dispatch.
    gone = {"number": 2, "title": "t", "author": "app/github-actions", "body": item_mark("retired"), "comments": []}
    got = plan([due], [marked, gone], week, close=True)
    if ("close", 2) not in kinds(got) or "removed" not in [a for a in got if a[:2] == ("close", 2)][0][2]:
        failures.append(f"removed item: {got}")
    if ("close", 2) in kinds(plan([due], [marked, gone], week, close=False)):
        failures.append("a dispatch closed a removed item's issue")
    # Done closes on the schedule only; a due item with no issue opens one,
    # carrying both markers.
    got = plan([done], [{"number": 3, "title": "x", "author": "app/github-actions", "body": item_mark("catalog"),
                         "comments": []}], week, close=True)
    if kinds(got) != [("close", 3)]:
        failures.append(f"done item: {got}")
    got = plan([due], [], week, close=True)
    if kinds(got) != [("open", "prices")] or item_mark("prices") not in got[0][3] or week_mark(week) not in got[0][3]:
        failures.append(f"open: {got}")
    # Migration: an unmarked issue with the exact original title gets the
    # marker once; one with another title is left alone.
    legacy = {"number": 4, "title": TITLE.format("prices"), "author": "app/github-actions", "body": "old", "comments": []}
    got = plan([due], [legacy], week, close=True)
    if kinds(got) != [("mark", 4), ("comment", 4)] or item_mark("prices") not in got[0][2]:
        failures.append(f"migration: {got}")
    stranger = {"number": 5, "title": "Gardening due: prices (renamed)", "author": "app/github-actions",
                "body": "old", "comments": []}
    got = kinds(plan([due], [stranger], week, close=True))
    if got != [("open", "prices")]:
        failures.append(f"an unmarked issue with another title must not match: {got}")
    # Markers from anyone but the workflow are ignored: a stranger's weekly
    # mark does not silence the reminder, and a stranger's issue carrying an
    # item mark (or the original title) is not taken for the item's issue.
    planted = {**marked, "comments": [{"author": "someone", "body": week_mark(week)}]}
    if kinds(plan([due], [planted], week, close=True)) != [("comment", 1)]:
        failures.append("a stranger's weekly mark silenced the reminder")
    impostor = {"number": 6, "title": TITLE.format("prices"), "author": "someone",
                "body": item_mark("prices"), "comments": []}
    got = kinds(plan([due], [impostor], week, close=True))
    if got != [("open", "prices")]:
        failures.append(f"a stranger's issue with the item mark was taken for the item's issue: {got}")
    if kinds(globals()["plan"]([due], [marked], week, close=True, trusted="")) != [("open", "prices")]:
        failures.append("with no trusted login, no marker may be trusted")
    if iso_week(dt.date(2026, 11, 23)) != "2026-W48" or iso_week(dt.date(2027, 1, 1)) != "2026-W53":
        failures.append("iso_week")
    return failures


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--sync-issues", action="store_true")
    ap.add_argument("--no-close", action="store_true")
    ap.add_argument("--self-test", action="store_true")
    ap.add_argument("--release-gate", metavar="ID")
    ap.add_argument("--report-only", action="store_true")
    ap.add_argument("--today", help="YYYY-MM-DD, for tests")
    ap.add_argument("--root", default=ROOT)
    args = ap.parse_args()
    if args.self_test:
        return self_test()
    today = dt.date.fromisoformat(args.today) if args.today else dt.date.today()
    if args.release_gate:
        ok, line = release_gate(args.root, args.release_gate, today)
        if ok:
            print(line)
            return 0
        if args.report_only:
            print(f"::warning::{line} (a build-only run does not stop)")
            return 0
        print(f"::error::{line}")
        return 1
    results = report(args.root, today)
    if args.json:
        print(json.dumps(results, indent=2))
    else:
        print_report(results, today)
    if args.sync_issues:
        origin = ""
        if args.no_close:
            origin = "\n\n(From a dispatch of the Gardening workflow.)"
        return sync_issues(results, close=not args.no_close, gardener=os.environ.get("GARDENER", "").strip(),
                           origin=origin, today=today)
    return 0


if __name__ == "__main__":
    sys.exit(main())
