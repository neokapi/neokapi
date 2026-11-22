#!/usr/bin/env python3
"""Print the reliability checklist items whose triggers match a diff.

The checklist is docs/internals/reliability-checklist.yaml. Each item names a
trigger that can be read from a diff (changed paths, added lines), a check to
perform, and the incident behind it. This script reads the diff between two
refs, selects the items whose trigger matches, and prints each one with the
reason it matched.

    scripts/reliability-checklist.py                      # merge base with origin/main..working tree
    scripts/reliability-checklist.py --base <ref> --head <ref>
    scripts/reliability-checklist.py --all                # every item
    scripts/reliability-checklist.py --self-test          # parser, matcher and fixtures

Exit status: 0 after printing (the selection informs; it does not gate), 2 when
the checklist is malformed, 1 when the self-test fails, 3 when git cannot
produce the diff.

The standard library has no YAML reader, so the checklist is parsed by a small
subset parser: block mappings, block lists, plain and quoted scalars, folded and
literal block scalars, and full-line comments. Anything outside that subset is
an error with a line number, never a silent misread.
"""

from __future__ import annotations

import argparse
import os
import re
import signal
import subprocess
import sys
import textwrap
from dataclasses import dataclass, field

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CHECKLIST = os.path.join(ROOT, "docs", "internals", "reliability-checklist.yaml")

SURFACES = (
    "framework", "cli", "server", "stores", "desktop", "web",
    "mobile", "formats", "ci", "release", "docs",
)
ITEM_KEYS = {"id", "title", "trigger", "check", "incident", "owner_surface"}
TRIGGER_KEYS = {"paths", "added", "added_in", "exclude", "note"}
ID_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")

# The checklist and this script quote every trigger as an example, so their
# own lines would select most items. They are left out of every diff.
SELF_PATHS = (
    "docs/internals/reliability-checklist.yaml",
    "docs/internals/reliability-checklist.md",
    "scripts/reliability-checklist.py",
)

# Untracked files larger than this are listed as changed paths, but their
# lines are not read as added lines.
MAX_UNTRACKED_BYTES = 512 * 1024


class ChecklistError(Exception):
    """The checklist file is malformed."""


# ── YAML subset parser ──────────────────────────────────────────────────────


@dataclass
class _Line:
    num: int
    indent: int
    text: str  # content with the indentation removed


def _logical_lines(src: str) -> list[_Line]:
    out = []
    for num, raw in enumerate(src.splitlines(), start=1):
        stripped = raw.lstrip(" ")
        if raw[: len(raw) - len(stripped)].count("\t") or stripped.startswith("\t"):
            raise ChecklistError(f"line {num}: tab in indentation")
        out.append(_Line(num, len(raw) - len(stripped), stripped.rstrip()))
    return out


def _is_blank_or_comment(line: _Line) -> bool:
    return line.text == "" or line.text.startswith("#")


class _Parser:
    def __init__(self, src: str):
        self.lines = _logical_lines(src)
        self.i = 0

    def _skip(self) -> None:
        while self.i < len(self.lines) and _is_blank_or_comment(self.lines[self.i]):
            self.i += 1

    def _peek(self) -> _Line | None:
        self._skip()
        return self.lines[self.i] if self.i < len(self.lines) else None

    def parse(self):
        first = self._peek()
        if first is None:
            raise ChecklistError("empty document")
        if first.indent != 0:
            raise ChecklistError(f"line {first.num}: document must start at column 0")
        value = self._node(0)
        rest = self._peek()
        if rest is not None:
            raise ChecklistError(f"line {rest.num}: unexpected content")
        return value

    def _node(self, indent: int):
        line = self._peek()
        if line is None or line.indent < indent:
            raise ChecklistError("expected a value")
        if line.text.startswith("- ") or line.text == "-":
            return self._list(line.indent)
        return self._mapping(line.indent)

    def _list(self, indent: int) -> list:
        items = []
        while True:
            line = self._peek()
            if line is None or line.indent < indent:
                return items
            if line.indent > indent:
                raise ChecklistError(f"line {line.num}: unexpected indentation")
            if not (line.text.startswith("- ") or line.text == "-"):
                return items
            rest = line.text[1:].lstrip(" ")
            if rest == "":
                self.i += 1
                items.append(self._node(indent + 1))
                continue
            inner = indent + (len(line.text) - len(rest))
            if _split_key(rest) is not None:
                # "- key: value" opens a mapping whose keys sit at `inner`.
                self.lines[self.i] = _Line(line.num, inner, rest)
                items.append(self._mapping(inner))
            else:
                self.i += 1
                items.append(self._scalar(rest, line, inner))

    def _mapping(self, indent: int) -> dict:
        out: dict = {}
        while True:
            line = self._peek()
            if line is None or line.indent < indent:
                return out
            if line.indent > indent:
                raise ChecklistError(f"line {line.num}: unexpected indentation")
            if line.text.startswith("- "):
                return out
            kv = _split_key(line.text)
            if kv is None:
                raise ChecklistError(f"line {line.num}: expected 'key: value'")
            key, rest = kv
            if key in out:
                raise ChecklistError(f"line {line.num}: duplicate key {key!r}")
            self.i += 1
            if rest == "":
                nxt = self._peek()
                if nxt is None or nxt.indent < indent or (
                    nxt.indent == indent and not nxt.text.startswith("- ")
                ):
                    raise ChecklistError(f"line {line.num}: {key!r} has no value")
                if nxt.indent == indent:
                    out[key] = self._list(indent)
                else:
                    out[key] = self._node(nxt.indent)
            else:
                out[key] = self._scalar(rest, line, indent + 1)

    def _scalar(self, text: str, line: _Line, block_indent: int):
        if text in (">", ">-", "|", "|-"):
            return self._block_scalar(text, block_indent)
        if text.startswith("'"):
            if len(text) < 2 or not text.endswith("'"):
                raise ChecklistError(f"line {line.num}: unterminated single-quoted string")
            body = text[1:-1]
            if re.search(r"(?<!')'(?!')", body.replace("''", "")):
                raise ChecklistError(f"line {line.num}: stray quote in single-quoted string")
            return body.replace("''", "'")
        if text.startswith('"'):
            if len(text) < 2 or not text.endswith('"'):
                raise ChecklistError(f"line {line.num}: unterminated double-quoted string")
            body = text[1:-1]
            return re.sub(r'\\(["\\])', r"\1", body)
        if text.startswith(("[", "{", "&", "*", "!", "%", "@", "`")):
            raise ChecklistError(
                f"line {line.num}: flow collections, anchors, tags and aliases are not "
                "supported; quote the value"
            )
        if " #" in text:
            text = text[: text.index(" #")].rstrip()
        return text

    def _block_scalar(self, style: str, min_indent: int) -> str:
        body: list[str] = []
        block_indent = None
        while self.i < len(self.lines):
            line = self.lines[self.i]
            if line.text == "":
                body.append("")
                self.i += 1
                continue
            if block_indent is None:
                if line.indent < min_indent:
                    break
                block_indent = line.indent
            if line.indent < block_indent:
                break
            body.append(" " * (line.indent - block_indent) + line.text)
            self.i += 1
        while body and body[-1] == "":
            body.pop()
        if block_indent is None:
            raise ChecklistError("empty block scalar")
        if style.startswith("|"):
            return "\n".join(body)
        # Folded: lines join with a space; a blank line is a paragraph break.
        paras, cur = [], []
        for b in body:
            if b == "":
                if cur:
                    paras.append(" ".join(cur))
                    cur = []
            else:
                cur.append(b.strip())
        if cur:
            paras.append(" ".join(cur))
        return "\n\n".join(paras)


_KEY_RE = re.compile(r"^([A-Za-z_][A-Za-z0-9_]*):(?:\s+(.*))?$")


def _split_key(text: str):
    m = _KEY_RE.match(text)
    if not m:
        return None
    return m.group(1), (m.group(2) or "").strip()


def parse_yaml_subset(src: str):
    return _Parser(src).parse()


# ── Checklist model ─────────────────────────────────────────────────────────


def glob_to_regex(glob: str) -> re.Pattern:
    """Translate a path glob: ** spans directories, * and ? stay in one segment."""
    i, out = 0, []
    while i < len(glob):
        c = glob[i]
        if glob.startswith("**/", i):
            out.append("(?:.*/)?")
            i += 3
        elif glob.startswith("**", i):
            out.append(".*")
            i += 2
        elif c == "*":
            out.append("[^/]*")
            i += 1
        elif c == "?":
            out.append("[^/]")
            i += 1
        elif c == "{":
            end = glob.find("}", i)
            if end < 0:
                raise ChecklistError(f"unclosed brace in glob {glob!r}")
            alts = glob[i + 1 : end].split(",")
            out.append("(?:" + "|".join(re.escape(a) for a in alts) + ")")
            i = end + 1
        else:
            out.append(re.escape(c))
            i += 1
    return re.compile("^" + "".join(out) + "$")


@dataclass
class Item:
    id: str
    title: str
    owner_surface: str
    check: str
    incident: str
    paths: list = field(default_factory=list)  # (glob, regex)
    added: list = field(default_factory=list)  # (source, regex)
    added_in: list = field(default_factory=list)
    exclude: list = field(default_factory=list)
    note: str = ""

    @staticmethod
    def _any(globs, path):
        return any(rx.match(path) for _, rx in globs)

    def match(self, diff: "Diff", limit: int = 3) -> list[str]:
        """Return up to `limit` reasons this item matches, or [] for no match."""
        reasons: list[str] = []
        for path in diff.paths:
            if self._any(self.exclude, path):
                continue
            for glob, rx in self.paths:
                if rx.match(path):
                    reasons.append(f"changed path {path} (paths: {glob})")
                    break
            if len(reasons) >= limit:
                return reasons
        if self.added:
            for path, lineno, text in diff.added:
                if self._any(self.exclude, path):
                    continue
                if self.added_in and not self._any(self.added_in, path):
                    continue
                for src, rx in self.added:
                    if rx.search(text):
                        shown = text.strip()
                        if len(shown) > 110:
                            shown = shown[:107] + "..."
                        reasons.append(f"added line {path}:{lineno}: {shown}")
                        break
                if len(reasons) >= limit:
                    return reasons
        return reasons


def _str_field(raw: dict, key: str, where: str) -> str:
    val = raw.get(key)
    if not isinstance(val, str) or not val.strip():
        raise ChecklistError(f"{where}: {key!r} must be a non-empty string")
    return val.strip()


def _list_field(raw: dict, key: str, where: str) -> list:
    val = raw.get(key, [])
    if not isinstance(val, list) or not all(isinstance(v, str) and v for v in val):
        raise ChecklistError(f"{where}: {key!r} must be a list of strings")
    return val


def build_items(doc) -> list[Item]:
    if not isinstance(doc, dict) or set(doc) - {"version", "items"}:
        raise ChecklistError("top level must hold only 'version' and 'items'")
    if doc.get("version") != "1":
        raise ChecklistError("version must be 1")
    raw_items = doc.get("items")
    if not isinstance(raw_items, list) or not raw_items:
        raise ChecklistError("'items' must be a non-empty list")
    items, seen = [], set()
    for n, raw in enumerate(raw_items, start=1):
        where = f"item {n}"
        if not isinstance(raw, dict):
            raise ChecklistError(f"{where}: must be a mapping")
        if isinstance(raw.get("id"), str):
            where = f"item {n} ({raw['id']})"
        unknown = set(raw) - ITEM_KEYS
        missing = ITEM_KEYS - set(raw)
        if unknown:
            raise ChecklistError(f"{where}: unknown keys {sorted(unknown)}")
        if missing:
            raise ChecklistError(f"{where}: missing keys {sorted(missing)}")
        item_id = _str_field(raw, "id", where)
        if not ID_RE.match(item_id):
            raise ChecklistError(f"{where}: id must be kebab-case")
        if item_id in seen:
            raise ChecklistError(f"{where}: duplicate id")
        seen.add(item_id)
        surface = _str_field(raw, "owner_surface", where)
        if surface not in SURFACES:
            raise ChecklistError(f"{where}: unknown owner_surface {surface!r}")
        trig = raw["trigger"]
        if not isinstance(trig, dict):
            raise ChecklistError(f"{where}: trigger must be a mapping")
        if set(trig) - TRIGGER_KEYS:
            raise ChecklistError(f"{where}: unknown trigger keys {sorted(set(trig) - TRIGGER_KEYS)}")
        item = Item(
            id=item_id,
            title=_str_field(raw, "title", where),
            owner_surface=surface,
            check=_str_field(raw, "check", where),
            incident=_str_field(raw, "incident", where),
        )
        try:
            item.paths = [(g, glob_to_regex(g)) for g in _list_field(trig, "paths", where)]
            item.added_in = [(g, glob_to_regex(g)) for g in _list_field(trig, "added_in", where)]
            item.exclude = [(g, glob_to_regex(g)) for g in _list_field(trig, "exclude", where)]
        except ChecklistError as e:
            raise ChecklistError(f"{where}: {e}") from None
        for src in _list_field(trig, "added", where):
            try:
                item.added.append((src, re.compile(src)))
            except re.error as e:
                raise ChecklistError(f"{where}: regex {src!r} does not compile: {e}") from None
        if not item.paths and not item.added:
            raise ChecklistError(f"{where}: trigger needs 'paths' or 'added'")
        if item.added_in and not item.added:
            raise ChecklistError(f"{where}: 'added_in' needs 'added'")
        if "note" in trig:
            item.note = _str_field(trig, "note", where)
        items.append(item)
    return items


def load_checklist(path: str = CHECKLIST) -> list[Item]:
    try:
        with open(path, encoding="utf-8") as f:
            src = f.read()
    except OSError as e:
        raise ChecklistError(f"cannot read {path}: {e}") from None
    return build_items(parse_yaml_subset(src))


# ── Diff ────────────────────────────────────────────────────────────────────


@dataclass
class Diff:
    paths: list = field(default_factory=list)
    added: list = field(default_factory=list)  # (path, line number, text)


_HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@")


def parse_unified_diff(text: str) -> Diff:
    diff = Diff()
    seen = set()
    path = None
    old_path = None
    lineno = 0
    in_hunk = False

    def note(p):
        if p and p not in seen and p not in SELF_PATHS:
            seen.add(p)
            diff.paths.append(p)

    for line in text.splitlines():
        if line.startswith("diff --git "):
            in_hunk = False
            path = old_path = None
            m = re.match(r"^diff --git a/(.*) b/(.*)$", line)
            if m:
                old_path, path = m.group(1), m.group(2)
                note(path)
            continue
        if not in_hunk:
            if line.startswith("--- "):
                continue
            if line.startswith("+++ "):
                target = line[4:]
                if target == "/dev/null":
                    path = old_path
                elif target.startswith("b/"):
                    path = target[2:]
                note(path)
                continue
        m = _HUNK_RE.match(line)
        if m:
            in_hunk = True
            lineno = int(m.group(1))
            continue
        if in_hunk and path and path not in SELF_PATHS:
            if line.startswith("+"):
                diff.added.append((path, lineno, line[1:]))
                lineno += 1
            elif line.startswith(" "):
                lineno += 1
    return diff


def _git(*args: str) -> str:
    res = subprocess.run(
        ["git", *args], cwd=ROOT, capture_output=True, text=True, errors="replace"
    )
    if res.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)}: {res.stderr.strip()}")
    return res.stdout


def collect_diff(base: str | None, head: str | None) -> tuple[Diff, str]:
    opts = ["diff", "--no-color", "--no-ext-diff", "--unified=0", "-M"]
    if head:
        base = base or "origin/main"
        text = _git(*opts, f"{base}...{head}")
        label = f"{base}...{head}"
        return parse_unified_diff(text), label
    # No head: the working tree against the merge base of HEAD and the base,
    # plus untracked files, so an agent sees its uncommitted work and none of
    # the commits the base gained since the branch was cut.
    base = base or "origin/main"
    try:
        fork = _git("merge-base", "HEAD", base).strip()
    except RuntimeError:
        fork = _git("rev-parse", "--verify", f"{base}^{{commit}}").strip()
    diff = parse_unified_diff(_git(*opts, fork))
    label = f"merge base of {base} and HEAD ({fork[:10]})..working tree"
    for p in _git("ls-files", "--others", "--exclude-standard", "-z").split("\0"):
        if not p or p in diff.paths or p in SELF_PATHS:
            continue
        diff.paths.append(p)
        full = os.path.join(ROOT, p)
        try:
            if os.path.getsize(full) > MAX_UNTRACKED_BYTES:
                continue
            with open(full, encoding="utf-8") as f:
                for n, line in enumerate(f, start=1):
                    diff.added.append((p, n, line.rstrip("\n")))
        except (OSError, UnicodeDecodeError):
            continue
    return diff, label


# ── Output ──────────────────────────────────────────────────────────────────


def _wrap(label: str, text: str, width: int = 100) -> str:
    pad = " " * 4
    lead = f"{pad}{label}: "
    hang = " " * len(lead)
    paras = text.split("\n\n")
    out = []
    for k, para in enumerate(paras):
        first = lead if k == 0 else hang
        out.append(
            textwrap.fill(
                " ".join(para.split()), width=width, initial_indent=first,
                subsequent_indent=hang, break_on_hyphens=False, break_long_words=False,
            )
        )
    return "\n".join(out)


def render(item: Item, n: int, reasons: list[str] | None) -> str:
    lines = [f"[{n}] {item.id}: {item.title}", f"    surface: {item.owner_surface}"]
    if reasons:
        lines.append(f"    matched: {reasons[0]}")
        lines.extend(f"             {r}" for r in reasons[1:])
    if item.note:
        lines.append(_wrap("false alarm when", item.note))
    lines.append(_wrap("check", item.check))
    lines.append(_wrap("incident", item.incident))
    return "\n".join(lines)


# ── Self-test ───────────────────────────────────────────────────────────────

# Each fixture is a synthetic diff: {path: [added lines]} (an empty list means
# the path changed with no added lines, as a deletion does). `expect` lists ids
# the diff must select, `reject` ids it must not. Every checklist item has to be
# selected by at least one fixture, so an item added without one fails here.
FIXTURES = [
    {
        "name": "docs-only change selects nothing",
        "files": {
            "web/docs/kapi/getting-started.md": ["Run `kapi init` to create a project."],
            "docs/internals/testing.md": ["The suites run in CI."],
        },
        "expect": [],
        "exact": True,
    },
    {
        "name": "near misses select nothing",
        "files": {
            "bowrain/mobile/src/components/Notice.tsx": [
                '  <Text className="text-warning-foreground text-sm">{msg}</Text>',
            ],
            "bowrain/store/followed.go": [
                "\t\tVALUES ($1, $2, $3::jsonb)",
                "\tn, err := s.CountBlocks(ctx, BlockQuery{ProjectID: pid})",
            ],
            "bowrain/server/handlers_upload_test.go": [
                "\t// through the resolver kapi push uses, so the file is read once",
            ],
            "core/formats/yaml/reader.go": ["\tif isBareURL(block.SourceText()) {"],
        },
        "expect": [],
        "reject": ["warning-token-as-text", "migration-cast-guards-bad-rows",
                   "unbounded-store-read", "kapi-invocation-isolated", "writer-keeps-inline-codes"],
    },
    {
        "name": "migration adds a column and casts to jsonb",
        "files": {
            "bowrain/terms/postgres.go": [
                "\t\tALTER TABLE tb_terms ADD COLUMN IF NOT EXISTS text_normalized TEXT;",
                "\t\tCREATE INDEX IF NOT EXISTS tb_terms_norm ON tb_terms (locale, text_normalized);",
                "\t\tALTER TABLE pilots ALTER COLUMN props TYPE jsonb USING props::jsonb;",
            ],
        },
        "expect": ["pg-suites-skip-on-pull-requests", "schema-change-reaches-existing-databases",
                   "new-column-needs-backfill", "migration-cast-guards-bad-rows"],
    },
    {
        "name": "a test drops a ledger row and expects a replay",
        "files": {
            "bowrain/server/termcheck_cache_pg_test.go": [
                "\tdb := pgtest.NewTestDB(t)",
                "\t_, err = db.Exec(`DELETE FROM tb_schema_migrations WHERE version = 11`)",
            ],
        },
        "expect": ["pg-suites-skip-on-pull-requests", "migration-runner-applies-only-higher-versions"],
    },
    {
        "name": "baseline edited in the migration registry",
        "files": {"bowrain/migrations/registry.go": ["\tVersion: 12,"]},
        "expect": ["schema-change-reaches-existing-databases"],
    },
    {
        "name": "an unbounded store read",
        "files": {"bowrain/server/handlers_review.go": [
            "\tblocks, err := s.store.GetBlocks(ctx, store.BlockQuery{ProjectID: pid})",
        ]},
        "expect": ["unbounded-store-read"],
    },
    {
        "name": "SQL lookup with no LIMIT",
        "files": {"terms/tiered_lookup.go": [
            "\trows, err := s.db.QueryContext(ctx, `SELECT id, text FROM tb_terms WHERE locale = ?`, loc)",
        ]},
        "expect": ["unbounded-store-read"],
    },
    {
        "name": "database clock compared with a process clock",
        "files": {"bowrain/auth/postgres.go": [
            "\t\tUPDATE refresh_tokens SET consumed_at = NOW() WHERE id = $1",
        ]},
        "expect": ["one-clock-for-compared-timestamps"],
    },
    {
        "name": "a sleep orders two timestamps in a test",
        "files": {"bowrain/store/history_test.go": ["\ttime.Sleep(5 * time.Millisecond)"]},
        "expect": ["one-clock-for-compared-timestamps"],
    },
    {
        "name": "pool ceiling on a pool-backed handle",
        "files": {"bowrain/storage/postgres.go": ["\tdb.SetMaxIdleConns(5)"]},
        "expect": ["one-connection-ceiling"],
    },
    {
        "name": "extension created without a schema",
        "files": {"bowrain/store/postgres.go": ["\tCREATE EXTENSION IF NOT EXISTS pg_trgm;"]},
        "expect": ["extension-schema-pinned"],
    },
    {
        "name": "a new Pulse route",
        "files": {"bowrain/server/handlers_pulse.go": [
            '\tg.GET("/pulse/:ws/heatmap", s.handlePulseHeatmap)',
        ]},
        "expect": ["new-route-applies-sibling-access-rules"],
    },
    {
        "name": "proxy trust widened",
        "files": {"bowrain/server/server.go": [
            "\topts = append(opts, echo.TrustIPRange(ipNet))",
            "\te.SchemeExtractor = echo.ExtractSchemeFromHeaders(opts...)",
        ]},
        "expect": ["proxy-trust"],
    },
    {
        "name": "Echo proxy middleware",
        "files": {"bowrain/server/server.go": ["\te.Use(middleware.Proxy(balancer))"]},
        "expect": ["proxy-trust"],
    },
    {
        "name": "trusted proxies configured in compose",
        "files": {"bowrain/compose.full.yaml": ["      BOWRAIN_TRUSTED_PROXIES: 203.0.113.0/24"]},
        "expect": ["proxy-trust"],
    },
    {
        "name": "proxy trust named outside the server",
        "files": {"host/proxy.go": ["\t// configureIPExtractor lives in the server"]},
        "expect": [],
        "reject": ["proxy-trust"],
    },
    {
        "name": "sync wire field",
        "files": {"core/proto/sync/v1/sync.proto": ["  string stream_id = 9;"]},
        "expect": ["sync-field-travels-both-projections", "wire-and-persisted-identifiers"],
    },
    {
        "name": "review compares the identity hash",
        "files": {"host/review_history.go": [
            "\tif v.GovernedBy == unit.ContextHash {",
        ]},
        "expect": ["governance-fingerprint-not-identity-hash"],
    },
    {
        "name": "AI recorder registered",
        "files": {"host/explain.go": ["\taiprovider.AddRecorder(rec)"]},
        "expect": ["ai-recorder-registered-first"],
    },
    {
        "name": "dependency bump in one module",
        "files": {"bowrain/go.mod": ["\tgithub.com/beevik/etree v1.8.0"]},
        "expect": ["dependency-bump-moves-every-module"],
    },
    {
        "name": "format writer renders TargetText",
        "files": {"core/formats/eml/writer.go": ["\t\treturn block.TargetText(w.Locale)"]},
        "expect": ["writer-keeps-inline-codes"],
    },
    {
        "name": "header decoder errors on a charset",
        "files": {"core/formats/eml/reader.go": [
            '\t\treturn fmt.Errorf("unsupported charset %q", cs)',
        ]},
        "expect": ["reader-leaves-undecodable-values"],
    },
    {
        "name": "bounded-memory test samples raw heap",
        "files": {"core/formats/resx/reader_test.go": ["\truntime.ReadMemStats(&m)"]},
        "expect": ["bounded-memory-test-measures-live-heap"],
    },
    {
        "name": "collection in a plugin format",
        "files": {"kapi.yaml": ["    format: sourcecode"]},
        "expect": ["plugin-format-collection"],
    },
    {
        "name": "cobra context at a store call",
        "files": {"cli/verify.go": ["\tst, err := memory.Open(cmd.Context(), path)"]},
        "expect": ["nil-command-context"],
    },
    {
        "name": "a script runs kapi",
        "files": {"scripts/demo-check.sh": ['kapi up -p "$dir"']},
        "expect": ["kapi-invocation-isolated"],
    },
    {
        "name": "eval resolves kapi from PATH",
        "files": {"scripts/skilleval/main.go": ['\tp, err := exec.LookPath("kapi")']},
        "expect": ["kapi-invocation-isolated"],
    },
    {
        "name": "a json field on a bound host type",
        "files": {"host/converge.go": ['\tSkipped int `json:"skipped"`']},
        "expect": ["desktop-bindings-follow-shared-types"],
    },
    {
        "name": "desktop reads the singular plugin variable",
        "files": {"apps/kapi-desktop/backend/app.go": ['\tdir := os.Getenv("KAPI_PLUGIN_DIR")']},
        "expect": ["plugins-dir-variable"],
    },
    {
        "name": "standalone e2e tree spec",
        "files": {"bowrain/e2e/cloud/specs/push.spec.ts": ["  expect(res.stream).toBe('main');"]},
        "expect": ["standalone-ts-tree-typechecked"],
    },
    {
        "name": "warning colour as text",
        "files": {"packages/ui/src/components/Badge.tsx": ['  warning: "text-warning",']},
        "expect": ["warning-token-as-text"],
    },
    {
        "name": "packageManager pin moved",
        "files": {"package.json": ['  "packageManager": "pnpm@11.24.0",']},
        "expect": ["package-manager-pin", "package-manager-launchers"],
    },
    {
        "name": "a client type gains a field the app will read",
        "files": {"packages/bowrain-client/src/plan.ts": [
            "  waiting: PlanWaiting[];",
        ]},
        "expect": ["mobile-reads-cached-shapes"],
    },
    {
        "name": "a mobile screen changes",
        "files": {"bowrain/mobile/src/app/sign-in.tsx": [
            '  if (session) return <Redirect href="/workspaces" />;',
        ]},
        "expect": ["mobile-maestro-flows"],
    },
    {
        "name": "a test makes a temporary directory",
        "files": {"core/formats/foo/foo_test.go": ['\tdir, err := os.MkdirTemp("", "foo-")']},
        "expect": ["temporary-file-owner"],
    },
    {
        "name": "a test that can only skip",
        "files": {"scripts/authoringeval/fixtures_test.go": ['\t\tt.Skip("no kapi binary")']},
        "expect": ["test-that-only-skips"],
    },
    {
        "name": "new scheduled workflow",
        "files": {".github/workflows/weekly-audit.yml": ["  schedule:", '    - cron: "0 3 * * 1"']},
        "expect": ["scheduled-job-reports-failure"],
    },
    {
        "name": "a download from a third-party release",
        "files": {".github/workflows/nightly.yml": [
            "          curl -sSL https://github.com/example/lib/releases/download/v1.2/lib.tgz | tar xz",
        ]},
        "expect": ["third-party-download-pinned"],
    },
    {
        "name": "lint piped to tail",
        "files": {"Makefile": ["\tgolangci-lint run ./... | tail -20"]},
        "expect": ["pipeline-hides-exit-code"],
    },
    {
        "name": "a scan loops over modules",
        "files": {".github/workflows/security.yml": [
            "          for mod in . host cli kapi bowrain; do (cd $mod && govulncheck ./...); done",
        ]},
        "expect": ["loop-reports-every-iteration"],
    },
    {
        "name": "repeated -tags",
        "files": {"Makefile": ["\t$(GOTEST) -tags integration ./bowrain/..."]},
        "expect": ["go-tags-last-wins"],
    },
    {
        "name": "chained step outputs",
        "files": {".github/workflows/ci.yml": [
            "      any: ${{ steps.filter.outputs.cli || steps.filter.outputs.kapi }}",
        ]},
        "expect": ["actions-output-truthiness"],
    },
    {
        "name": "BSD sed with a word boundary",
        "files": {"scripts/rename-terms.sh": [r"sed -i '' 's/\bTM\b/memory/g' \"$f\""]},
        "expect": ["shell-pattern-dialect"],
    },
    {
        "name": "a new guard script",
        "files": {"scripts/check-new-thing.sh": ["#!/bin/bash"]},
        "expect": ["new-gate-fails-on-purpose"],
    },
    {
        "name": "digest over a whole tree",
        "files": {"scripts/format-ops/content-maturity.mjs": [
            "  const digest = hashTree('core/formats');",
        ]},
        "expect": ["freshness-digest-scope"],
    },
    {
        "name": "gitignore edited",
        "files": {"harness/.gitignore": ["public/"]},
        "expect": ["ignore-rule-swallows-files"],
    },
    {
        "name": "release workflow edited",
        "files": {".github/workflows/release-sat.yml": ["        uses: dtolnay/rust-toolchain@stable"]},
        "expect": ["release-path-dry-run", "toolchain-pinned"],
    },
    {
        "name": "Go toolchain bump",
        "files": {"go.work": ["go 1.28.0"]},
        "expect": ["toolchain-pinned"],
    },
    {
        "name": "CDN publish script",
        "files": {"scripts/publish-cdn-assets.sh": ['aws s3 cp "$f" "s3://$BUCKET/$key"']},
        "expect": ["cdn-object-headers"],
    },
    {
        "name": "target-locale build policy",
        "files": {"web/docusaurus.config.ts": ['  onBrokenLinks: "throw",']},
        "expect": ["target-locale-never-gates"],
    },
]


_SELFTEST_YAML_OK = """\
# comment
version: 1
items:
  - id: sample-item
    title: 'It''s a title'
    owner_surface: ci
    trigger:
      paths:
        - 'a/**/*.go'
      added:
        - 'foo\\(\\)'
      note: >-
        folded
        text
    check: |
      line one
        indented
    incident: plain text # trailing comment
"""


def self_test() -> int:
    failures: list[str] = []

    def expect(cond: bool, msg: str) -> None:
        if not cond:
            failures.append(msg)

    # Parser: the supported subset.
    doc = parse_yaml_subset(_SELFTEST_YAML_OK)
    it = doc["items"][0]
    expect(it["title"] == "It's a title", f"single-quote escape: {it['title']!r}")
    expect(it["trigger"]["added"] == ["foo\\(\\)"], f"regex scalar: {it['trigger']['added']!r}")
    expect(it["trigger"]["note"] == "folded text", f"folded scalar: {it['trigger']['note']!r}")
    expect(it["check"] == "line one\n  indented", f"literal scalar: {it['check']!r}")
    expect(it["incident"] == "plain text", f"trailing comment: {it['incident']!r}")
    expect(len(build_items(doc)) == 1, "valid sample builds")

    # Parser and validation: what must be refused.
    bad = {
        "tab indent": _SELFTEST_YAML_OK.replace("    title", "\ttitle"),
        "unknown key": _SELFTEST_YAML_OK.replace("    incident:", "    severity: high\n    incident:"),
        "bad regex": _SELFTEST_YAML_OK.replace("'foo\\(\\)'", "'foo('"),
        "unknown surface": _SELFTEST_YAML_OK.replace("owner_surface: ci", "owner_surface: infra"),
        "bad id": _SELFTEST_YAML_OK.replace("id: sample-item", "id: Sample_Item"),
        "missing check": _SELFTEST_YAML_OK.replace("    check: |\n      line one\n        indented\n", ""),
        "alias": _SELFTEST_YAML_OK.replace("'a/**/*.go'", "*.go"),
        "duplicate id": _SELFTEST_YAML_OK + _SELFTEST_YAML_OK.split("items:\n", 1)[1],
        "empty trigger": _SELFTEST_YAML_OK.replace(
            "      paths:\n        - 'a/**/*.go'\n      added:\n        - 'foo\\(\\)'\n", ""),
    }
    for name, src in bad.items():
        try:
            build_items(parse_yaml_subset(src))
            failures.append(f"malformed sample accepted: {name}")
        except ChecklistError:
            pass

    # Globs.
    for glob, path, want in [
        ("bowrain/**/*.go", "bowrain/server/x.go", True),
        ("bowrain/**/*.go", "bowrain/x.go", True),
        ("bowrain/*.go", "bowrain/server/x.go", False),
        (".github/workflows/release*.yml", ".github/workflows/release-sat.yml", True),
        ("**/go.{mod,sum}", "plugins/sat/go.sum", True),
        ("**/go.{mod,sum}", "go.mod", True),
        ("core/formats/**", "core/formatsx/a.go", False),
    ]:
        expect(bool(glob_to_regex(glob).match(path)) == want, f"glob {glob} vs {path}")

    # Unified diff parsing.
    sample = (
        "diff --git a/old.go b/new.go\n"
        "similarity index 90%\nrename from old.go\nrename to new.go\n"
        "--- a/old.go\n+++ b/new.go\n"
        "@@ -3,0 +4,2 @@ func x() {\n+first\n+second\n"
        "diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644\n"
        "--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-bye\n"
    )
    d = parse_unified_diff(sample)
    expect(d.paths == ["new.go", "gone.txt"], f"diff paths: {d.paths}")
    expect(d.added == [("new.go", 4, "first"), ("new.go", 5, "second")], f"diff added: {d.added}")

    # Fixtures against the real checklist.
    try:
        items = load_checklist()
    except ChecklistError as e:
        failures.append(f"checklist does not load: {e}")
        items = []
    ids = {i.id for i in items}
    covered = set()
    for fx in FIXTURES:
        diff = Diff()
        for path, lines in fx["files"].items():
            diff.paths.append(path)
            diff.added.extend((path, n, text) for n, text in enumerate(lines, start=1))
        got = {i.id for i in items if i.match(diff)}
        for want in fx["expect"]:
            if want not in ids:
                failures.append(f"fixture {fx['name']!r} names unknown item {want}")
            elif want not in got:
                failures.append(f"fixture {fx['name']!r} did not select {want}")
        for no in fx.get("reject", []):
            if no in got:
                failures.append(f"fixture {fx['name']!r} selected {no}")
        if fx.get("exact") and got != set(fx["expect"]):
            failures.append(f"fixture {fx['name']!r} selected {sorted(got - set(fx['expect']))}")
        covered |= got & set(fx["expect"])
    for missing in sorted(ids - covered):
        failures.append(f"item {missing} is selected by no fixture; add one to FIXTURES")

    if failures:
        for f in failures:
            print(f"FAIL: {f}", file=sys.stderr)
        print(f"reliability-checklist self-test: {len(failures)} failure(s)", file=sys.stderr)
        return 1
    print(f"reliability-checklist self-test: ok ({len(items)} items, {len(FIXTURES)} fixtures)")
    return 0


# ── Main ────────────────────────────────────────────────────────────────────


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--base", help="base ref (default: origin/main); the diff starts at its merge base with the head")
    ap.add_argument("--head", help="head ref (default: the working tree, untracked files included)")
    ap.add_argument("--all", action="store_true", help="print every item")
    ap.add_argument("--self-test", action="store_true", help="test the parser, matcher and fixtures")
    ap.add_argument("--ids", action="store_true", help="print only the ids of matching items")
    args = ap.parse_args(argv)

    if args.self_test:
        return self_test()

    try:
        items = load_checklist()
    except ChecklistError as e:
        print(f"reliability-checklist: {os.path.relpath(CHECKLIST, ROOT)}: {e}", file=sys.stderr)
        return 2

    if args.all:
        print(f"reliability-checklist: all {len(items)} items\n")
        print("\n\n".join(render(i, n, None) for n, i in enumerate(items, start=1)))
        return 0

    try:
        diff, label = collect_diff(args.base, args.head)
    except RuntimeError as e:
        print(f"reliability-checklist: {e}", file=sys.stderr)
        return 3

    matches = [(i, r) for i in items for r in [i.match(diff)] if r]
    if args.ids:
        print(" ".join(i.id for i, _ in matches))
        return 0
    print(
        f"reliability-checklist: {len(matches)} of {len(items)} items match {label} "
        f"({len(diff.paths)} files, {len(diff.added)} added lines)"
    )
    if not matches:
        print("No item applies to this diff.")
        return 0
    print("Run each check below, or state why it does not apply.\n")
    print("\n\n".join(render(i, n, r) for n, (i, r) in enumerate(matches, start=1)))
    return 0


if __name__ == "__main__":
    # Exit quietly when the reader of the output (head, less) closes early.
    signal.signal(signal.SIGPIPE, signal.SIG_DFL)
    sys.exit(main(sys.argv[1:]))
