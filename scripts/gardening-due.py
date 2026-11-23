#!/usr/bin/env python3
"""Report the gardening that is due: the upkeep a person runs locally.

    scripts/gardening-due.py                 print what is due (exit 0)
    scripts/gardening-due.py --json          the same, as JSON
    scripts/gardening-due.py --sync-issues   keep one `gardening` issue per item
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
what was read. --no-close opens and comments only. GARDENER names a login to
assign new issues to; empty assigns nobody.

This is a report: it never fails on what is due.
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


def gh(*args: str, check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(["gh", *args], capture_output=True, text=True, check=check)


def issue_body(r: dict) -> str:
    lines = [r["purpose"], "", f"**Run:** {r['run']}", "", f"**Status:** {overdue_text(r)}. {r['reason']}."]
    for d in r.get("details") or []:
        lines.append(f"- {d}")
    if r["needs"]:
        lines += ["", f"**Needs:** {r['needs']}."]
    lines += ["", "The work is local; this issue is a reminder. The weekly Gardening workflow comments here while "
              "the item is due and closes the issue when it reads the item as done "
              "(docs/internals/gardening.yaml)."]
    return "\n".join(lines)


def sync_issues(results: list[dict], close: bool, gardener: str, origin: str) -> int:
    gh("label", "create", LABEL, "--force", "--color", "0E8A16",
       "--description", "Local upkeep that is due; closed when it is done")
    listed = json.loads(gh("issue", "list", "--label", LABEL, "--state", "open", "--limit", "200",
                           "--json", "number,title").stdout)
    open_by_title = {i["title"]: i["number"] for i in listed}
    for r in results:
        title = TITLE.format(r["id"])
        number = open_by_title.get(title)
        if r["due"]:
            if number is None:
                args = ["issue", "create", "--title", title, "--label", LABEL, "--body", issue_body(r) + origin]
                made = gh(*args, *(["--assignee", gardener] if gardener else []), check=False)
                if made.returncode != 0 and gardener:
                    print(f"::warning::could not assign {title} to {gardener}: {made.stderr.strip()}; opening it unassigned")
                    made = gh(*args, check=False)
                if made.returncode != 0:
                    print(f"::error::could not open {title}: {made.stderr.strip()}")
                    return 1
                print(f"opened {made.stdout.strip()} ({r['id']}: {overdue_text(r)})")
            else:
                gh("issue", "comment", str(number), "--body", f"Still due: {overdue_text(r)}. {r['reason']}.{origin}")
                print(f"commented on #{number} ({r['id']}: {overdue_text(r)})")
        elif number is not None:
            if close:
                gh("issue", "comment", str(number), "--body", f"Done: read {r['read']} ({r['reason']}). Closing.")
                gh("issue", "close", str(number))
                print(f"closed #{number} ({r['id']}: {r['reason']})")
            else:
                print(f"#{number} ({r['id']}) reads as done; a dispatch does not close it")
    return 0


# ── self-test ────────────────────────────────────────────────────────────


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
    if failures:
        print("gardening-due self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("gardening-due self-test: OK")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--sync-issues", action="store_true")
    ap.add_argument("--no-close", action="store_true")
    ap.add_argument("--self-test", action="store_true")
    ap.add_argument("--today", help="YYYY-MM-DD, for tests")
    ap.add_argument("--root", default=ROOT)
    args = ap.parse_args()
    if args.self_test:
        return self_test()
    today = dt.date.fromisoformat(args.today) if args.today else dt.date.today()
    results = report(args.root, today)
    if args.json:
        print(json.dumps(results, indent=2))
    else:
        print_report(results, today)
    if args.sync_issues:
        origin = ""
        if args.no_close:
            origin = "\n\n(From a dispatch of the Gardening workflow.)"
        return sync_issues(results, close=not args.no_close, gardener=os.environ.get("GARDENER", "").strip(), origin=origin)
    return 0


if __name__ == "__main__":
    sys.exit(main())
