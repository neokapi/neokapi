#!/usr/bin/env python3
"""First-pass, file-signal maturity audit for a single neokapi format.

This is the deterministic step of the refresh-format-maturity skill: it reports
what exists on disk, a coarse L0-L4 estimate, the Okapi counterpart (if any), and
the ready-to-run GitLab tracker query. It deliberately does NOT judge test
quality -- the skill's human/agent step reads the test assertions for the real
score.

Scorer v3: alongside the legacy engine fields (`base`/`ceiling`/`coarse_level`,
which are the repro-check stdin contract and stay byte-compatible), the JSON
output carries an additive `axes` block — one `{base, ceiling, signals}` entry
per axis (engine L0-L4, vocabulary V0-V3, editor E0-E4, knowledge K0-K3,
corpus C0-C3) — computed by parsing the axis artifacts the rubric names
(vocabulary.yaml, integrations.yaml, dossier.yaml, corpus.yaml, the nativedocs
sidecar, spec.yaml refs). Absent artifacts floor at the zero grade; they never
crash the audit. See docs/internals/format-maturity.md §2-§3 and §5.

Usage:
    python3 .skills/refresh-format-maturity/scripts/audit-format.py <format-id>
    python3 .skills/refresh-format-maturity/scripts/audit-format.py --all --json
    python3 ... --all --json --ledger docs/internals/format-ops-ledger.json

Flags:
    --json            machine output (2-space indented JSON)
    --all             every real format under core/formats/
    --ledger <path>   the format-ops ledger; unlocks the ledger-dependent
                      signals (mutation-check demotions, corpus external
                      verification, acceptance CI conclusions, citations /
                      context-pack results). Omitted or unreadable => those
                      signals report 'unknown' and floors stay conservative.

Env:
    OKAPI_SRC   path to the Okapi clone (default: ~/src/okapi/Okapi)
"""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys

try:  # optional; axis-artifact parsing degrades to zero floors without it
    import yaml as _yaml
except Exception:  # pragma: no cover
    _yaml = None

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

# v3 axis artifact locations
INTEGRATIONS_PATH = os.path.join(REPO, "core", "formats", "integrations.yaml")
EQUIV_TEST_PATH = os.path.join(REPO, "core", "formats", "vocab_equivalence_test.go")
NATIVEDOCS_DIR = os.path.join(REPO, "scripts", "gen-refs", "nativedocs", "formats")
NATIVEDOCS_TEMPLATE = os.path.join(REPO, "scripts", "gen-refs", "nativedocs", "_TEMPLATE.yaml")
FETCH_CORPUS_SH = os.path.join(REPO, "scripts", "fetch-corpus.sh")

AXIS_GRADES = {
    "engine": ["L0", "L1", "L2", "L3", "L4"],
    "vocabulary": ["V0", "V1", "V2", "V3"],
    "editor": ["E0", "E1", "E2", "E3", "E4"],
    "knowledge": ["K0", "K1", "K2", "K3"],
    "corpus": ["C0", "C1", "C2", "C3"],
    "security": ["S0", "S1", "S2", "S3", "S4"],
    "structure": ["G0", "G1", "G2", "G3", "G4"],
}

# Rank lookup for the structure (G) grades, for ceiling comparisons.
RANK_STRUCT = {g: i for i, g in enumerate(AXIS_GRADES["structure"])}

# Consume-only boundaries: a reader but no native writer. Generalizes the old
# hardcoded 'pdf' so docling — and any future consume-only / OCR ingestion
# boundary — also gets the read-only `na` writer/writecells patch
# (SHARPEN-PROPOSAL §4 applicability). `image` is NOT here: it ships a writer
# (bytes + alt-text sidecar) and is the binary-asset boundary, not consume-only.
READ_ONLY_FORMATS = {"pdf", "docling"}
VOCAB_STATUSES = {"lossless", "lossy", "dropped", "rejected"}
CANONICAL_TYPE_RE = re.compile(r'"(fmt|link|media|code):')


def has_file(d: str, name: str) -> bool:
    return os.path.isfile(os.path.join(d, name))


def test_kinds(d: str) -> list[str]:
    kinds = []
    probes = [
        "reader_test", "writer_test", "roundtrip_test", "skeleton_test",
        "snippets_test",
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


def coarse_level(d: str, fmt: str, parity_test: bool, counterpart: str,
                 ftype: str = "parity") -> str:
    kinds = test_kinds(d)
    has = lambda n: has_file(d, n)
    k = lambda n: n in kinds
    if not has("reader.go"):
        return "L0? (no reader.go -- stub/internal?)"
    # L1 floor. Read->write fidelity is proven by a conventionally-named
    # roundtrip/skeleton test OR, equivalently, by a head-to-head Okapi parity
    # test or an upstream real-file round-trip (the kinds markdown/idml/odf use
    # in place of roundtrip_test — the grandfathered set in maturity_test.go).
    if ftype == "read-only":
        # No writer to round-trip with; fidelity is extraction correctness,
        # proven by a reader/spec/real-file test.
        l1 = has("config.go") and (k("reader") or k("spec") or k("corpus") or k("upstream"))
    else:
        fidelity = (k("roundtrip") or k("skeleton") or k("snippets")
                    or k("okapi_parity") or k("okapi") or k("upstream"))
        l1 = has("writer.go") and has("config.go") and fidelity
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
    # L3 met. The L4 ceiling requires schema_test (rubric §2.1 L4: "schema_test
    # asserts schema == config struct") — without it the ceiling stays L3, so a
    # gate that would optimistically grant L4 is clamped back. The edge-case
    # matrix + xfail hygiene remain a by-hand call within [L3, L4].
    if k("schema"):
        return "L3+ (assess edge-case matrix, xfail hygiene for L4 by hand)"
    return "L3 (to L4 add: schema_test + edge-case matrix)"


def floor_ceiling(coarse: str) -> tuple[str, str]:
    """Map the human coarse_level string to (deterministic base, promotable ceiling).

    `base` is the level the on-disk files alone guarantee; `ceiling` is the
    highest a by-hand/LLM judgment may promote to. A scorer must keep the
    published level within [base, ceiling] — judgment can only decide the
    boundary the files cannot (harvest L2->L3, parity L3->L4), never invent a
    tier whose required files are absent.
    """
    if coarse.startswith("L0"):
        return "L0", "L0"
    if coarse.startswith("L1"):
        return "L1", "L1"
    if coarse.startswith("L2+"):
        return "L2", "L3"  # harvest ceiling: L3 reachable by the self-contained ladder
    if coarse.startswith("L2"):
        return "L2", "L2"
    if coarse.startswith("L3+"):
        return "L3", "L4"  # parity L3 met + schema_test present; L4 is a by-hand quality call
    if coarse.startswith("L3"):
        return "L3", "L3"  # L3 met but no schema_test => L4 ceiling not reachable
    return "L0", "L4"


def _ftype(fmt: str, counterpart: str) -> str:
    if fmt in READ_ONLY_FORMATS:
        return "read-only"
    if fmt == "splicedlines":
        return "internal"
    return "harvest" if counterpart.startswith("none") else "parity"


# ──────────────────────── v3 axis machinery ────────────────────────

def _read_text(path: str) -> str:
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            return f.read()
    except OSError:
        return ""


def _load_yaml(path: str):
    """Parse a YAML artifact; absence or any parse problem => None (zero floor)."""
    if _yaml is None or not os.path.isfile(path):
        return None
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            return _yaml.safe_load(f)
    except Exception:
        return None


def _git(args: list[str]) -> str:
    try:
        out = subprocess.run(["git", *args], cwd=REPO, capture_output=True,
                             text=True, timeout=30)
        return out.stdout.strip() if out.returncode == 0 else ""
    except Exception:
        return ""


def _grade_min(axis: str, a: str, b: str) -> str:
    order = AXIS_GRADES[axis]
    try:
        return a if order.index(a) <= order.index(b) else b
    except ValueError:
        return order[0]


_CACHE: dict = {}


def _structure_rules_json_text() -> str:
    """Concatenated text of any exported STRUCTURE_RULES index json (none today)."""
    if "structure_rules" not in _CACHE:
        files = _git(["grep", "-l", "--fixed-strings", "STRUCTURE_RULES", "--", "*.json"])
        text = ""
        for rel in files.splitlines():
            text += _read_text(os.path.join(REPO, rel))
        _CACHE["structure_rules"] = text
    return _CACHE["structure_rules"]


def _integrations_doc():
    if "integrations" not in _CACHE:
        _CACHE["integrations"] = (_load_yaml(INTEGRATIONS_PATH)
                                  if os.path.isfile(INTEGRATIONS_PATH) else None)
        _CACHE["integrations_present"] = os.path.isfile(INTEGRATIONS_PATH)
    return _CACHE["integrations"], _CACHE["integrations_present"]


def _ledger_check(ledger, fmt: str, needles: tuple[str, ...]) -> str:
    """Latest runs[].evidence entry whose check names this format + needle.

    green = exit 0, red = nonzero; 'unknown' when never recorded (or no ledger).
    """
    if not isinstance(ledger, dict):
        return "unknown"
    res = "unknown"
    for r in ledger.get("runs", []) or []:
        if not isinstance(r, dict):
            continue
        for e in r.get("evidence", []) or []:
            if not isinstance(e, dict):
                continue
            chk = str(e.get("check", "")).lower()
            if fmt.lower() in chk and any(n in chk for n in needles):
                res = "green" if e.get("exit", 1) == 0 else "red"
    return res


# ── engine axis ──

def _mutation_unchecked(d: str, fmt: str, ledger) -> list[str] | None:
    """Robustness test files introduced by a remediation-run commit that lack
    mutation-check evidence in that run (rubric §3). None => no ledger given."""
    if not isinstance(ledger, dict):
        return None
    runs = [r for r in ledger.get("runs", []) or []
            if isinstance(r, dict) and r.get("ritual") == "remediate" and r.get("commit")]
    if not runs:
        return []
    by_commit: dict[str, list] = {}
    for r in runs:
        by_commit.setdefault(str(r["commit"]), []).append(r)
    out = []
    files = os.listdir(d) if os.path.isdir(d) else []
    for name in sorted(files):
        if not name.endswith("_test.go"):
            continue
        if "malformed" not in name and "fuzz" not in name:
            continue  # the rule covers the robustness floor cells
        rel = f"core/formats/{fmt}/{name}"
        intro = _git(["log", "-1", "--diff-filter=A", "--format=%H", "--", rel])
        if intro not in by_commit:
            continue
        checked = False
        for r in by_commit[intro]:
            for e in r.get("evidence", []) or []:
                chk = str(e.get("check", "")).lower() if isinstance(e, dict) else ""
                if "mutation" in chk and (fmt.lower() in chk or name.lower() in chk):
                    checked = True
        if not checked:
            out.append(name)
    return out


def _engine_axis(d: str, fmt: str, has: dict, kinds: list[str], ftype: str,
                 base: str, ceiling: str, ledger) -> dict:
    k = lambda n: n in kinds
    sig = {
        "reader": "complete" if has["reader"] else "none",
        "writer": "complete" if has["writer"] else ("na" if ftype == "read-only" else "none"),
        "config": "complete" if has["config"] else "none",
        "spec": ("na" if ftype == "harvest"
                 else "complete" if has["spec_yaml"] and k("spec")
                 else "partial" if has["spec_yaml"] else "none"),
        "parity": ("na" if ftype == "harvest"
                   else "complete" if has["parity_spec_test"] else "none"),
        "malformed": "complete" if k("malformed") else "none",
        "corpus": ("complete" if (k("corpus") or k("upstream"))
                   else "partial" if has["testdata"] else "none"),
        "detection": "complete",  # constant, v2 compatibility (rubric §2.1)
        "docs": "complete",       # constant; real docs signals live on Knowledge
    }
    unchecked = _mutation_unchecked(d, fmt, ledger)
    if unchecked is None:
        sig["mutation_check"] = "unknown"
    else:
        sig["mutation_check"] = "clean" if not unchecked else "unchecked"
        if unchecked:
            sig["mutation_unchecked"] = unchecked
            if sig["malformed"] == "complete" and any("malformed" in n for n in unchecked):
                sig["malformed"] = "partial"  # rubric §3: partial until mutation-checked
    return {"base": base, "ceiling": ceiling, "signals": sig}


# ── vocabulary axis ──

def _evidence_list(v) -> list[str]:
    if v is None:
        return []
    if isinstance(v, str):
        return [v]
    if isinstance(v, list):
        return [str(x) for x in v if x is not None]
    if isinstance(v, dict):
        out = []
        for x in v.values():
            out.extend(_evidence_list(x))
        return out
    return [str(v)]


def _resolve_evidence(ev: str, fmt: str, spec_text: str) -> bool:
    """pkg.TestFunc must grep to 'func TestFunc' in that package's *_test.go;
    a spec-case id must appear in this format's spec.yaml. Else: unresolved."""
    ev = ev.strip()
    if not ev:
        return False
    m = re.match(r"^([A-Za-z][A-Za-z0-9_]*)\.((?:Test|Fuzz|Benchmark|Example)\w+)$", ev)
    if m:
        pkg, fn = m.group(1), m.group(2)
        pdir = os.path.join(REPO, "core", "formats", pkg)
        if not os.path.isdir(pdir):
            return False
        pat = re.compile(r"func\s+" + re.escape(fn) + r"\b")
        for name in os.listdir(pdir):
            if name.endswith("_test.go") and pat.search(_read_text(os.path.join(pdir, name))):
                return True
        return False
    # path/file_test.go:TestName form (same shape the editor probe accepts)
    pm = re.match(r"^([\w./-]+_test\.go):(\w+)$", ev)
    if pm:
        path = os.path.join(REPO, pm.group(1))
        return (os.path.isfile(path)
                and re.search(r"func\s+" + re.escape(pm.group(2)) + r"\b", _read_text(path)) is not None)
    # spec-case id
    return bool(spec_text) and len(ev) >= 3 and ev in spec_text


def _vocab_rows(doc) -> list[dict]:
    if doc is None:
        return []
    if isinstance(doc, dict):
        body = doc.get("constructs", doc)
        if isinstance(body, list):
            return [r for r in body if isinstance(r, dict)]
        if isinstance(body, dict):
            rows = []
            for key, val in body.items():
                if isinstance(val, dict):
                    rows.append({"construct": key, **val})
            return rows
        return []
    if isinstance(doc, list):
        return [r for r in doc if isinstance(r, dict)]
    return []


def _cell(raw, row_evidence: list[str]) -> tuple[str | None, list[str]]:
    if isinstance(raw, dict):
        status = raw.get("status") or raw.get("value") or raw.get("state")
        ev = _evidence_list(raw.get("evidence")) or row_evidence
    else:
        status, ev = raw, row_evidence
    if isinstance(status, str):
        status = status.strip().lower()
    elif status is not None:
        status = str(status).lower()
    return status, ev


def _vocabulary_axis(d: str, fmt: str, ftype: str) -> dict:
    vpath = os.path.join(d, "vocabulary.yaml")
    present = os.path.isfile(vpath)
    doc = _load_yaml(vpath)
    parseable = doc is not None
    rows = _vocab_rows(doc)
    spec_text = _read_text(os.path.join(d, "spec.yaml"))

    # vocabtypes: package-wide grep over non-test .go files
    vocabtypes = False
    if os.path.isdir(d):
        for name in os.listdir(d):
            if not name.endswith(".go") or name.endswith("_test.go"):
                continue
            src = _read_text(os.path.join(d, name))
            if CANONICAL_TYPE_RE.search(src) or "core/model/vocabularies" in src:
                vocabtypes = True
                break

    # equivalence: a case in core/formats/vocab_equivalence_test.go (absent => none)
    equivalence = (os.path.isfile(EQUIV_TEST_PATH)
                   and f'"{fmt}"' in _read_text(EQUIV_TEST_PATH))

    expressible = 0
    read_claimed = read_resolved = 0
    write_claimed = write_resolved = 0
    unknown_read = unknown_write = 0
    unresolved = 0
    for row in rows:
        if row.get("expressible", True) is False:
            continue
        expressible += 1
        row_ev = _evidence_list(row.get("evidence"))
        for side in ("read", "write"):
            status, ev = _cell(row.get(side), row_ev)
            claimed = status in VOCAB_STATUSES
            resolved = claimed and any(_resolve_evidence(e, fmt, spec_text) for e in ev)
            if claimed and not resolved:
                unresolved += 1  # unresolved evidence => the cell counts as unknown
            if side == "read":
                read_claimed += claimed
                read_resolved += resolved
                unknown_read += not resolved
            else:
                write_claimed += claimed
                write_resolved += resolved
                unknown_write += not resolved

    read_ok = present and parseable and (expressible == 0 or unknown_read == 0)
    write_ok = (ftype == "read-only") or (write_claimed > 0 and unknown_write == 0)
    v1 = read_ok and vocabtypes
    v2 = v1 and expressible > 0 and write_ok and equivalence
    base = "V2" if v2 else "V1" if v1 else "V0"
    if present and parseable and expressible == 0:
        base = _grade_min("vocabulary", base, "V1")  # no-construct formats cap at V1
        ceiling = base
    else:
        unknown_total = unknown_read + (0 if ftype == "read-only" else unknown_write)
        ceiling = "V3" if (base == "V2" and unknown_total == 0) else base
    sig = {
        "vocabmap": ("missing" if not present
                     else "unparseable" if not parseable else "present"),
        "cells": {
            "constructs": len(rows), "expressible": expressible,
            "read_claimed": read_claimed, "read_resolved": read_resolved,
            "write_claimed": write_claimed, "write_resolved": write_resolved,
            "unknown_read": unknown_read, "unknown_write": unknown_write,
            "unresolved_evidence": unresolved,
        },
        "vocabtypes": vocabtypes,
        "equivalence": equivalence,
    }
    return {"base": base, "ceiling": ceiling, "signals": sig}


# ── editor axis ──

def _integration_entries(fmt: str) -> tuple[list[dict] | None, bool]:
    doc, file_present = _integrations_doc()
    if doc is None:
        return None, file_present
    entry = None
    if isinstance(doc, dict):
        body = doc.get("integrations") or doc.get("formats") or doc
        if isinstance(body, dict):
            entry = body.get(fmt)
    elif isinstance(doc, list):
        entry = [x for x in doc if isinstance(x, dict)
                 and (x.get("format") == fmt or x.get("id") == fmt)]
    if entry is None:
        return [], file_present
    if isinstance(entry, dict):
        return [entry], file_present
    if isinstance(entry, list):
        return [x for x in entry if isinstance(x, dict)], file_present
    return [], file_present


def _editor_axis(d: str, fmt: str) -> dict:
    # E1 probe: PreviewBuilder impl in the package, or id in an exported
    # STRUCTURE_RULES index json (no such json exists today => Go probe only).
    preview = False
    if has_file(d, "preview.go"):
        for name in os.listdir(d):
            if name.endswith(".go") and not name.endswith("_test.go"):
                if "PreviewBuilder" in _read_text(os.path.join(d, name)):
                    preview = True
                    break
    if not preview and f'"{fmt}"' in _structure_rules_json_text():
        preview = True

    entries, file_present = _integration_entries(fmt)
    declared, e2, e3, e4 = "E0", False, False, False
    if entries:
        for entry in entries:
            dep = str(entry.get("depth") or entry.get("declared") or entry.get("level") or "").strip()
            if dep in AXIS_GRADES["editor"]:
                if AXIS_GRADES["editor"].index(dep) > AXIS_GRADES["editor"].index(declared):
                    declared = dep
            for ev in _evidence_list(entry.get("evidence") or entry.get("gate_evidence")):
                pm = re.match(r"^([\w./-]+\.go):(\w+)$", ev.strip())
                if pm:
                    path = os.path.join(REPO, pm.group(1))
                    if os.path.isfile(path) and re.search(
                            r"func\s+" + re.escape(pm.group(2)) + r"\b", _read_text(path)):
                        e2 = True
            man = entry.get("manifest") or entry.get("manifest_path")
            if isinstance(man, str) and os.path.isfile(os.path.join(REPO, man)):
                e3 = True
            handler = entry.get("handler") or entry.get("handler_symbol")
            if isinstance(handler, str) and handler.strip():
                hits = _git(["grep", "-l", "--fixed-strings", handler.strip(), "--",
                             ".", ":(exclude)core/formats/integrations.yaml"])
                if hits:
                    e4 = True

    # cumulative probe ladder: a missing lower rung caps the probed level
    probed = "E0"
    for nxt, ok in (("E1", preview), ("E2", e2), ("E3", e3), ("E4", e4)):
        if not ok:
            break
        probed = nxt
    if entries:
        base = _grade_min("editor", declared, probed)  # floor = min(declared, probed)
    else:
        base = "E1" if preview else "E0"  # no entry => E0/E1 band from the preview probe
    sig = {
        "integrations": ("missing" if not file_present
                         else "no-entry" if not entries else f"{len(entries)} entry(ies)"),
        "declared": declared if entries else None,
        "probes": {"preview": preview, "roundtrip_test": e2,
                   "manifest": e3, "handler": e4},
        "probed": probed,
    }
    return {"base": base, "ceiling": base, "signals": sig}  # floor-only axis


# ── knowledge axis ──

def _spec_refs_census(spec_path: str) -> dict:
    out = {"spec_refs": 0, "okapi_refs": 0, "native_refs": 0,
           "expected_fail": 0, "divergence_kind": 0}
    if not os.path.isfile(spec_path):
        return out
    doc = _load_yaml(spec_path)
    if doc is not None:
        def walk(node):
            if isinstance(node, dict):
                for key in ("spec_refs", "okapi_refs", "native_refs"):
                    if key in node:
                        v = node[key]
                        out[key] += len(v) if isinstance(v, list) else 1
                if "expected_fail" in node:
                    out["expected_fail"] += 1
                    if node.get("divergence_kind"):
                        out["divergence_kind"] += 1
                for v in node.values():
                    walk(v)
            elif isinstance(node, list):
                for v in node:
                    walk(v)
        walk(doc)
        return out
    # fallback: comment-stripped line counting (yaml missing/unparseable)
    for line in _read_text(spec_path).splitlines():
        s = line.strip()
        if s.startswith("#"):
            continue
        for key in out:
            if s.startswith(key + ":"):
                out[key] += 1
    return out


def _knowledge_axis(d: str, fmt: str, has: dict, kinds: list[str],
                    ftype: str, ledger) -> dict:
    dpath = os.path.join(d, "dossier.yaml")
    dossier_present = os.path.isfile(dpath)
    doc = _load_yaml(dpath)
    sources = []
    implementations = False
    if isinstance(doc, dict):
        for key in ("spec_sources", "sources", "specs"):
            v = doc.get(key)
            if isinstance(v, list):
                sources = [s for s in v if isinstance(s, dict)]
                break
        implementations = bool(doc.get("implementations") or doc.get("other_implementations"))
    valid_sources = [s for s in sources
                     if all(s.get(k) for k in ("id", "version", "url"))]
    dossier_ok = len(valid_sources) >= 1

    sidecar_path = os.path.join(NATIVEDOCS_DIR, f"{fmt}.yaml")
    sidecar_present = os.path.isfile(sidecar_path)
    if sidecar_present and os.path.isfile(NATIVEDOCS_TEMPLATE):
        sidecar_ok = _read_text(sidecar_path).strip() != _read_text(NATIVEDOCS_TEMPLATE).strip()
    else:
        sidecar_ok = sidecar_present  # no template to diff against => exists-check

    refs = _spec_refs_census(os.path.join(d, "spec.yaml"))
    coverage = (refs["divergence_kind"] / refs["expected_fail"]) if refs["expected_fail"] else 1.0
    k = lambda n: n in kinds
    refs_ok = (has["spec_yaml"] and refs["spec_refs"] > 0 and refs["native_refs"] > 0
               and (refs["okapi_refs"] > 0 or ftype != "parity") and coverage == 1.0)
    if not refs_ok and ftype == "harvest":
        # harvest ladder stands in for spec.yaml refs (rubric K2 "or the harvest ladder")
        refs_ok = k("okapi_skip") and k("invariants") and k("corpus")

    citations = _ledger_check(ledger, fmt, ("citation",))
    contextpack = _ledger_check(ledger, fmt, ("context-pack", "contextpack", "context_pack"))

    k1 = dossier_ok and sidecar_ok
    k2 = k1 and has["schema"] and refs_ok
    base = "K2" if k2 else "K1" if k1 else "K0"
    ceiling = "K3" if (base == "K2" and citations == "green" and contextpack == "green") else base
    sig = {
        "dossier": ("missing" if not dossier_present
                    else "unparseable" if doc is None else "present"),
        "spec_sources": {"total": len(sources), "valid": len(valid_sources)},
        "implementations": implementations,
        "sidecar": ("ok" if sidecar_ok
                    else "template" if sidecar_present else "missing"),
        "refs": {**refs, "divergence_coverage": round(coverage, 3)},
        "citations": citations,
        "contextpack": contextpack,
    }
    return {"base": base, "ceiling": ceiling, "signals": sig}


# ── corpus axis ──

def _corpus_entries(doc) -> list[dict]:
    if doc is None:
        return []
    if isinstance(doc, list):
        return [e for e in doc if isinstance(e, dict)]
    if isinstance(doc, dict):
        for key in ("entries", "files", "corpus"):
            v = doc.get(key)
            if isinstance(v, list):
                return [e for e in v if isinstance(e, dict)]
    return []


def _testdata_files(d: str, fmt: str) -> list[str]:
    root = os.path.join(d, "testdata")
    out = []
    for dirpath, _dirs, files in os.walk(root):
        for name in files:
            if name == ".DS_Store":
                continue
            rel = os.path.relpath(os.path.join(dirpath, name), REPO)
            out.append(rel.replace(os.sep, "/"))
    return sorted(out)


def _corpus_axis(d: str, fmt: str, kinds: list[str], ledger) -> dict:
    cpath = os.path.join(d, "corpus.yaml")
    present = os.path.isfile(cpath)
    doc = _load_yaml(cpath)
    entries = _corpus_entries(doc)
    tiers: dict[str, int] = {}
    origins: dict[str, int] = {}
    paths = set()
    for e in entries:
        tiers[str(e.get("tier", "?"))] = tiers.get(str(e.get("tier", "?")), 0) + 1
        origins[str(e.get("origin", "?"))] = origins.get(str(e.get("origin", "?")), 0) + 1
        if isinstance(e.get("path"), str):
            paths.add(e["path"].strip().replace(os.sep, "/"))

    testdata = _testdata_files(d, fmt)
    missing = [p for p in testdata if p not in paths]
    coverage_ok = present and doc is not None and not missing

    spec_text = _read_text(os.path.join(d, "spec.yaml"))
    scheme = re.search(r"corpus:[A-Za-z0-9_./-]", spec_text) is not None
    fetch_script = os.path.isfile(FETCH_CORPUS_SH)
    fetchwired = scheme and fetch_script
    countersigned = bool(isinstance(doc, dict)
                         and (doc.get("reviewed_by")
                              or (isinstance(doc.get("na"), dict)
                                  and doc["na"].get("reviewed_by"))))

    # ledger-only signals
    acceptance_ci = _ledger_check(ledger, fmt, ("acceptance",))
    has_acceptance_test = "acceptance" in kinds
    acceptance = ("complete" if has_acceptance_test and acceptance_ci == "green"
                  else "partial" if has_acceptance_test else "none")
    sweep = "unknown"
    wild_verified = 0
    if isinstance(ledger, dict):
        rituals = ledger.get("rituals", {}) or {}
        counts = (((rituals.get("corpus-sweep") or {}).get("watermarks") or {})
                  .get("per_format_counts") or {})
        if fmt in counts:
            sweep = counts[fmt]
        extv = (rituals.get("corpus-census") or {}).get("external_verification") or {}
        for e in entries:
            if str(e.get("origin")) in ("url", "archive-member"):
                rec = extv.get(str(e.get("path", "")))
                if isinstance(rec, dict) and rec.get("ok"):
                    wild_verified += 1

    c1 = coverage_ok
    c2 = c1 and (fetchwired or countersigned)
    base = "C2" if c2 else "C1" if c1 else "C0"
    # C3 is reachable only with ledger-verified wild entries; without a ledger
    # the ceiling stays at C2 (and never above the base band the files support).
    ceiling = "C3" if (base == "C2" and wild_verified > 0) else base
    sig = {
        "corpusmanifest": ("missing" if not present
                           else "unparseable" if doc is None else "present"),
        "census": {"entries": len(entries), "tiers": tiers, "origins": origins,
                   "testdata_files": len(testdata), "uncovered": len(missing)},
        "corpus": origins,  # shared engine+corpus quality-dim seed (origin census)
        "fetchwiring": {"scheme_in_spec": scheme, "fetch_script": fetch_script,
                        "wired": fetchwired},
        "countersigned_na": countersigned,
        "acceptance": acceptance,
        "acceptance_ci": acceptance_ci,
        "sweep": sweep,
        "externally_verified_wild": wild_verified if isinstance(ledger, dict) else "unknown",
    }
    return {"base": base, "ceiling": ceiling, "signals": sig}


# ── security axis (S0–S4) ──
# A pure floor ladder, no quality dimensions (rubric §2.6). Signals are
# deterministic file facts plus two ledger-driven ceiling rungs:
#   S1 bounded  — the package imports core/safeio (boundedness is structural).
#   S2 fuzzed   — a Fuzz* target AND ≥1 testdata/fuzz seed exist for the format.
#   S3 hostile-hardened — S2 + a CLEAN corpus-sweep record in the ledger
#                 (0 CRASH/HANG/OOM/ROUNDTRIP_DRIFT); absent ledger ⇒ ceiling S2.
#   S4 continuously-assured — S3 + a sustained ledger signal (ceiling only;
#                 reachable later). govulncheck-clean is module-wide — a noted
#                 co-signal, not a per-format gate.
# `base` is the structural file floor (S0/S1/S2 only); the ledger rungs raise
# the `ceiling` exactly like Knowledge/Corpus, and the format-triage gate then
# computes the published level from the cells and caps it at the ceiling.

def _has_fuzz_seed(d: str) -> bool:
    """≥1 seed file under testdata/fuzz/<FuzzName>/ (Go native fuzz corpus)."""
    root = os.path.join(d, "testdata", "fuzz")
    if not os.path.isdir(root):
        return False
    for _dirpath, _dirs, files in os.walk(root):
        if any(name != ".DS_Store" for name in files):
            return True
    return False


def _sweep_counts(ledger, fmt: str):
    """The corpus-sweep per-format classification counts, or 'unknown'."""
    if not isinstance(ledger, dict):
        return "unknown"
    counts = ((((ledger.get("rituals", {}) or {}).get("corpus-sweep") or {})
               .get("watermarks") or {}).get("per_format_counts") or {})
    return counts[fmt] if fmt in counts else "unknown"


def _sweepclean_cell(sweep) -> str:
    """complete = recorded with 0 CRASH/HANG/OOM/ROUNDTRIP_DRIFT; partial =
    recorded but dirty; none = no record (mirrors the corpus sweep cell)."""
    if sweep is None or sweep == "unknown":
        return "none"
    if isinstance(sweep, dict):
        bad = ("CRASH", "HANG", "OOM", "ROUNDTRIP_DRIFT")
        return "partial" if any(int(sweep.get(k, 0) or 0) > 0 for k in bad) else "complete"
    return "partial"  # recorded but in an unrecognized shape


def _security_sustained(ledger, fmt: str) -> bool:
    """S4 ceiling: a ledger-recorded sustained green-sweep / batch-fuzz streak
    (≥30 days) for the format. Accepts a `sustained_formats` list or a
    `sustained` map of {fmt: {green_days}} under the corpus-sweep watermarks."""
    if not isinstance(ledger, dict):
        return False
    wm = ((((ledger.get("rituals", {}) or {}).get("corpus-sweep") or {})
           .get("watermarks")) or {})
    lst = wm.get("sustained_formats")
    if isinstance(lst, list) and fmt in lst:
        return True
    streaks = wm.get("sustained")
    rec = streaks.get(fmt) if isinstance(streaks, dict) else None
    if isinstance(rec, dict):
        return int(rec.get("green_days", 0) or 0) >= 30
    if isinstance(rec, (int, float)) and not isinstance(rec, bool):
        return rec >= 30
    return False


def _security_axis(d: str, fmt: str, ledger) -> dict:
    # S1 signal: the package imports core/safeio in a non-test .go file.
    safeio = False
    if os.path.isdir(d):
        for name in sorted(os.listdir(d)):
            if name.endswith(".go") and not name.endswith("_test.go"):
                if "core/safeio" in _read_text(os.path.join(d, name)):
                    safeio = True
                    break
    # S2 signal: a Fuzz* target AND ≥1 testdata/fuzz seed for the format.
    fuzz_target = False
    if os.path.isdir(d):
        for name in sorted(os.listdir(d)):
            if name.endswith("_test.go") and re.search(
                    r"func\s+Fuzz\w+", _read_text(os.path.join(d, name))):
                fuzz_target = True
                break
    fuzz_seed = _has_fuzz_seed(d)
    fuzzed = fuzz_target and fuzz_seed
    # S3 / S4 signals come from the ledger (ceiling rungs).
    sweep = _sweep_counts(ledger, fmt)
    sweepclean = _sweepclean_cell(sweep)
    sustained = _security_sustained(ledger, fmt)

    cells = {
        "safeio": "complete" if safeio else "none",
        "fuzz": "complete" if fuzzed else "none",
        "sweepclean": sweepclean,
        "sustained": "complete" if sustained else "none",
    }
    full = lambda c: c == "complete"
    # structural file floor — never above what the files alone guarantee (S2).
    if not full(cells["safeio"]):
        base = "S0"
    elif not full(cells["fuzz"]):
        base = "S1"
    else:
        base = "S2"
    # ledger-enhanced ceiling (cumulative): a clean sweep unlocks S3, a
    # sustained signal unlocks S4. No ledger ⇒ ceiling stays at the file floor.
    ceiling = base
    if base == "S2" and full(cells["sweepclean"]):
        ceiling = "S3"
        if full(cells["sustained"]):
            ceiling = "S4"
    sig = {
        "safeio": safeio,
        "fuzz_target": fuzz_target,
        "fuzz_seed": fuzz_seed,
        "fuzzed": fuzzed,
        "sweep": sweep,
        "sustained": sustained,
        "cells": cells,
    }
    return {"base": base, "ceiling": ceiling, "signals": sig}


# ── structure & geometry axis (G0–G4) ──
# A pure floor ladder, no quality dimensions (rubric §2.7) — the comprehension-
# depth ladder the vision/structure stack populates. Signals are deterministic
# file greps over the non-test .go in the format package, mirroring the
# Vocabulary `vocabtypes` / Security `safeio` greps:
#   G1 metaplane     — SetLayoutLayer(...LayerMetadata), an import of
#                      core/docmeta, or AddRelation(...RelCaptionOf): metadata-
#                      plane / alt-text/caption text recovered.
#   G2 readingorder  — emits model.PartGroupStart and/or sets
#                      StructureAnnotation.ReadingOrder (SetStructure): body text
#                      recovered grouped / in reading order.
#   G3 roles         — SetSemanticRole / SetStructure AND a table Group
#                      (Type:"table"/"table-row" / RoleTableCell|Header) and/or
#                      AddRelation, PLUS a roles/structure test.
#   G4 geometry      — SetGeometry PLUS a geometry test.
# The ladder is CUMULATIVE: a deeper payload entails the shallower body-text/plane
# rungs, so the cells are DOWN-FILLED (roles ⇒ metaplane+readingorder; geometry ⇒
# metaplane+readingorder but NOT roles — geometry-without-roles is "positioned
# text we don't understand", capped at G2: odf/idml). `base` is then the
# cumulative gate over the filled cells.
#
# na geometry (non-spatial catalogs with no intrinsic geometry) is a CEILING cap,
# NOT a gate-pass (rubric §5 decision 6): the gate-visible geometry cell stays
# 'none' (so the gate caps at G3 anyway) and the per-format `ceiling` records that
# G4 is unreachable. structure.yaml, if present, declares the AD-028 authority
# tier (native|tagged|geometric|ml) and may countersign the na geometry cell.

_STRUCT_ROLE_TEST_RE = re.compile(
    r"func\s+Test\w*(Role|Structure|Heading|Table|ReadingOrder)")
_STRUCT_GEOM_TEST_RE = re.compile(r"func\s+Test\w*(Geometr|BBox|Glyph)")
_STRUCT_TABLE_RE = re.compile(r'Type:\s*"table(-row)?"')
_STRUCT_SPAN_TEST_RE = re.compile(r"func\s+Test\w*(Span|ColSpan|RowSpan)")
# Canonical form-cluster roles / state conventions (core/model/structure.go).
_STRUCT_FORMS_RE = re.compile(
    r"\b(RoleKey|RoleValue|RoleHint|RoleFieldItem|RoleFieldHeading|"
    r"RoleFieldRegion|RoleCheckbox|PropFieldFillable|PropCheckboxChecked)\b")


def _structure_axis(d: str, fmt: str, ftype: str, ledger=None) -> dict:
    metaplane = readingorder = roles_setter = table_or_rel = geometry_setter = False
    roles_test = geometry_test = False
    span_setter = span_test = forms_setter = False
    if os.path.isdir(d):
        for name in sorted(os.listdir(d)):
            if not name.endswith(".go"):
                continue
            src = _read_text(os.path.join(d, name))
            if name.endswith("_test.go"):
                if _STRUCT_ROLE_TEST_RE.search(src):
                    roles_test = True
                if _STRUCT_GEOM_TEST_RE.search(src):
                    geometry_test = True
                if _STRUCT_SPAN_TEST_RE.search(src) or "ColSpan" in src or "RowSpan" in src:
                    span_test = True
                continue
            if (re.search(r"SetLayoutLayer\([^)]*LayerMetadata", src)
                    or "core/docmeta" in src
                    or re.search(r"AddRelation\([^,]*RelCaptionOf", src)):
                metaplane = True
            if ("PartGroupStart" in src or "GroupStart{" in src
                    or re.search(r"\.ReadingOrder\b", src)
                    or "SetStructure(" in src):
                readingorder = True
            if "SetSemanticRole(" in src or "SetStructure(" in src:
                roles_setter = True
            if (_STRUCT_TABLE_RE.search(src) or "RoleTableCell" in src
                    or "RoleTableHeader" in src or "AddRelation(" in src):
                table_or_rel = True
            if "SetGeometry(" in src:
                geometry_setter = True
            if "ColSpan" in src or "RowSpan" in src:
                span_setter = True
            if _STRUCT_FORMS_RE.search(src):
                forms_setter = True

    # Sub-signals (display only, not gated): G3 span fidelity (a reader that
    # populates ColSpan/RowSpan AND proves it) and forms recovery (canonical
    # key/value/hint/checkbox roles + fillable/checked state). They refine what a
    # G3 grade means — G3 certifies table TOPOLOGY, not span/forms fidelity.
    span_fidelity = span_setter and span_test
    forms = forms_setter

    # rung completeness per the floor-signals table
    meta_c = metaplane
    order_c = readingorder
    roles_c = roles_setter and table_or_rel and roles_test
    geom_c = geometry_setter and geometry_test

    # CUMULATIVE down-fill: a deeper standoff payload implies the shallower
    # body-text/plane rungs. Geometry fills body-text/plane (positioned text) but
    # NOT roles — that asymmetry is what caps geometry-without-roles at G2.
    if roles_c:
        meta_c = order_c = True
    if geom_c:
        meta_c = order_c = True
    if order_c:
        meta_c = True

    cells = {
        "metaplane": "complete" if meta_c else "none",
        "readingorder": "complete" if order_c else "none",
        "roles": "complete" if roles_c else "none",
        "geometry": "complete" if geom_c else "none",
    }
    full = lambda c: c == "complete"
    if not full(cells["metaplane"]):
        base = "G0"
    elif not full(cells["readingorder"]):
        base = "G1"
    elif not full(cells["roles"]):
        base = "G2"
    elif not full(cells["geometry"]):
        base = "G3"
    else:
        base = "G4"

    # structure.yaml: authority tier + countersigned na geometry cell + a
    # PLUGIN-PROVIDED ceiling (pdf/image, whose real depth lives out-of-core).
    authority = None
    geometry_na = False
    plugin_declared_ceiling = None
    sdoc = _load_yaml(os.path.join(d, "structure.yaml"))
    if isinstance(sdoc, dict):
        authority = sdoc.get("authority") or sdoc.get("tier")
        geo = sdoc.get("geometry")
        if isinstance(geo, dict):
            if str(geo.get("status", "")).lower() == "na" and geo.get("reviewed_by"):
                geometry_na = True
        elif str(geo).lower() == "na":
            geometry_na = True
        # A `plugin:` block means the depth is realized out-of-core; its declared
        # ceiling is the promotable target the nightly plugin job certifies.
        if isinstance(sdoc.get("plugin"), dict):
            dc = str(sdoc.get("ceiling", "")).upper()
            if dc in RANK_STRUCT:
                plugin_declared_ceiling = dc

    # ceiling: floor-only ⇒ base. na-as-ceiling — a non-spatial format (no
    # geometry capability, or a countersigned na) cannot climb to G4, so the
    # ceiling caps below G4. Mirrors the per-axis ceiling mechanism, NOT the
    # gate's full('na')==pass: base is already ≤ G3 when geometry is absent.
    ceiling = base
    spatial = geom_c or geometry_setter
    if geometry_na or not spatial:
        if RANK_STRUCT[ceiling] > RANK_STRUCT["G3"]:
            ceiling = "G3"

    # Plugin-provided G certification (rubric §2.7; issue #916). pdf/image publish
    # the conservative IN-CORE grep floor until a NIGHTLY plugin job (kapi-pdfium /
    # vision-onnx) records a green structure-certification in the ledger — exactly
    # like the Corpus acceptance signal. Without that signal the declared plugin
    # ceiling is only the promotable ceiling (dashboard shows floor→ceiling); with
    # it, the published cells (and base) rise to the certified plugin ceiling.
    plugin_certified = False
    if plugin_declared_ceiling and RANK_STRUCT[plugin_declared_ceiling] > RANK_STRUCT[ceiling]:
        ceiling = plugin_declared_ceiling  # declared/promotable ceiling
        if _ledger_check(ledger, fmt, ("structure",)) == "green":
            plugin_certified = True
            # Fill the published cells up to the certified plugin ceiling so the
            # gate (gateStructure over signals.cells) publishes it.
            want = RANK_STRUCT[plugin_declared_ceiling]
            cells["metaplane"] = "complete"
            cells["readingorder"] = "complete"
            cells["roles"] = "complete" if want >= RANK_STRUCT["G3"] else cells["roles"]
            cells["geometry"] = "complete" if want >= RANK_STRUCT["G4"] else cells["geometry"]
            base = plugin_declared_ceiling

    sig = {
        "metaplane": metaplane,
        "readingorder": readingorder,
        "roles_setter": roles_setter,
        "table_or_rel": table_or_rel,
        "roles_test": roles_test,
        "geometry_setter": geometry_setter,
        "geometry_test": geometry_test,
        "span_fidelity": span_fidelity,
        "forms": forms,
        "authority": authority,
        "geometry_na": geometry_na,
        "plugin_ceiling": plugin_declared_ceiling,
        "plugin_certified": plugin_certified,
        "cells": cells,
    }
    return {"base": base, "ceiling": ceiling, "signals": sig}


def _axes_block(d: str, fmt: str, has: dict, kinds: list[str], ftype: str,
                base: str, ceiling: str, ledger) -> dict:
    return {
        "engine": _engine_axis(d, fmt, has, kinds, ftype, base, ceiling, ledger),
        "vocabulary": _vocabulary_axis(d, fmt, ftype),
        "editor": _editor_axis(d, fmt),
        "knowledge": _knowledge_axis(d, fmt, has, kinds, ftype, ledger),
        "corpus": _corpus_axis(d, fmt, kinds, ledger),
        "security": _security_axis(d, fmt, ledger),
        "structure": _structure_axis(d, fmt, ftype, ledger),
    }


def _axis_hints(fmt: str, axes: dict) -> list[str]:
    """Per-axis missing-artifact hints for human mode."""
    hints = []
    v = axes["vocabulary"]["signals"]
    if axes["vocabulary"]["base"] != "V3":
        miss = []
        if v["vocabmap"] != "present":
            miss.append(f"core/formats/{fmt}/vocabulary.yaml ({v['vocabmap']})")
        else:
            if v["cells"]["unresolved_evidence"]:
                miss.append(f"{v['cells']['unresolved_evidence']} unresolved evidence cell(s)")
            if v["cells"]["unknown_read"] or v["cells"]["unknown_write"]:
                miss.append(f"unknown cells r{v['cells']['unknown_read']}/w{v['cells']['unknown_write']}")
        if not v["vocabtypes"]:
            miss.append("no canonical-type literals in package")
        if not v["equivalence"]:
            miss.append("no vocab_equivalence_test case")
        if miss:
            hints.append("vocab     : " + "; ".join(miss))
    e = axes["editor"]["signals"]
    if axes["editor"]["base"] != "E4":
        miss = []
        if e["integrations"] in ("missing", "no-entry"):
            miss.append(f"integrations.yaml {e['integrations']}")
        if not e["probes"]["preview"]:
            miss.append("no PreviewBuilder/preview.go")
        if e.get("declared") and e["declared"] != e["probed"]:
            miss.append(f"declared {e['declared']} vs probed {e['probed']}")
        if miss:
            hints.append("editor    : " + "; ".join(miss))
    kn = axes["knowledge"]["signals"]
    if axes["knowledge"]["base"] != "K3":
        miss = []
        if kn["dossier"] != "present":
            miss.append(f"dossier.yaml {kn['dossier']}")
        elif kn["spec_sources"]["valid"] == 0:
            miss.append("no spec source with id+version+url")
        if kn["sidecar"] != "ok":
            miss.append(f"nativedocs sidecar {kn['sidecar']}")
        if kn["refs"]["divergence_coverage"] < 1.0:
            miss.append(f"divergence_kind coverage {kn['refs']['divergence_coverage']}")
        if kn["citations"] == "unknown" or kn["contextpack"] == "unknown":
            miss.append("citations/context-pack unknown (pass --ledger)")
        if miss:
            hints.append("knowledge : " + "; ".join(miss))
    c = axes["corpus"]["signals"]
    if axes["corpus"]["base"] != "C3":
        miss = []
        if c["corpusmanifest"] != "present":
            miss.append(f"corpus.yaml {c['corpusmanifest']}")
        elif c["census"]["uncovered"]:
            miss.append(f"{c['census']['uncovered']} uncovered testdata file(s)")
        if not c["fetchwiring"]["wired"] and not c["countersigned_na"]:
            miss.append("no Tier B fetch wiring / countersigned na")
        if c["acceptance_ci"] == "unknown" or c["sweep"] == "unknown":
            miss.append("acceptance/sweep unknown (pass --ledger)")
        if miss:
            hints.append("corpus    : " + "; ".join(miss))
    s = axes["security"]["signals"]
    if axes["security"]["ceiling"] != "S4":
        miss = []
        if not s["safeio"]:
            miss.append("reader does not import core/safeio (S1)")
        elif not s["fuzzed"]:
            miss.append("no Fuzz* target + testdata/fuzz seed (S2)")
        elif s["cells"]["sweepclean"] != "complete":
            miss.append("no clean corpus-sweep record (S3; pass --ledger)")
        elif not s["sustained"]:
            miss.append("no sustained sweep/batch-fuzz signal (S4)")
        if miss:
            hints.append("security  : " + "; ".join(miss))
    st = axes["structure"]["signals"]
    if axes["structure"]["base"] != axes["structure"]["ceiling"] or axes["structure"]["base"] != "G4":
        miss = []
        if st["cells"]["metaplane"] != "complete":
            miss.append("no metadata-plane / docmeta / caption block (G1)")
        elif st["cells"]["readingorder"] != "complete":
            miss.append("no group emission / reading-order (G2)")
        elif st["cells"]["roles"] != "complete":
            need = []
            if not st["roles_setter"]:
                need.append("SetSemanticRole/SetStructure")
            if not st["table_or_rel"]:
                need.append("table Group / AddRelation")
            if not st["roles_test"]:
                need.append("a roles/structure test")
            miss.append("no logical structure (G3): " + ", ".join(need or ["roles"]))
        elif st["cells"]["geometry"] != "complete":
            if st["geometry_na"] or axes["structure"]["ceiling"] != "G4":
                miss.append("geometry N/A (non-spatial) — capped below G4")
            else:
                need = []
                if not st["geometry_setter"]:
                    need.append("SetGeometry")
                if not st["geometry_test"]:
                    need.append("a geometry test")
                miss.append("no spatial geometry (G4): " + ", ".join(need or ["geometry"]))
        if miss:
            hints.append("structure : " + "; ".join(miss))
    return hints


def _axes_summary(axes: dict) -> str:
    def band(name):
        a = axes[name]
        return f"{a['base']}..{a['ceiling']}"
    return (f"engine {band('engine')} | vocab {band('vocabulary')} | "
            f"editor {band('editor')} | knowledge {band('knowledge')} | "
            f"corpus {band('corpus')} | security {band('security')} | "
            f"structure {band('structure')}")


def audit_one(fmt: str, ledger=None) -> dict:
    """Deterministic file-signal audit for one format as a machine contract."""
    d = os.path.join(REPO, "core", "formats", fmt)
    counterpart = okapi_counterpart(fmt)
    parity_test = has_file(os.path.join(REPO, "cli", "parity", "formats"), f"{fmt}_spec_test.go")
    ftype = _ftype(fmt, counterpart)
    coarse = coarse_level(d, fmt, parity_test, counterpart, ftype)
    base, ceiling = floor_ceiling(coarse)
    has = {
        "reader": has_file(d, "reader.go"),
        "writer": has_file(d, "writer.go"),
        "config": has_file(d, "config.go"),
        "schema": has_file(d, "schema.go"),
        "spec_yaml": has_file(d, "spec.yaml"),
        "transform": has_file(d, "transform.go"),
        "testdata": os.path.isdir(os.path.join(d, "testdata")),
        "parity_spec_test": parity_test,
        "annotations": has_file(d, "parity-annotations.yaml"),
    }
    kinds = test_kinds(d)
    return {
        "format": fmt,
        "type": ftype,
        "okapi_counterpart": counterpart,
        "has": has,
        "applymap_rejects_unknown": applymap_rejects_unknown(os.path.join(d, "config.go")),
        "test_kinds": kinds,
        "coarse_level": coarse,
        "base": base,
        "ceiling": ceiling,
        # v3 additive multi-axis floors; engine.base == top-level base always.
        # Per-axis harvest ceilings: only engine carries the harvest/parity cap.
        "axes": _axes_block(d, fmt, has, kinds, ftype, base, ceiling, ledger),
    }


def all_formats() -> list[str]:
    """The reporting universe: a dir that ships a reader.go (an in-core format)
    OR a structure.yaml (the dashboard allowlist seat for a PLUGIN-PROVIDED,
    out-of-core format such as pdf — no in-core reader.go, read by the
    kapi-pdfium plugin / a PDFium-wasm bridge). Mirrors realFormatDirs in
    core/formats/maturity_test.go and scripts/format-ops/lib.mjs so the audit,
    the Go maturity gates, and the JS format-ops gates agree on the universe."""
    base = os.path.join(REPO, "core", "formats")
    out = []
    for name in sorted(os.listdir(base)):
        d = os.path.join(base, name)
        if not os.path.isdir(d) or name in NOT_A_FORMAT:
            continue
        if not (has_file(d, "reader.go") or has_file(d, "structure.yaml")):
            continue
        out.append(name)
    return out


def _load_ledger(argv: list[str]) -> tuple[object, list[str]]:
    """Extract --ledger <path> (or --ledger=path); returns (ledger|None, rest)."""
    ledger = None
    rest = []
    i = 0
    while i < len(argv):
        a = argv[i]
        if a == "--ledger" and i + 1 < len(argv):
            path = argv[i + 1]
            i += 2
        elif a.startswith("--ledger="):
            path = a.split("=", 1)[1]
            i += 1
        else:
            rest.append(a)
            i += 1
            continue
        try:
            with open(path, encoding="utf-8") as f:
                ledger = json.load(f)
        except Exception as exc:
            print(f"!! --ledger {path}: unreadable ({exc}); "
                  "ledger-dependent signals stay 'unknown'", file=sys.stderr)
            ledger = None
    return ledger, rest


def main() -> int:
    ledger, argv = _load_ledger(sys.argv[1:])
    json_mode = "--json" in argv
    all_mode = "--all" in argv
    pos = [a for a in argv if not a.startswith("-")]

    if all_mode:
        data = [audit_one(f, ledger) for f in all_formats()]
        if json_mode:
            print(json.dumps(data, indent=2))
        else:
            for x in data:
                ax = x["axes"]
                compact = " ".join(ax[a]["base"] for a in
                                   ("vocabulary", "editor", "knowledge",
                                    "corpus", "security", "structure"))
                print(f"{x['format']}: {x['coarse_level']}  [{compact}]")
        return 0

    if len(pos) != 1:
        print(__doc__)
        return 2
    fmt = pos[0].strip().lstrip("/")
    d = os.path.join(REPO, "core", "formats", fmt)
    if not os.path.isdir(d):
        print(f"!! no such format dir: core/formats/{fmt}/")
        return 1
    if fmt in NOT_A_FORMAT:
        print(f"** {fmt} is not a real format (thin/internal) -- skip.")
        return 0
    if json_mode:
        print(json.dumps(audit_one(fmt, ledger), indent=2))
        return 0

    parity_test = has_file(os.path.join(REPO, "cli", "parity", "formats"),
                           f"{fmt}_spec_test.go")
    bridge_cfg = has_file(os.path.join(REPO, "cli", "parity", "formats"),
                          f"{fmt}_bridge_config.go")
    counterpart = okapi_counterpart(fmt)
    annotations = has_file(d, "parity-annotations.yaml")
    audit = audit_one(fmt, ledger)

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
    print(f"coarse level   : {coarse_level(d, fmt, parity_test, counterpart, _ftype(fmt, counterpart))}")
    print(f"axes           : {_axes_summary(audit['axes'])}")
    for hint in _axis_hints(fmt, audit["axes"]):
        print(f"  {hint}")
    print()
    print("--- Okapi GitLab tracker (verify against pinned 1.48.0) ---")
    print(f'curl -s "https://gitlab.com/api/v4/projects/{GITLAB_PROJECT}/issues'
          f'?search={fmt}&state=all&per_page=50"')
    print(f"web: https://gitlab.com/okapiframework/Okapi/-/issues?search={fmt}")
    print()
    print(">> next: read the *_test.go assertions and score the per-axis rubric "
          "dimensions (see docs/internals/format-maturity.md).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
