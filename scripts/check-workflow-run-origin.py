#!/usr/bin/env python3
"""Guard: no workflow acts for an account outside this repository.

A workflow triggered by `workflow_run` runs from the default branch with the
permissions and secrets it declares, whoever started the run it follows. A
pull request from a fork starts runs of the `pull_request` workflows, so a
follower that does not ask where the run came from acts on a fork's run as
if it were ours. Two things the event reports are taken from the fork and
prove nothing: `head_branch` is the fork's branch name, and a `branches:`
filter matches that same name, so a fork's `main` passes a test for `main`.

So every job of a `workflow_run` workflow carries a condition that holds it
to this repository. One of these three must be required for the job to run
on a `workflow_run` event:

    github.event.workflow_run.head_repository.full_name == github.repository
    github.event.workflow_run.event == 'push'
    github.event.workflow_run.event == 'schedule'

A push and a schedule happen in this repository only. The condition counts
when the job's `if:` cannot be true on a `workflow_run` event without it:
it is one side of a top-level `&&`, or every other side of an `||` is a test
that the event is some other one (`github.event_name == 'workflow_dispatch'`,
`github.event_name != 'workflow_run'`). A job with no `if:` of its own is
held when a job it `needs` is held, because it is skipped with that job. A
condition this script cannot read is reported, never assumed to hold.

Some triggers are refused in every workflow, because any GitHub account can
cause the event and the run then has this repository's permissions and
secrets: `pull_request_target`, `issues`, `issue_comment`, `discussion`,
`discussion_comment`, `fork`, `watch` and `gollum` (a wiki edit). The two
review events, `pull_request_review` and `pull_request_review_comment`, are
refused with them: a review of a fork's pull request runs read-only, and a
review of one of ours does not, so a job cannot tell from the trigger which
it has. So are `status`, `check_run` and `check_suite`: an installed app
posts them on a fork's commit too, and the run they start is not read-only.
No workflow here uses any of them. One that needs to must first be able to
prove who caused the event, and that rule belongs in this script.

    scripts/check-workflow-run-origin.py              check the repository
    scripts/check-workflow-run-origin.py --self-test  prove it on fixtures

Exit status: 0 held, 1 a job or trigger is reported, 2 a file is unreadable.
"""

from __future__ import annotations

import glob
import os
import re
import sys

import yaml

ORIGIN_TESTS = frozenset(
    {
        "github.event.workflow_run.head_repository.full_name == github.repository",
        "github.repository == github.event.workflow_run.head_repository.full_name",
        "github.event.workflow_run.event == 'push'",
        "github.event.workflow_run.event == 'schedule'",
    }
)

# Events an account with no access to the repository can cause, each starting
# a run from the default branch.
REFUSED_EVENTS = (
    "pull_request_target",
    "issues",
    "issue_comment",
    "discussion",
    "discussion_comment",
    "fork",
    "watch",
    "pull_request_review",
    "pull_request_review_comment",
    "gollum",
    "status",
    "check_run",
    "check_suite",
)

OTHER_EVENT = re.compile(r"^github\.event_name == '([a-z_]+)'$")
NOT_THIS_EVENT = "github.event_name != 'workflow_run'"


class Unreadable(Exception):
    """A condition whose structure this script cannot follow."""


def split_top(expr: str, operator: str) -> list[str]:
    """Split expr at each operator outside parentheses and string literals."""
    parts, depth, start, i = [], 0, 0, 0
    while i < len(expr):
        ch = expr[i]
        if ch == "'":
            end = i + 1
            while True:
                end = expr.find("'", end)
                if end < 0:
                    raise Unreadable("a string literal is not closed")
                if expr[end : end + 2] == "''":
                    end += 2
                    continue
                break
            i = end + 1
            continue
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth < 0:
                raise Unreadable("a parenthesis closes nothing")
        elif depth == 0 and expr.startswith(operator, i):
            parts.append(expr[start:i])
            i += len(operator)
            start = i
            continue
        i += 1
    if depth != 0:
        raise Unreadable("a parenthesis is not closed")
    parts.append(expr[start:])
    return [part.strip() for part in parts]


def unwrap(expr: str) -> str:
    """Drop parentheses that enclose the whole expression."""
    expr = expr.strip()
    while expr.startswith("(") and expr.endswith(")"):
        inner = expr[1:-1]
        try:
            split_top(inner, "&&")
        except Unreadable:
            break
        expr = inner.strip()
    return expr


def normalise(condition: object) -> str:
    """The expression of an `if:`, without `${{ }}` and on one line."""
    text = " ".join(str(condition).split())
    if text.startswith("${{") and text.endswith("}}") and text.count("${{") == 1:
        text = text[3:-2].strip()
    if "${{" in text or "}}" in text:
        raise Unreadable("the condition mixes text with `${{ }}`")
    return text


def other_event(expr: str) -> bool:
    """Whether expr is true only on an event other than workflow_run."""
    expr = unwrap(expr)
    if expr == NOT_THIS_EVENT:
        return True
    named = OTHER_EVENT.match(expr)
    return bool(named) and named.group(1) != "workflow_run"


def held(expr: str) -> bool:
    """Whether expr can be true on a workflow_run event only for this repository."""
    expr = unwrap(expr)
    sides = split_top(expr, "||")
    if len(sides) > 1:
        return all(other_event(side) or held(side) for side in sides)
    terms = split_top(expr, "&&")
    if len(terms) > 1:
        return any(other_event(term) or held(term) for term in terms)
    return expr in ORIGIN_TESTS or other_event(expr)


def triggers(doc: dict) -> set[str]:
    on = doc.get("on", doc.get(True))
    if isinstance(on, str):
        return {on}
    if isinstance(on, list):
        return {str(item) for item in on}
    if isinstance(on, dict):
        return {str(key) for key in on}
    return set()


def problems(rel: str, doc: dict) -> list[str]:
    found = []
    events = triggers(doc)
    for event in REFUSED_EVENTS:
        if event in events:
            found.append(
                f"{rel}: `{event}` starts a run with this repository's permissions "
                "for any GitHub account that causes the event; no workflow may use it"
            )
    if "workflow_run" not in events:
        return found
    jobs = doc.get("jobs")
    if not isinstance(jobs, dict):
        return found + [f"{rel}: no jobs could be read"]

    own: dict[str, bool] = {}
    for name, job in jobs.items():
        job = job if isinstance(job, dict) else {}
        if "if" not in job:
            continue
        try:
            own[name] = held(normalise(job["if"]))
        except Unreadable as err:
            found.append(f"{rel}: job `{name}`: its `if:` cannot be read ({err})")
            own[name] = False

    def job_held(name: str, seen: frozenset[str]) -> bool:
        if name in own:
            return own[name]
        if name in seen or name not in jobs:
            return False
        job = jobs[name] if isinstance(jobs[name], dict) else {}
        needs = job.get("needs") or []
        needs = [needs] if isinstance(needs, str) else list(needs)
        return any(job_held(str(need), seen | {name}) for need in needs)

    for name in jobs:
        if not job_held(name, frozenset()):
            found.append(
                f"{rel}: job `{name}` can run for a run that did not come from this "
                "repository. Require `github.event.workflow_run.head_repository.full_name "
                "== github.repository` (or an event of `push` or `schedule`) in its `if:`"
            )
    return found


def check(root: str) -> list[str]:
    found = []
    pattern = os.path.join(root, ".github", "workflows", "*.y*ml")
    paths = sorted(glob.glob(pattern))
    if not paths:
        raise Unreadable(f"no workflow under {os.path.dirname(pattern)}")
    for path in paths:
        try:
            with open(path, encoding="utf-8") as fh:
                doc = yaml.safe_load(fh)
        except (OSError, yaml.YAMLError) as err:
            raise Unreadable(f"{path}: {err}") from err
        if isinstance(doc, dict):
            found += problems(os.path.relpath(path, root), doc)
    return found


def main() -> int:
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    try:
        found = check(root)
    except Unreadable as err:
        print(f"check-workflow-run-origin: {err}", file=sys.stderr)
        return 2
    for line in found:
        print(line)
    if found:
        print(f"check-workflow-run-origin: {len(found)} problem(s)")
        return 1
    return 0


OURS = "github.event.workflow_run.head_repository.full_name == github.repository"


def self_test() -> int:
    def follower(condition: object = None, **more: object) -> dict:
        job = {"runs-on": "ubuntu-latest"}
        if condition is not None:
            job["if"] = condition
        job.update(more)
        return {"on": {"workflow_run": {"workflows": ["CI"]}}, "jobs": {"act": job}}

    held_cases = {
        "the origin test alone": follower(OURS),
        "the origin test in ${{ }}": follower("${{ " + OURS + " }}"),
        "beside another test": follower(f"github.event.workflow_run.head_branch == 'main' && {OURS}"),
        "a folded condition over lines": follower(f"github.event.workflow_run.conclusion == 'success'\n  && {OURS}"),
        "a pushed run": follower("github.event.workflow_run.event == 'push'"),
        "a scheduled run": follower("github.event.workflow_run.event == 'schedule'"),
        "or started by hand": follower(f"github.event_name == 'workflow_dispatch' || {OURS}"),
        "or started by hand, then a conjunction": follower(
            f"github.event_name == 'workflow_dispatch' || (github.event.workflow_run.conclusion == 'success' && {OURS})"
        ),
        "not this event, or pushed": follower(
            "github.ref == 'refs/heads/main' && (github.event_name != 'workflow_run' || github.event.workflow_run.event == 'push')"
        ),
        "beside always()": follower(f"always() && {OURS}"),
        "a string holding an operator": follower(f"contains(github.event.workflow_run.name, 'a || b') && {OURS}"),
    }
    reported_cases = {
        "no condition": follower(),
        "a branch name only": follower("github.event.workflow_run.head_branch == 'main'"),
        "the conclusion only": follower("github.event.workflow_run.conclusion == 'success'"),
        "the origin test or anything": follower(f"{OURS} || github.event.workflow_run.conclusion == 'success'"),
        "the origin test or always()": follower(f"always() || {OURS}"),
        "the origin test negated": follower(f"!({OURS})"),
        "the origin test inside a call": follower(f"contains('{OURS}', 'x')"),
        "the origin test inside a string": follower(f"github.event.workflow_run.name != '{OURS.replace(chr(39), chr(39) * 2)}'"),
        "another repository's name": follower("github.event.workflow_run.head_repository.full_name == 'someone/else'"),
        "a pull request event": follower("github.event.workflow_run.event == 'pull_request'"),
        "this event, or the origin test": follower(f"github.event_name == 'workflow_run' || {OURS}"),
        "mixed text and expression": follower("${{ " + OURS + " }} && true"),
        "an unclosed parenthesis": follower(f"({OURS}"),
    }
    failures = []
    for label, doc in held_cases.items():
        if problems("held.yml", doc):
            failures.append(f"held, but reported: {label}")
    for label, doc in reported_cases.items():
        if not problems("reported.yml", doc):
            failures.append(f"reported nothing: {label}")

    def two_jobs(second: dict) -> dict:
        return {
            "on": {"workflow_run": {"workflows": ["CI"]}, "schedule": [{"cron": "0 0 * * *"}]},
            "jobs": {"first": {"if": OURS}, "second": second},
        }

    if problems("needs.yml", two_jobs({"needs": ["first"]})):
        failures.append("held, but reported: a job with no `if:` that needs a held job")
    if problems("needs.yml", two_jobs({"needs": "first"})):
        failures.append("held, but reported: `needs` given as one name")
    if not problems("needs.yml", two_jobs({"needs": ["first"], "if": "always()"})):
        failures.append("reported nothing: a job that needs a held job and runs always()")
    if not problems("needs.yml", two_jobs({})):
        failures.append("reported nothing: a second job with neither `if:` nor `needs`")
    if not problems("needs.yml", two_jobs({"needs": ["second"]})):
        failures.append("reported nothing: a job that needs itself")

    for event in REFUSED_EVENTS:
        for form in (event, ["push", event], {event: {"types": ["created"]}}):
            if not problems("refused.yml", {"on": form, "jobs": {}}):
                failures.append(f"reported nothing: {event} as {type(form).__name__}")
        if not problems("refused.yml", {True: {event: None}, "jobs": {}}):
            failures.append(f"reported nothing: {event} under a key YAML read as true")
    gated = {"on": {"issue_comment": None}, "jobs": {"act": {"if": "github.event.comment.author_association == 'OWNER'"}}}
    if not problems("refused.yml", gated):
        failures.append("reported nothing: issue_comment behind a condition on the author")
    if not problems("true.yml", {True: {"workflow_run": {}}, "jobs": {"act": {}}}):
        failures.append("reported nothing: workflow_run under a key YAML read as true")
    plain = {"pull_request": None, "push": None, "schedule": [{"cron": "0 0 * * *"}], "workflow_dispatch": None}
    if problems("plain.yml", {"on": plain, "jobs": {"act": {}}}):
        failures.append("reported: a workflow with none of these triggers")

    for line in failures:
        print(f"  {line}")
    if failures:
        print("check-workflow-run-origin self-test: FAIL")
        return 1
    print("check-workflow-run-origin self-test: OK")
    return 0


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-test"]:
        sys.exit(self_test())
    if sys.argv[1:]:
        print(__doc__, file=sys.stderr)
        sys.exit(2)
    sys.exit(main())
