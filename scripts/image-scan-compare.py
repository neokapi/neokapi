#!/usr/bin/env python3
"""Compare a rebuilt image's scan with the scan of the image it replaces.

    scripts/image-scan-compare.py <candidate.json> <current.json>
    scripts/image-scan-compare.py --self-test

Both files are Trivy JSON reports of one image and platform, scanned with
the same severities and ignore file. A finding is a vulnerability id in a
package. The candidate is worse when it has a finding the current image does
not have; the rebuild then publishes nothing (exit 1). Findings it fixes are
listed too. A missing or unreadable current report counts as no findings, so
a candidate with any finding is worse.

Exit status: 0 not worse, 1 worse, 2 the candidate report cannot be read.
"""

from __future__ import annotations

import json
import sys


def findings(report: dict) -> set[tuple[str, str]]:
    out = set()
    for result in report.get("Results") or []:
        for v in result.get("Vulnerabilities") or []:
            out.add((v.get("VulnerabilityID", ""), v.get("PkgName", "")))
    return out


def load(path: str) -> dict | None:
    try:
        with open(path) as fh:
            return json.load(fh)
    except (OSError, ValueError):
        return None


def compare(candidate: dict, current: dict | None) -> tuple[bool, list[str], list[str]]:
    cand = findings(candidate)
    cur = findings(current or {})
    new = sorted(f"{i} in {p}" for i, p in cand - cur)
    fixed = sorted(f"{i} in {p}" for i, p in cur - cand)
    return bool(new), new, fixed


def main(argv: list[str]) -> int:
    if argv == ["--self-test"]:
        return self_test()
    if len(argv) != 2:
        print(__doc__)
        return 2
    candidate = load(argv[0])
    if candidate is None:
        print(f"image-scan-compare: cannot read the candidate report {argv[0]}")
        return 2
    current = load(argv[1])
    if current is None:
        print(f"image-scan-compare: no readable report for the current image ({argv[1]}); comparing against none")
    worse, new, fixed = compare(candidate, current)
    for f in fixed:
        print(f"fixed by the rebuild: {f}")
    for f in new:
        print(f"::error::new in the rebuild: {f}")
    print(f"image-scan-compare: {len(new)} new, {len(fixed)} fixed: {'WORSE, not published' if worse else 'not worse'}")
    return 1 if worse else 0


def self_test() -> int:
    def report(*pairs):
        return {"Results": [{"Target": "t", "Vulnerabilities": [{"VulnerabilityID": i, "PkgName": p} for i, p in pairs]}]}

    failures = []
    same = report(("CVE-1", "libxml2"))
    if compare(same, same)[0]:
        failures.append("an identical scan is not worse")
    worse, new, _ = compare(report(("CVE-1", "libxml2"), ("CVE-2", "openssl")), same)
    if not worse or new != ["CVE-2 in openssl"]:
        failures.append(f"a new finding is worse: {new}")
    worse, _, fixed = compare(report(), same)
    if worse or fixed != ["CVE-1 in libxml2"]:
        failures.append(f"a fixed finding is not worse: {fixed}")
    if not compare(report(("CVE-1", "libxml2")), None)[0]:
        failures.append("with no current report, any finding is worse")
    if compare(report(), None)[0]:
        failures.append("a clean candidate with no current report is not worse")
    worse, new, _ = compare(report(("CVE-1", "zlib")), same)
    if not worse or new != ["CVE-1 in zlib"]:
        failures.append("the same id in another package is a new finding")
    if failures:
        print("image-scan-compare self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("image-scan-compare self-test: OK")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
