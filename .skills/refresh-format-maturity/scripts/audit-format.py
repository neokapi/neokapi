#!/usr/bin/env python3
"""First-pass, file-signal maturity audit for a single neokapi format.

This is the deterministic step of the refresh-format-maturity skill: it reports
what exists on disk, a coarse L0-L4 estimate, the Okapi counterpart (if any), and
the ready-to-run GitLab tracker query. It deliberately does NOT judge test
quality -- the skill's human/agent step reads the test assertions for the real
score.

Usage:
    python3 .skills/refresh-format-maturity/scripts/audit-format.py <format-id>

Env:
    OKAPI_SRC   path to the Okapi clone (default: ~/src/okapi/Okapi)
"""
from __future__ import annotations

import os
import re
import sys

# repo root = three levels up from this script (.skills/<skill>/scripts/)
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(SCRIPT_DIR, "..", "..", ".."))
OKAPI = os.environ.get("OKAPI_SRC", os.path.expanduser("~/src/okapi/Okapi"))
OKAPI_FILTERS = os.path.join(OKAPI, "okapi", "filters")
GITLAB_PROJECT = "62298414"  # okapiframework/Okapi

# neokapi id -> okapi filter directory, where it is not a direct name match.
OKAPI_ALIAS = {
    "csv": "table",
    "fixedwidth": "table",
    "xml": "xmlstream",
    "srt": "subtitles",
    "vtt": "subtitles",
    "ttml": "subtitles",
    "phpcontent": "php",
    "paraplaintext": "plaintext",
}
# Formats known to have NO Okapi counterpart (harvest cohort + a few natives).
KNOWN_HARVEST = {
    "androidxml", "applestrings", "arb", "designtokens",
    "i18next", "mdx", "resx", "xcstrings",
}
NOT_A_FORMAT = {"exec", "jsx", "memorytest"}


def has_file(d: str, name: str) -> bool:
    return os.path.isfile(os.path.join(d, name))


def test_kinds(d: str) -> list[str]:
    kinds = []
    probes = [
        "reader_test", "writer_test", "roundtrip_test", "skeleton_test",
        "spec_test", "invariants_test", "malformed_test", "acceptance_test",
        "corpus_test", "upstream_test", "subfilter_test", "config_test",
        "schema_test", "transform_test", "fuzz",
        "okapi_stubs_test", "okapi_skip_test", "okapi_test", "okapi_parity_test",
    ]
    files = os.listdir(d) if os.path.isdir(d) else []
    tests = [f for f in files if f.endswith("_test.go")]
    for p in probes:
        if any(p in t for t in tests):
            kinds.append(p.replace("_test", ""))
    return kinds


def applymap_rejects_unknown(cfg_path: str) -> str:
    if not os.path.isfile(cfg_path):
        return "no config.go"
    src = open(cfg_path, encoding="utf-8", errors="replace").read()
    if "DisallowUnknownFields" in src:
        return "yes (ApplyMapViaJSON / DisallowUnknownFields)"
    # a default branch in ApplyMap that errors on unknown keys
    if re.search(r"default:\s*\n\s*return\s+fmt\.Errorf\([^)]*unknown", src, re.I):
        return "yes (default branch errors)"
    if "unknown parameter" in src or "unknown key" in src:
        return "yes (unknown-key error present)"
    return "UNCLEAR -- read ApplyMap"


def okapi_counterpart(fmt: str) -> str:
    if fmt in KNOWN_HARVEST:
        return "none (harvest cohort)"
    if not os.path.isdir(OKAPI_FILTERS):
        return f"unknown (Okapi clone not found at {OKAPI}; set OKAPI_SRC)"
    dirs = {d for d in os.listdir(OKAPI_FILTERS)
            if os.path.isdir(os.path.join(OKAPI_FILTERS, d))}
    cand = OKAPI_ALIAS.get(fmt, fmt)
    if cand in dirs:
        note = f" (via alias '{cand}')" if cand != fmt else ""
        return f"okf_{cand}{note}"
    # loose substring match as a hint (skip for short ids -- too noisy)
    if len(fmt) >= 4:
        hits = [d for d in dirs if (fmt in d or d in fmt) and len(d) >= 4]
        if hits:
            return f"maybe: {', '.join(sorted(hits))} (verify)"
    return "none found (verify manually)"


def coarse_level(d: str, fmt: str, parity_test: bool, counterpart: str) -> str:
    kinds = test_kinds(d)
    has = lambda n: has_file(d, n)
    k = lambda n: n in kinds
    if not has("reader.go"):
        return "L0? (no reader.go -- stub/internal?)"
    # L1 floor
    l1 = has("writer.go") and has("config.go") and (k("roundtrip") or k("skeleton"))
    if not l1:
        return "L0 (missing writer/config or a fidelity test)"
    harvest = counterpart.startswith("none")
    # L2 floor
    if harvest:
        l2 = k("okapi_skip") and k("invariants") and k("corpus") and k("malformed")
    else:
        l2 = has_file(d, "spec.yaml") and k("spec") and k("malformed")
    if not l2:
        miss = []
        if harvest:
            for kind, ok in [("okapi_skip", k("okapi_skip")), ("invariants", k("invariants")),
                             ("corpus", k("corpus")), ("malformed", k("malformed"))]:
                if not ok: miss.append(kind)
        else:
            if not has_file(d, "spec.yaml"): miss.append("spec.yaml")
            if not k("spec"): miss.append("spec_test")
            if not k("malformed"): miss.append("malformed_test")
        return f"L1 (to L2 add: {', '.join(miss)})"
    # L3 floor (parity formats only)
    if harvest:
        return "L2+ (harvest ceiling; assess invariants/acceptance depth by hand)"
    l3 = parity_test and (k("corpus") or k("upstream"))
    if not l3:
        miss = []
        if not parity_test: miss.append("cli/parity/formats/%s_spec_test.go" % fmt)
        if not (k("corpus") or k("upstream")): miss.append("corpus/upstream_test")
        return f"L2 (to L3 add: {', '.join(miss)})"
    return "L3+ (assess edge-case matrix, schema_test, xfail hygiene for L4 by hand)"


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    fmt = sys.argv[1].strip().lstrip("/")
    d = os.path.join(REPO, "core", "formats", fmt)
    if not os.path.isdir(d):
        print(f"!! no such format dir: core/formats/{fmt}/")
        return 1
    if fmt in NOT_A_FORMAT:
        print(f"** {fmt} is not a real format (thin/internal) -- skip.")
        return 0

    parity_test = has_file(os.path.join(REPO, "cli", "parity", "formats"),
                           f"{fmt}_spec_test.go")
    bridge_cfg = has_file(os.path.join(REPO, "cli", "parity", "formats"),
                          f"{fmt}_bridge_config.go")
    counterpart = okapi_counterpart(fmt)
    annotations = has_file(d, "parity-annotations.yaml")

    print(f"=== format: {fmt} ===")
    print(f"dir            : core/formats/{fmt}/")
    print(f"reader.go      : {has_file(d, 'reader.go')}")
    print(f"writer.go      : {has_file(d, 'writer.go')}")
    print(f"config.go      : {has_file(d, 'config.go')}")
    print(f"  ApplyMap rejects unknown keys: {applymap_rejects_unknown(os.path.join(d, 'config.go'))}")
    print(f"schema.go      : {has_file(d, 'schema.go')}")
    print(f"transform.go   : {has_file(d, 'transform.go')}")
    print(f"spec.yaml      : {has_file(d, 'spec.yaml')}")
    print(f"testdata/      : {os.path.isdir(os.path.join(d, 'testdata'))}")
    print(f"test kinds     : {', '.join(test_kinds(d)) or '(none)'}")
    print(f"parity spec_test : {parity_test}  | bridge_config: {bridge_cfg}")
    print(f"parity-annotations.yaml : {annotations}")
    print(f"okapi counterpart : {counterpart}")
    print(f"coarse level   : {coarse_level(d, fmt, parity_test, counterpart)}")
    print()
    print("--- Okapi GitLab tracker (verify against pinned 1.48.0) ---")
    print(f'curl -s "https://gitlab.com/api/v4/projects/{GITLAB_PROJECT}/issues'
          f'?search={fmt}&state=all&per_page=50"')
    print(f"web: https://gitlab.com/okapiframework/Okapi/-/issues?search={fmt}")
    print()
    print(">> next: read the *_test.go assertions and score the 9 rubric "
          "dimensions (see docs/internals/format-maturity.md).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
