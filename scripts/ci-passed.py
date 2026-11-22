#!/usr/bin/env python3
"""Decide whether a commit passed CI: the body of the `CI passed` job.

`CI passed` is the one status check the main branch requires. It is the last
job of .github/workflows/ci.yml and answers for everything that ran on the
commit, so a pull request with auto-merge enabled merges when this job is
green and at no other time.

    scripts/ci-passed.py jobs         # every job of the CI workflow
    scripts/ci-passed.py workflows    # every other workflow on the same commit
    scripts/ci-passed.py needs        # guard: the job lists every other job
    scripts/ci-passed.py --self-test

`jobs` reads NEEDS, the JSON of the `needs` context. `changes` must have
succeeded: every path-gated job skips when it fails, and a skip is a pass.
Every other job must have succeeded or been skipped.

`workflows` reads GITHUB_REPOSITORY, GITHUB_RUN_ID, HEAD_SHA and GITHUB_TOKEN.
It lists the workflow runs for HEAD_SHA, leaves out the runs a person started
by hand or a schedule started, takes the newest run of each workflow other
than this one, and waits until each has completed. It fails as soon as one
completes without passing, and when WAIT_MINUTES (default 50) pass with a run
still going. It passes only when two listings SETTLE_SECONDS (default 30)
apart both contain this run and show the same runs, all passed: a listing
without this run is incomplete, and a run created late is seen by the second.
The path-filtered workflows (E2E, the docs builds, format acceptance and the
rest) cannot be required checks themselves, because a pull request outside
their paths never reports them; this step is how they still hold a merge.

`needs` reads the workflow file and fails when a job is missing from the
`needs` of `ci-passed`. A job the list omits would not hold a merge. A line
under `jobs:` that it cannot read as a job is an error, never a job silently
left out.

The workflow file and this script come from the pull request's own merge
commit, as every `pull_request` workflow does, so a pull request that edits
them changes its own gate. That is why a diff that touches a check waits for
the person (CLAUDE.md, "Verify before you commit").

Exit status: 0 passed, 1 did not pass, 2 the input could not be read.
"""

from __future__ import annotations

import json
import os
import re
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WORKFLOW = os.path.join(ROOT, ".github", "workflows", "ci.yml")
GATE_JOB = "ci-passed"

PASSING_JOB_RESULTS = ("success", "skipped")
PASSING_RUN_CONCLUSIONS = ("success", "skipped")
# Runs nobody's push started: an experiment on the branch, read by whoever
# started it. Every other event on the commit is judged.
MANUAL_EVENTS = ("workflow_dispatch", "repository_dispatch", "schedule")


def failed_jobs(needs: dict) -> list[str]:
    """Return one line per job that keeps the commit from passing."""
    problems = []
    if "changes" not in needs:
        problems.append("changes: missing from the needs context")
    for job in sorted(needs):
        result = needs[job].get("result")
        if job == "changes" and result != "success":
            problems.append(f"changes: {result} (path-gated jobs skip when it does not succeed)")
        elif result not in PASSING_JOB_RESULTS:
            problems.append(f"{job}: {result}")
    return problems


def newest_other_runs(runs: list[dict], own_run_id: int) -> list[dict]:
    """Return the newest run of each workflow except this one.

    A pull request event can start a workflow twice on one commit (a label
    added after the push); the older run is cancelled and does not count.
    Raises LookupError when the listing does not contain this run: such a
    listing is incomplete and proves nothing about the other workflows.
    """
    own_workflow = next((r["workflow_id"] for r in runs if r["id"] == own_run_id), None)
    if own_workflow is None:
        raise LookupError(f"the listing of {len(runs)} run(s) does not contain this run")
    newest: dict[int, dict] = {}
    for run in runs:
        if run["event"] in MANUAL_EVENTS or run["workflow_id"] == own_workflow:
            continue
        kept = newest.get(run["workflow_id"])
        if kept is None or (run["run_number"], run["id"]) > (kept["run_number"], kept["id"]):
            newest[run["workflow_id"]] = run
    return sorted(newest.values(), key=lambda r: r["name"])


def judge_runs(runs: list[dict]) -> tuple[list[str], list[str]]:
    """Split runs into those that failed and those still going, as lines."""
    failed, going = [], []
    for run in runs:
        if run["status"] != "completed":
            going.append(f"{run['name']}: {run['status']}")
        elif run["conclusion"] not in PASSING_RUN_CONCLUSIONS:
            failed.append(f"{run['name']}: {run['conclusion']} ({run['html_url']})")
    return failed, going


class UnreadableWorkflow(ValueError):
    """The workflow file has a shape the guard does not read."""


def workflow_jobs(text: str) -> tuple[list[str], list[str]]:
    """Return the job ids of a workflow file and the `needs` of the gate job.

    The file is read by indentation: a job is a key at two spaces under
    `jobs:`, and the gate's `needs` is a block list at six spaces. Any other
    shape raises UnreadableWorkflow with the line number: a top-level key
    after `jobs:`, a line at two spaces that is not a plain job key, and a
    gate `needs` written inline or holding anything but plain job ids.
    """
    jobs, gate_needs = [], []
    in_jobs = in_gate = in_needs = False
    for number, line in enumerate(text.splitlines(), start=1):
        if not line.strip() or line.strip().startswith("#"):
            continue
        if re.match(r"^jobs:\s*(#.*)?$", line):
            if in_jobs:
                raise UnreadableWorkflow(f"line {number}: a second `jobs:`")
            in_jobs = True
            continue
        if not in_jobs:
            continue
        indent = len(line) - len(line.lstrip(" "))
        if indent < 2:
            raise UnreadableWorkflow(f"line {number}: `jobs:` is not the last top-level key")
        if indent == 2:
            job = re.match(r"^  ([A-Za-z0-9_-]+):\s*(#.*)?$", line)
            if not job:
                raise UnreadableWorkflow(f"line {number}: cannot read {line.strip()!r} as a job")
            jobs.append(job.group(1))
            in_gate, in_needs = job.group(1) == GATE_JOB, False
            continue
        if not in_gate:
            continue
        if indent == 4:
            in_needs = bool(re.match(r"^    needs:\s*(#.*)?$", line))
            if re.match(r"^    needs:", line) and not in_needs:
                raise UnreadableWorkflow(f"line {number}: write the needs of {GATE_JOB} as a block list")
            continue
        if in_needs:
            item = re.match(r"^      - ([A-Za-z0-9_-]+)\s*(#.*)?$", line)
            if not item:
                raise UnreadableWorkflow(f"line {number}: cannot read {line.strip()!r} as a job id")
            gate_needs.append(item.group(1))
    if not in_jobs:
        raise UnreadableWorkflow("no `jobs:` key")
    return jobs, gate_needs


def missing_needs(text: str) -> list[str]:
    """Return one line per defect in the gate job's `needs` list."""
    try:
        jobs, gate_needs = workflow_jobs(text)
    except UnreadableWorkflow as err:
        return [str(err)]
    if GATE_JOB not in jobs:
        return [f"no job named {GATE_JOB}"]
    problems = [f"{job}: not in the needs of {GATE_JOB}" for job in jobs if job != GATE_JOB and job not in gate_needs]
    problems += [f"{job}: in the needs of {GATE_JOB} but not a job" for job in gate_needs if job not in jobs]
    return problems


def fetch_runs(repository: str, head_sha: str, token: str) -> list[dict]:
    runs, page = [], 1
    while True:
        url = f"https://api.github.com/repos/{repository}/actions/runs?head_sha={head_sha}&per_page=100&page={page}"
        request = urllib.request.Request(url, headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
        })
        with urllib.request.urlopen(request, timeout=30) as response:
            batch = json.load(response)["workflow_runs"]
        runs += batch
        if len(batch) < 100:
            return runs
        page += 1


def report(title: str, problems: list[str]) -> int:
    if not problems:
        return 0
    print(f"::error::{title}")
    for line in problems:
        print(f"  {line}")
    return 1


def command_jobs() -> int:
    try:
        needs = json.loads(os.environ["NEEDS"])
    except (KeyError, ValueError) as err:
        print(f"::error::cannot read NEEDS: {err}")
        return 2
    for job in sorted(needs):
        print(f"{needs[job].get('result'):10} {job}")
    return report("A job of the CI workflow did not pass", failed_jobs(needs))


def command_workflows() -> int:
    try:
        repository = os.environ["GITHUB_REPOSITORY"]
        own_run_id = int(os.environ["GITHUB_RUN_ID"])
        head_sha = os.environ["HEAD_SHA"]
        token = os.environ["GITHUB_TOKEN"]
    except (KeyError, ValueError) as err:
        print(f"::error::missing input: {err}")
        return 2
    deadline = time.monotonic() + 60 * float(os.environ.get("WAIT_MINUTES", "50"))
    interval = float(os.environ.get("POLL_SECONDS", "20"))
    settle = float(os.environ.get("SETTLE_SECONDS", "30"))
    last = passed_before = None
    while True:
        try:
            runs = newest_other_runs(fetch_runs(repository, head_sha, token), own_run_id)
        except (urllib.error.URLError, TimeoutError, ValueError, KeyError, LookupError) as err:
            if time.monotonic() > deadline:
                print(f"::error::cannot list the workflow runs of {head_sha[:9]}: {err}")
                return 2
            print(f"cannot list the workflow runs ({err}), trying again")
            passed_before = None
            time.sleep(interval)
            continue
        failed, going = judge_runs(runs)
        if failed:
            return report("Another workflow on this commit did not pass", failed)
        if not going:
            seen = sorted(run["id"] for run in runs)
            if seen == passed_before:
                for run in runs:
                    print(f"{run['conclusion']:10} {run['event']:20} {run['name']}")
                print(f"{len(runs)} other workflow(s) on {head_sha[:9]} passed")
                return 0
            passed_before = seen
            print(f"{len(runs)} other workflow(s) passed; listing again in {settle:g} s to confirm")
            time.sleep(settle)
            continue
        passed_before = None
        if going != last:
            print("waiting for: " + "; ".join(going))
            last = going
        if time.monotonic() > deadline:
            return report("A workflow on this commit is still running", going)
        time.sleep(interval)


def command_needs() -> int:
    try:
        with open(WORKFLOW, encoding="utf-8") as handle:
            text = handle.read()
    except OSError as err:
        print(f"cannot read {WORKFLOW}: {err}", file=sys.stderr)
        return 2
    problems = missing_needs(text)
    for line in problems:
        print(f"ci-passed needs: {line}", file=sys.stderr)
    return 1 if problems else 0


def self_test() -> int:
    failures = []

    def expect(name: str, got, want) -> None:
        if got != want:
            failures.append(f"{name}: got {got!r}, want {want!r}")

    ok = {"changes": {"result": "success"}, "build": {"result": "success"}, "harness": {"result": "skipped"}}
    expect("jobs pass", failed_jobs(ok), [])
    expect("a failed job", failed_jobs({**ok, "build": {"result": "failure"}}), ["build: failure"])
    expect("a cancelled job", failed_jobs({**ok, "build": {"result": "cancelled"}}), ["build: cancelled"])
    expect(
        "changes failed and the rest skipped",
        len(failed_jobs({"changes": {"result": "failure"}, "build": {"result": "skipped"}})),
        1,
    )
    expect("changes skipped", len(failed_jobs({"changes": {"result": "skipped"}})), 1)
    expect("changes absent", len(failed_jobs({"build": {"result": "success"}})), 1)

    def run(run_id, workflow_id, name, status, conclusion, event="pull_request", number=1):
        return {
            "id": run_id, "workflow_id": workflow_id, "name": name, "status": status,
            "conclusion": conclusion, "event": event, "run_number": number, "html_url": f"url/{run_id}",
        }

    runs = [
        run(1, 10, "CI", "in_progress", None),
        run(2, 20, "E2E", "completed", "success"),
        run(3, 30, "Docs", "completed", "cancelled", number=4),
        run(4, 30, "Docs", "completed", "success", number=5),
        run(5, 40, "Deploy", "completed", "failure", event="push"),
        run(6, 50, "Relock", "completed", "skipped", event="pull_request_target"),
    ]
    others = newest_other_runs(runs, own_run_id=1)
    expect("own workflow left out, every event judged", [r["id"] for r in others], [5, 4, 2, 6])
    expect("a failed run of another event", judge_runs(others)[0], ["Deploy: failure (url/5)"])
    runs[4] = run(5, 40, "Nightly", "completed", "failure", event="workflow_dispatch")
    others = newest_other_runs(runs, own_run_id=1)
    expect("a run started by hand left out", [r["id"] for r in others], [4, 2, 6])
    expect("all passed", judge_runs(others), ([], []))
    expect("a neutral workflow", len(judge_runs([run(10, 80, "Y", "completed", "neutral")])[0]), 1)
    try:
        newest_other_runs(runs[1:], own_run_id=1)
        failures.append("a listing without this run: no error")
    except LookupError:
        pass
    try:
        newest_other_runs([], own_run_id=1)
        failures.append("an empty listing: no error")
    except LookupError:
        pass
    expect(
        "a failed workflow",
        judge_runs(newest_other_runs(runs + [run(7, 20, "E2E", "completed", "failure", number=2)], 1))[0],
        ["E2E: failure (url/7)"],
    )
    expect(
        "a running workflow",
        judge_runs(newest_other_runs(runs + [run(8, 60, "l10n", "queued", None)], 1))[1],
        ["l10n: queued"],
    )
    expect("a timed out workflow", len(judge_runs([run(9, 70, "X", "completed", "timed_out")])[0]), 1)
    expect("no other workflow", judge_runs(newest_other_runs([run(1, 10, "CI", "in_progress", None)], 1)), ([], []))

    workflow = "\n".join([
        "name: CI", "on:", "  push:", "jobs:", "  changes:", "    name: Detect", "  build:  # comment",
        "    needs: changes", "    steps:", "      - uses: x", "  ci-passed:", "    name: CI passed",
        "    needs:", "      - changes", "      # a comment", "      - build", "    runs-on: ubuntu-latest",
        "    steps:", "      - run: true",
    ])
    expect("needs complete", missing_needs(workflow), [])
    expect(
        "a job the gate omits",
        missing_needs(workflow.replace("      - build\n", "")),
        ["build: not in the needs of ci-passed"],
    )
    expect(
        "a need that is not a job",
        missing_needs(workflow.replace("      - build\n", "      - build\n      - gone\n")),
        ["gone: in the needs of ci-passed but not a job"],
    )
    expect("no gate job", missing_needs(workflow.replace("ci-passed", "other")), ["no job named ci-passed"])
    expect("steps are not needs", workflow_jobs(workflow)[1], ["changes", "build"])
    for name, broken in (
        ("a job written inline", workflow.replace("  build:  # comment", "  build: {runs-on: x}")),
        ("a quoted job key", workflow.replace("  build:  # comment", '  "build":')),
        ("inline needs", workflow.replace("    needs:\n      - changes", "    needs: [changes, build]\n    env:\n      - changes")),
        ("a need that is not a plain id", workflow.replace("      - build\n", "      - ${{ x }}\n")),
        ("a key after jobs", workflow + "\nenv:\n  A: b"),
        ("no jobs key", "name: CI"),
    ):
        problems = missing_needs(broken)
        if len(problems) != 1 or ("line" not in problems[0] and "jobs" not in problems[0]):
            failures.append(f"{name}: got {problems!r}, want one unreadable-workflow line")

    for line in failures:
        print(f"self-test: {line}", file=sys.stderr)
    if not failures:
        print("ci-passed self-test: ok")
    return 1 if failures else 0


def main(argv: list[str]) -> int:
    commands = {"jobs": command_jobs, "workflows": command_workflows, "needs": command_needs, "--self-test": self_test}
    if len(argv) != 2 or argv[1] not in commands:
        print(__doc__, file=sys.stderr)
        return 2
    return commands[argv[1]]()


if __name__ == "__main__":
    sys.exit(main(sys.argv))
