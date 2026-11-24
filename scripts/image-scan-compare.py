#!/usr/bin/env python3
"""Decide whether a rebuilt image may replace the image `latest` points at.

    scripts/image-scan-compare.py compare <candidate.json> --image-id <sha256:...>
                                  (--current <current.json> --current-ref <image@digest>
                                   | --no-current)
    scripts/image-scan-compare.py image-id <oci-layout-dir> <os/arch>
    scripts/image-scan-compare.py --self-test

The reports are Trivy JSON, scanned with --list-all-pkgs, the same severities
and the same ignore file. Trivy's image scan runs the vulnerability and
secret scanners; both are compared. Misconfiguration scanning is not run on
images, so there is nothing of it to compare.

A report counts as evidence only when it names the scanner version, has at
least one result with a package inventory, and is tied to its image: the
candidate's image id is the config digest of the layout about to be
published, and the current report scanned the exact digest `latest` pointed
at (--current-ref, its ArtifactName). Anything less
is an error, never "no worse": a scan that inspected nothing proves nothing.

A finding is (target, class, identifier, package, installed version), so a
finding that moves to another layer or version is a new finding. The
candidate is worse when it has a finding the current image does not have.
With --no-current (no image is published under `latest` yet), the candidate
must have no finding at all, the threshold container-scan.yml applies.

`image-id` prints the config digest of an OCI layout's image for one
platform, for --image-id.

Exit status: 0 publish, 1 worse, 2 usage, 3 the candidate report is not
evidence, 4 the current report is not evidence.
"""

from __future__ import annotations

import json
import sys

USAGE, CANDIDATE_INVALID, CURRENT_INVALID = 2, 3, 4


def evidence_problems(report: object) -> list[str]:
    """Why a report does not prove a scan happened; empty when it does."""
    if not isinstance(report, dict):
        return ["the report is not a JSON object"]
    problems = []
    trivy = report.get("Trivy") or {}
    if not (isinstance(trivy, dict) and trivy.get("Version")):
        problems.append("no scanner version (Trivy.Version)")
    results = report.get("Results") or []
    if not any(r.get("Packages") for r in results if isinstance(r, dict)):
        problems.append("no result with a package inventory (scan with --list-all-pkgs)")
    if not (report.get("Metadata") or {}).get("ImageID"):
        problems.append("no image id (Metadata.ImageID)")
    return problems


def findings(report: dict) -> set[tuple[str, str, str, str, str]]:
    out = set()
    for r in report.get("Results") or []:
        target = r.get("Target", "")
        for v in r.get("Vulnerabilities") or []:
            out.add((target, "vulnerability", v.get("VulnerabilityID", ""), v.get("PkgName", ""), v.get("InstalledVersion", "")))
        for s in r.get("Secrets") or []:
            out.add((target, "secret", s.get("RuleID", ""), s.get("Category", ""), str(s.get("StartLine", ""))))
    return out


def describe(f: tuple) -> str:
    target, kind, ident, pkg, version = f
    return f"{kind} {ident} in {pkg} {version} ({target})"


def decide(candidate: object, image_id: str, current: object | None, no_current: bool,
           current_ref: str = "") -> tuple[int, list[str]]:
    """Return (exit status, lines to print)."""
    lines = []
    problems = evidence_problems(candidate)
    if not problems and candidate["Metadata"]["ImageID"] != image_id:
        problems.append(f"its image id {candidate['Metadata']['ImageID']} is not the archive's {image_id}")
    if problems:
        return CANDIDATE_INVALID, [f"::error::the candidate report is not evidence: {p}" for p in problems]
    cand = findings(candidate)
    if no_current:
        lines.append("no image is published under latest yet: the candidate must have no finding")
        for f in sorted(cand):
            lines.append(f"::error::finding in a first publish: {describe(f)}")
        return (1 if cand else 0), lines
    problems = evidence_problems(current)
    if not problems and current.get("ArtifactName") != current_ref:
        problems.append(f"it scanned {current.get('ArtifactName')!r}, not {current_ref!r}")
    if problems:
        return CURRENT_INVALID, [f"::error::the current image's report is not evidence: {p}" for p in problems]
    cur = findings(current)
    for f in sorted(cur - cand):
        lines.append(f"fixed by the rebuild: {describe(f)}")
    new = sorted(cand - cur)
    for f in new:
        lines.append(f"::error::new in the rebuild: {describe(f)}")
    lines.append(f"{len(new)} new, {len(cur - cand)} fixed: {'WORSE, not published' if new else 'not worse'}")
    return (1 if new else 0), lines


def oci_image_id(layout: str, platform: str) -> str:
    """The config digest of the OCI layout's image for os/arch."""
    import os

    want_os, want_arch = platform.split("/", 1)

    def blob(digest: str) -> dict:
        algo, hexd = digest.split(":", 1)
        with open(os.path.join(layout, "blobs", algo, hexd)) as fh:
            return json.load(fh)

    with open(os.path.join(layout, "index.json")) as fh:
        stack = [json.load(fh)]
    while stack:
        index = stack.pop()
        for m in index.get("manifests") or []:
            p = m.get("platform") or {}
            media = m.get("mediaType", "")
            if "index" in media or "manifest.list" in media:
                stack.append(blob(m["digest"]))
            elif p.get("os") == want_os and p.get("architecture") == want_arch:
                return blob(m["digest"])["config"]["digest"]
    raise SystemExit(f"image-scan-compare: no {platform} image in {layout}")


def load(path: str) -> object:
    try:
        with open(path) as fh:
            return json.load(fh)
    except (OSError, ValueError) as err:
        return {"_unreadable": str(err)}


def main(argv: list[str]) -> int:
    if argv == ["--self-test"]:
        return self_test()
    if len(argv) == 3 and argv[0] == "image-id":
        print(oci_image_id(argv[1], argv[2]))
        return 0
    if len(argv) >= 4 and argv[0] == "compare":
        args = argv[2:]
        opts = {}
        while args:
            flag = args.pop(0)
            if flag == "--no-current":
                opts["no_current"] = True
            elif flag in ("--image-id", "--current", "--current-ref") and args:
                opts[flag] = args.pop(0)
            else:
                print(__doc__)
                return USAGE
        if "--image-id" not in opts or ("--current" in opts) == bool(opts.get("no_current")) \
                or ("--current" in opts) != ("--current-ref" in opts):
            print(__doc__)
            return USAGE
        current = load(opts["--current"]) if "--current" in opts else None
        status, lines = decide(load(argv[1]), opts["--image-id"], current, bool(opts.get("no_current")),
                               opts.get("--current-ref", ""))
        for line in lines:
            print(line)
        return status
    print(__doc__)
    return USAGE


def self_test() -> int:
    image = "sha256:aaa"

    ref = "ghcr.io/neokapi/x@sha256:ccc"

    def report(*vulns, packages=True, image_id=image, version="0.76.2", artifact=ref):
        return {
            "ArtifactName": artifact,
            "Trivy": {"Version": version} if version else {},
            "Metadata": {"ImageID": image_id} if image_id else {},
            "Results": [{
                "Target": "os",
                "Packages": [{"Name": "libxml2"}] if packages else [],
                "Vulnerabilities": [{"VulnerabilityID": i, "PkgName": p, "InstalledVersion": v} for i, p, v in vulns],
            }],
        }

    base = report(("CVE-1", "libxml2", "2.13.9-r2"))
    cases = [
        ("empty candidate", {"Results": []}, base, False, CANDIDATE_INVALID),
        ("candidate with no package inventory", report(packages=False), base, False, CANDIDATE_INVALID),
        ("candidate with no scanner version", report(version=""), base, False, CANDIDATE_INVALID),
        ("digest mismatch", report(image_id="sha256:bbb"), base, False, CANDIDATE_INVALID),
        ("packages and no findings", report(), base, False, 0),
        ("one more finding", report(("CVE-1", "libxml2", "2.13.9-r2"), ("CVE-2", "openssl", "3.5.0")), base, False, 1),
        ("one fewer finding", report(), base, False, 0),
        ("same finding, installed version changed", report(("CVE-1", "libxml2", "2.13.9-r3")), base, False, 1),
        ("same findings", report(("CVE-1", "libxml2", "2.13.9-r2")), base, False, 0),
        ("current report empty", report(), {"Results": []}, False, CURRENT_INVALID),
        ("current report unreadable", report(), {"_unreadable": "x"}, False, CURRENT_INVALID),
        ("current report of another digest", report(), report(artifact="ghcr.io/neokapi/x:latest"), False, CURRENT_INVALID),
        ("missing baseline, clean candidate", report(), None, True, 0),
        ("missing baseline, candidate with a finding", report(("CVE-1", "libxml2", "2.13.9-r2")), None, True, 1),
    ]
    failures = []
    for name, cand, cur, no_current, want in cases:
        got, lines = decide(cand, image, cur, no_current, "" if no_current else ref)
        if got != want:
            failures.append(f"{name}: exit {got}, want {want} ({lines})")
    secret = report()
    secret["Results"][0]["Secrets"] = [{"RuleID": "aws-access-key-id", "Category": "AWS", "StartLine": 3}]
    if decide(secret, image, report(), False, ref)[0] != 1:
        failures.append("a new secret is worse")
    if failures:
        print("image-scan-compare self-test: FAIL")
        for f in failures:
            print("  " + f)
        return 1
    print("image-scan-compare self-test: OK")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
