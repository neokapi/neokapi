#!/usr/bin/env python3
"""Report how many package builds a CI test job took from the Go build cache.

    scripts/go-cache-hits.py [DIR]     report the action graphs in DIR
    scripts/go-cache-hits.py --self-test

DIR defaults to $GO_ACTIONGRAPH_DIR. setup-go-modules sets that variable, and
the Makefiles then pass `-debug-actiongraph=<a new file in DIR>` to every
`go test`. Each file lists the build actions of one invocation. An action
with a command ran the compiler. An action without one came from the cache.

The share shows whether the warm jobs on main compile what the test jobs
compile. When their flags drift apart, the share drops, while every test still
passes. The report is a notice and a step summary line. It never fails the
job.
"""

from __future__ import annotations

import glob
import json
import os
import sys
import tempfile


def count(directory: str) -> tuple[int, int, int]:
    """(cached, total, files) over every action graph in directory."""
    cached = total = files = 0
    for path in sorted(glob.glob(os.path.join(directory, "actiongraph.*"))):
        try:
            with open(path) as fh:
                actions = json.load(fh)
        except (OSError, ValueError):
            continue  # an invocation that failed before writing its graph
        files += 1
        for action in actions:
            if action.get("Mode") == "build":
                total += 1
                if not action.get("Cmd"):
                    cached += 1
    return cached, total, files


def line(cached: int, total: int, files: int) -> str:
    if not total:
        return "Go build cache: no package builds recorded"
    return (f"Go build cache: {cached} of {total} package builds from the cache "
            f"({100 * cached / total:.0f}%), over {files} go test runs")


def main(directory: str) -> int:
    if not directory or not os.path.isdir(directory):
        print("go-cache-hits: no action graph directory; nothing to report")
        return 0
    text = line(*count(directory))
    print(f"::notice title=Go build cache hits::{text}")
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as fh:
            fh.write(text + "\n")
    return 0


def self_test() -> int:
    failures = []
    with tempfile.TemporaryDirectory() as d:
        with open(os.path.join(d, "actiongraph.a"), "w") as fh:
            json.dump([{"Mode": "build", "Cmd": ["compile"]}, {"Mode": "build", "Cmd": None},
                       {"Mode": "build"}, {"Mode": "link", "Cmd": ["link"]}], fh)
        with open(os.path.join(d, "actiongraph.b"), "w") as fh:
            json.dump([{"Mode": "build", "Cmd": None}], fh)
        with open(os.path.join(d, "actiongraph.c"), "w") as fh:
            fh.write("")  # mktemp made it; the run failed before writing
        got = count(d)
        if got != (3, 4, 2):
            failures.append(f"three of four builds cached over two graphs, got {got}")
        if "3 of 4 package builds from the cache (75%), over 2 go test runs" not in line(*got):
            failures.append(f"report line: {line(*got)}")
    if line(0, 0, 0) != "Go build cache: no package builds recorded":
        failures.append("an empty directory reports no builds")
    if failures:
        print("go-cache-hits self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("go-cache-hits self-test: OK")
    return 0


if __name__ == "__main__":
    args = sys.argv[1:]
    if args == ["--self-test"]:
        sys.exit(self_test())
    sys.exit(main(args[0] if args else os.environ.get("GO_ACTIONGRAPH_DIR", "")))
