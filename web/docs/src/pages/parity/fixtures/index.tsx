import { useMemo, useState } from "react";
import Layout from "@theme/Layout";
import data from "@site/static/data/parity-fixtures.json";
import styles from "./index.module.css";

type Tier = "byte-equal" | "canonical-equal" | "semantic-equal" | "divergent";

interface EngineTotals {
  total: number;
  byte: number;
  canon: number;
  sem: number;
  div: number;
  skip: number;
  byte_pct: number;
}

type Severity = "bug" | "cosmetic" | "native-more-correct" | "fixture-bug" | "unknown";

interface Annotation {
  severity?: Severity;
  issue?: number;
  issue_url?: string;
  summary?: string;
  spec_ref?: string;
  notes_anchor?: string;
}

interface FixtureEntry {
  fixture: string;
  required: Tier;
  achieved: Tier;
  got_size: number;
  ref_size: number;
  delta: number;
  raw_diff_offset: number;
  norm_diff_offset?: number;
  normalizer?: string;
  reason: string;
  annotation?: Annotation;
}

interface FormatBreakdown {
  format: string;
  engine: string;
  total: number;
  byte: number;
  canon: number;
  sem: number;
  div: number;
  skip: number;
  fixtures?: FixtureEntry[];
}

interface FixturesData {
  generated_at: string;
  engines: Record<string, EngineTotals>;
  formats: FormatBreakdown[];
}

const dataset = data as unknown as FixturesData;

const tierClass: Record<string, string> = {
  "byte-equal": styles.tierByte,
  "canonical-equal": styles.tierCanon,
  "semantic-equal": styles.tierSem,
  divergent: styles.tierDiv,
};

const tierShort: Record<string, string> = {
  "byte-equal": "byte",
  "canonical-equal": "canon",
  "semantic-equal": "sem",
  divergent: "div",
};

const severityClass: Record<Severity, string> = {
  bug: styles.severityBug,
  cosmetic: styles.severityCosmetic,
  "native-more-correct": styles.severityNativeBetter,
  "fixture-bug": styles.severityFixtureBug,
  unknown: styles.severityUnknown,
};

const severityLabel: Record<Severity, string> = {
  bug: "bug",
  cosmetic: "cosmetic",
  "native-more-correct": "native+",
  "fixture-bug": "fixture-bug",
  unknown: "unannotated",
};

const severityDescription: Record<Severity, string> = {
  bug: "Real correctness/data-loss bug in native. Must be fixed.",
  cosmetic:
    "Output differs only in attribute placement, run granularity, or other choices that render identically per spec. Spec-permissible on both sides.",
  "native-more-correct":
    "Native output is more spec-compliant than the Okapi reference. The divergence is a 'win' — flagged only because Okapi is the byte-equality reference.",
  "fixture-bug":
    "The upstream fixture itself is malformed or otherwise unusable for parity comparison.",
  unknown:
    "Divergence not yet triaged. The fail-new CI gate fails on these — add a YAML entry documenting why it diverges.",
};

const severityFilterOrder: Severity[] = [
  "bug",
  "cosmetic",
  "native-more-correct",
  "fixture-bug",
  "unknown",
];

function rowKey(f: FormatBreakdown): string {
  return `${f.engine}/${f.format}`;
}

function formatBytes(n: number): string {
  return n.toLocaleString();
}

interface ParsedDiff {
  normalizer?: string;
  zipEntry?: string;
  context?: string;
  offset?: number;
  got?: string;
  ref?: string;
}

// findQuotedSubstring locates the matched closing `"` for a Go-style %q
// quoted string starting at pos (where text[pos] === '"'). Returns the
// substring contents (escapes preserved) and the position right after
// the closing quote, or null on parse failure.
function findQuotedSubstring(text: string, pos: number): [string, number] | null {
  if (text[pos] !== '"') return null;
  let i = pos + 1;
  while (i < text.length) {
    const c = text[i];
    if (c === '"') return [text.slice(pos + 1, i), i + 1];
    if (c === "\\" && i + 1 < text.length) {
      i += 2;
    } else {
      i++;
    }
  }
  return null;
}

// parseDiffReason extracts structured pieces from a parityRecord.Reason
// string. Handles four shapes produced by compare.go:
//   - byte length differs: got N, reference M (first diff at offset O: got "X" vs "Y")
//   - byte content differs at offset O: got "X" vs "Y"
//   - zip entry "ENTRY" differs at offset O: got "X" vs "Y"
//   - [after NORM] <any of the above>
// Anything it can't parse falls back to {raw: reason} via caller.
function parseDiffReason(reason: string): ParsedDiff {
  const out: ParsedDiff = {};
  let s = reason;

  // Optional [after ...] prefix carrying the normalizer chain.
  if (s.startsWith("[after ")) {
    let depth = 1;
    let i = "[after ".length;
    for (; i < s.length && depth > 0; i++) {
      if (s[i] === "[") depth++;
      else if (s[i] === "]") depth--;
    }
    if (depth === 0) {
      out.normalizer = s.slice("[after ".length, i - 1);
      s = s.slice(i).replace(/^\s+/, "");
    }
  }

  // Optional zip entry: zip entry "ENTRY" differs at offset N
  const zipPrefix = 'zip entry "';
  if (s.startsWith(zipPrefix)) {
    const q = findQuotedSubstring(s, zipPrefix.length - 1);
    if (q) {
      out.zipEntry = q[0];
      s = s.slice(q[1]).replace(/^\s+/, "");
    }
  }

  // First-diff offset.
  const offMatch = s.match(/(?:first diff )?at offset (\d+)/);
  if (offMatch) {
    out.offset = parseInt(offMatch[1], 10);
  }

  // Context like "byte length differs: got N, reference M (...)".
  const ctxMatch = s.match(/^([^:]+?)(?:: got \d+|differs|$)/);
  if (ctxMatch) {
    out.context = ctxMatch[1].replace(/[:.]\s*$/, "").trim();
  }

  // got "X" vs "Y" — find the LAST occurrence of `got "` (the snippet,
  // not the byte-count "got 238048").
  const gotIdx = s.lastIndexOf('got "');
  if (gotIdx >= 0) {
    const gq = findQuotedSubstring(s, gotIdx + "got ".length);
    if (gq) {
      out.got = gq[0];
      const after = s.slice(gq[1]).replace(/^\s+/, "");
      const vsIdx = after.indexOf('vs "');
      if (vsIdx >= 0) {
        const rq = findQuotedSubstring(after, vsIdx + "vs ".length);
        if (rq) out.ref = rq[0];
      }
    }
  }

  return out;
}

// unescapeGo reverses Go's %q escaping so the dashboard shows actual
// file content rather than Go's quoted representation:
//   `\\` `\"` → literal backslash / quote
//   `\n` `\t` `\r` etc. → real control chars (`.diffLine` is pre-wrap,
//     so real newlines render as line breaks — far more readable than
//     `\n` literals scattered through the snippet)
//   `\xHH` → the corresponding byte (typical for invalid UTF-8 in
//     binary content like docx/idml)
//   `\uHHHH` / `\UHHHHHHHH` → the corresponding rune (e.g. invisible
//     `­` SHY, `﻿` BOM that often *are* the diff)
// Unrecognised escapes are kept literal.
function unescapeGo(s: string): string {
  let out = "";
  for (let i = 0; i < s.length; i++) {
    if (s[i] !== "\\" || i + 1 >= s.length) {
      out += s[i];
      continue;
    }
    const n = s[i + 1];
    switch (n) {
      case "n":
        out += "\n";
        i++;
        continue;
      case "t":
        out += "\t";
        i++;
        continue;
      case "r":
        out += "\r";
        i++;
        continue;
      case "a":
        out += "\x07";
        i++;
        continue;
      case "b":
        out += "\x08";
        i++;
        continue;
      case "f":
        out += "\x0c";
        i++;
        continue;
      case "v":
        out += "\x0b";
        i++;
        continue;
      case "0":
        out += "\x00";
        i++;
        continue;
      case "\\":
        out += "\\";
        i++;
        continue;
      case '"':
        out += '"';
        i++;
        continue;
      case "x": {
        if (i + 3 < s.length) {
          const code = parseInt(s.slice(i + 2, i + 4), 16);
          if (!Number.isNaN(code)) {
            out += String.fromCharCode(code);
            i += 3;
            continue;
          }
        }
        break;
      }
      case "u": {
        if (i + 5 < s.length) {
          const code = parseInt(s.slice(i + 2, i + 6), 16);
          if (!Number.isNaN(code)) {
            out += String.fromCharCode(code);
            i += 5;
            continue;
          }
        }
        break;
      }
      case "U": {
        if (i + 9 < s.length) {
          const code = parseInt(s.slice(i + 2, i + 10), 16);
          if (!Number.isNaN(code)) {
            out += String.fromCodePoint(code);
            i += 9;
            continue;
          }
        }
        break;
      }
    }
    out += s[i];
  }
  return out;
}

type DiffSeg = { type: "common" | "del" | "ins"; text: string };

// renderDiffText prepares a diff segment's text for rendering. Inside a
// del/ins span, whitespace control characters are swapped for visible
// Unicode glyphs (CR → ␍, LF → ␊, TAB → →) so EOL/whitespace-only
// diffs are no longer invisible coloured boxes — the user can see at a
// glance whether got is CRLF and ref is LF, or whether one side has a
// stray tab. Newlines stay in-place after the glyph so the layout still
// wraps correctly. Inside a common span, control chars pass through
// unchanged so real newlines render as actual line breaks.
function renderDiffText(text: string, type: DiffSeg["type"]): string {
  if (type === "common") return text;
  let out = "";
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (c === "\r") out += "␍";
    else if (c === "\n") out += "␊\n";
    else if (c === "\t") out += "→";
    else out += c;
  }
  return out;
}

// tokenize splits a string into atomic units for token-level diffing:
// each match is either a maximal Unicode word run (letters + digits +
// underscore + diacritics), a maximal whitespace run, or one
// non-word/non-space character. This stops a char-level LCS from
// stitching together coincidental letter overlaps in pseudo-translated
// text — `Some text` vs `Sōm ēxt` should diff as two whole-word
// replacements, not as five scattered char edits around three
// coincidentally-shared letters.
function tokenize(s: string): string[] {
  const re = /[\p{L}\p{N}_]+|\s+|./gsu;
  return Array.from(s.matchAll(re), (m) => m[0]);
}

// diffTokens returns the token-level diff between a (got) and b (ref)
// using a standard LCS DP backtrace over tokens, then coalesces runs of
// the same type so a phrase of consecutive insertions or deletions
// renders as one highlighted span. Bounded to 768 tokens per side —
// beyond that we render a whole-replacement fallback to keep the page
// responsive.
function diffTokens(a: string, b: string): DiffSeg[] {
  const ta = tokenize(a);
  const tb = tokenize(b);
  const m = ta.length;
  const n = tb.length;
  const limit = 768;
  if (m > limit || n > limit) {
    const out: DiffSeg[] = [];
    if (a) out.push({ type: "del", text: a });
    if (b) out.push({ type: "ins", text: b });
    return out;
  }
  const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      if (ta[i - 1] === tb[j - 1]) {
        dp[i][j] = dp[i - 1][j - 1] + 1;
      } else {
        dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
      }
    }
  }
  const segs: DiffSeg[] = [];
  let i = m;
  let j = n;
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && ta[i - 1] === tb[j - 1]) {
      segs.push({ type: "common", text: ta[i - 1] });
      i--;
      j--;
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      segs.push({ type: "ins", text: tb[j - 1] });
      j--;
    } else {
      segs.push({ type: "del", text: ta[i - 1] });
      i--;
    }
  }
  segs.reverse();
  const out: DiffSeg[] = [];
  for (const s of segs) {
    const prev = out[out.length - 1];
    if (prev && prev.type === s.type) prev.text += s.text;
    else out.push({ type: s.type, text: s.text });
  }
  return out;
}

interface DiffViewProps {
  reason: string;
}

function DiffView({ reason }: DiffViewProps) {
  const parsed = parseDiffReason(reason);
  const hasSnippet = parsed.got !== undefined && parsed.ref !== undefined;
  if (!hasSnippet) {
    return <span className={styles.diffFallback}>{reason}</span>;
  }
  const got = unescapeGo(parsed.got ?? "");
  const ref = unescapeGo(parsed.ref ?? "");
  return (
    <div className={styles.diffBox}>
      {(parsed.zipEntry || parsed.context || parsed.normalizer || parsed.offset !== undefined) && (
        <div className={styles.diffMeta}>
          {parsed.offset !== undefined && (
            <span className={styles.diffOffset}>@{parsed.offset.toLocaleString()}</span>
          )}
          {parsed.context && <span className={styles.diffContext}>{parsed.context}</span>}
          {parsed.zipEntry && <span className={styles.diffEntryChip}>zip:{parsed.zipEntry}</span>}
          {parsed.normalizer && (
            <span className={styles.diffNormChip} title={parsed.normalizer}>
              norm:
              {parsed.normalizer.length > 60
                ? parsed.normalizer.slice(0, 57) + "…"
                : parsed.normalizer}
            </span>
          )}
        </div>
      )}
      <div className={styles.diffStack}>
        <div className={styles.diffSideCol}>
          <span className={styles.diffLabelGot}>got</span>
          <pre className={styles.diffLine}>
            {diffTokens(got, ref).map((seg, i) => {
              if (seg.type === "ins") return null;
              const cls = seg.type === "common" ? styles.diffCommon : styles.diffDelGot;
              return (
                <span key={i} className={cls}>
                  {renderDiffText(seg.text, seg.type)}
                </span>
              );
            })}
          </pre>
        </div>
        <div className={styles.diffSideCol}>
          <span className={styles.diffLabelRef}>ref</span>
          <pre className={styles.diffLine}>
            {diffTokens(got, ref).map((seg, i) => {
              if (seg.type === "del") return null;
              const cls = seg.type === "common" ? styles.diffCommon : styles.diffDelRef;
              return (
                <span key={i} className={cls}>
                  {renderDiffText(seg.text, seg.type)}
                </span>
              );
            })}
          </pre>
        </div>
      </div>
    </div>
  );
}

type TierFilter = "all-non-byte" | "div-only" | "sem-only" | "canon-only";

const tierFilterLabels: Record<TierFilter, string> = {
  "all-non-byte": "all non-byte",
  "div-only": "div only",
  "sem-only": "sem only",
  "canon-only": "canon only",
};

function fixtureMatchesTier(f: FixtureEntry, t: TierFilter): boolean {
  switch (t) {
    case "div-only":
      return f.achieved === "divergent";
    case "sem-only":
      return f.achieved === "semantic-equal";
    case "canon-only":
      return f.achieved === "canonical-equal";
    default:
      return f.achieved !== "byte-equal";
  }
}

function formatHasMatchingFixture(f: FormatBreakdown, t: TierFilter): boolean {
  switch (t) {
    case "div-only":
      return f.div > 0;
    case "sem-only":
      return f.sem > 0;
    case "canon-only":
      return f.canon > 0;
    default:
      return f.div + f.sem + f.canon > 0;
  }
}

function fixtureSeverity(f: FixtureEntry): Severity {
  if (f.annotation?.severity) return f.annotation.severity;
  // A divergent fixture without an annotation is "unannotated" — the
  // fail-new CI gate fails on these. A non-divergent fixture without
  // an annotation is just an expected canonical/semantic result and
  // doesn't need one — we don't badge those.
  if (f.achieved === "divergent") return "unknown";
  return "unknown";
}

function fixtureMatchesSeverity(f: FixtureEntry, set: Set<Severity>): boolean {
  if (set.size === 0) return true;
  return set.has(fixtureSeverity(f));
}

function isUnannotatedDivergence(f: FixtureEntry): boolean {
  return f.achieved === "divergent" && !f.annotation;
}

// SeverityLegend renders a compact card explaining each severity
// value. It doubles as a discoverable filter — clicking a row toggles
// the corresponding severity in the filter set.
function SeverityLegend({
  active,
  onToggle,
}: {
  active: Set<Severity>;
  onToggle: (s: Severity) => void;
}) {
  return (
    <div className={styles.legend}>
      <div className={styles.legendTitle}>Severity legend (click to filter)</div>
      <ul className={styles.legendList}>
        {severityFilterOrder.map((s) => {
          const isOn = active.has(s);
          return (
            <li key={s}>
              <button
                type="button"
                className={`${styles.legendRow} ${isOn ? styles.legendRowActive : ""}`}
                onClick={() => onToggle(s)}
              >
                <span
                  className={`${styles.severityBadge} ${severityClass[s]}`}
                  title={severityDescription[s]}
                >
                  {severityLabel[s]}
                </span>
                <span className={styles.legendDescription}>{severityDescription[s]}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export default function ParityFixturesDashboard() {
  const [search, setSearch] = useState("");
  const [activeEngine, setActiveEngine] = useState<string | null>("native");
  const [tierFilter, setTierFilter] = useState<TierFilter>("all-non-byte");
  const [severityFilter, setSeverityFilter] = useState<Set<Severity>>(() => new Set());
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const engineNames = useMemo(() => {
    const set = new Set<string>();
    for (const f of dataset.formats) set.add(f.engine);
    return Array.from(set).sort();
  }, []);

  // Aggregate severity counts across the entire current view so the
  // header shows e.g. "1 unannotated divergence" prominently — that's
  // exactly what the CI gate fails on, so it deserves to surface.
  const severityCounts = useMemo<Record<Severity, number>>(() => {
    const init: Record<Severity, number> = {
      bug: 0,
      cosmetic: 0,
      "native-more-correct": 0,
      "fixture-bug": 0,
      unknown: 0,
    };
    for (const f of dataset.formats) {
      if (!f.fixtures) continue;
      for (const fx of f.fixtures) {
        if (fx.achieved !== "divergent") continue;
        if (isUnannotatedDivergence(fx)) {
          init.unknown += 1;
          continue;
        }
        const sev = fx.annotation?.severity ?? "unknown";
        init[sev] = (init[sev] ?? 0) + 1;
      }
    }
    return init;
  }, []);

  const toggleSeverity = (s: Severity): void => {
    setSeverityFilter((prev) => {
      const next = new Set(prev);
      if (next.has(s)) next.delete(s);
      else next.add(s);
      return next;
    });
  };

  const filteredFixtures = (f: FormatBreakdown): FixtureEntry[] => {
    const q = search.trim().toLowerCase();
    return (f.fixtures ?? []).filter((d) => {
      if (!fixtureMatchesTier(d, tierFilter)) return false;
      if (!fixtureMatchesSeverity(d, severityFilter)) return false;
      if (!q) return true;
      return (
        d.fixture.toLowerCase().includes(q) ||
        d.reason.toLowerCase().includes(q) ||
        (d.annotation?.summary?.toLowerCase().includes(q) ?? false)
      );
    });
  };

  const visible = useMemo<FormatBreakdown[]>(() => {
    const q = search.trim().toLowerCase();
    return dataset.formats
      .filter((f) => (activeEngine ? f.engine === activeEngine : true))
      .filter((f) => formatHasMatchingFixture(f, tierFilter))
      .filter((f) => {
        // Apply severity filter at format level so empty formats hide.
        if (severityFilter.size === 0) return true;
        return (f.fixtures ?? []).some((fx) => fixtureMatchesSeverity(fx, severityFilter));
      })
      .filter((f) => {
        if (!q) return true;
        if (f.format.toLowerCase().includes(q)) return true;
        if (f.fixtures?.some((d) => d.fixture.toLowerCase().includes(q))) return true;
        if (f.fixtures?.some((d) => d.reason.toLowerCase().includes(q))) return true;
        if (f.fixtures?.some((d) => d.annotation?.summary?.toLowerCase().includes(q))) return true;
        return false;
      })
      .sort((a, b) => {
        // Sort by remaining-work severity: div first, then sem, then canon.
        if (a.div !== b.div) return b.div - a.div;
        if (a.sem !== b.sem) return b.sem - a.sem;
        if (a.canon !== b.canon) return b.canon - a.canon;
        if (a.format !== b.format) return a.format.localeCompare(b.format);
        return a.engine.localeCompare(b.engine);
      });
  }, [search, activeEngine, tierFilter, severityFilter]);

  const toggle = (k: string): void => setExpanded((prev) => ({ ...prev, [k]: !prev[k] }));

  return (
    <Layout
      title="Parity Fixtures Drill-Down"
      description="Per-fixture parity divergence detail (first-diff offset, byte delta, snippet)"
    >
      <main className="container margin-vert--lg">
        <h1>Parity Fixtures Drill-Down</h1>
        <p className={styles.subtitle}>
          Per (format, engine, fixture) drill-down for every fixture that isn't byte-equal — the
          remaining work toward byte-exact parity. Each row carries raw/normalized first-diff
          offsets and the comparison snippet so you can scan for patterns (line endings, whitespace,
          encoding…) without re-running the test. Canon and sem rows show the <em>raw</em> byte gap
          that the normalizer (or semantic comparator) is currently bridging. Generated by{" "}
          <code>make parity-fixtures</code> at <strong>{dataset.generated_at}</strong>. See also the
          higher-level <a href="/parity">/parity dashboard</a>.
        </p>

        <div className={styles.totals}>
          {Object.entries(dataset.engines).map(([eng, t]) => (
            <div key={eng} className={styles.totalCard}>
              <h2>{eng}</h2>
              <div className={styles.headline}>
                {t.byte} / {t.total - t.skip}{" "}
                <span className={styles.headlineSuffix}>byte-equal ({t.byte_pct.toFixed(1)}%)</span>
              </div>
              <ul className={styles.totalBreakdown}>
                <li>
                  <span className={`${styles.tierBadge} ${styles.tierByte}`}>byte</span> {t.byte}
                </li>
                <li>
                  <span className={`${styles.tierBadge} ${styles.tierCanon}`}>canon</span> {t.canon}
                </li>
                {t.sem > 0 && (
                  <li>
                    <span className={`${styles.tierBadge} ${styles.tierSem}`}>sem</span> {t.sem}
                  </li>
                )}
                <li>
                  <span className={`${styles.tierBadge} ${styles.tierDiv}`}>div</span> {t.div}
                </li>
                {t.skip > 0 && (
                  <li>
                    <span className={`${styles.tierBadge} ${styles.tierSkip}`}>skip</span> {t.skip}
                  </li>
                )}
              </ul>
            </div>
          ))}
        </div>

        <div className={styles.severityHeadline}>
          {severityFilterOrder.map((s) => (
            <button
              key={s}
              type="button"
              className={`${styles.severityHeadlineItem} ${
                severityFilter.has(s) ? styles.severityHeadlineActive : ""
              }`}
              onClick={() => toggleSeverity(s)}
              title={severityDescription[s]}
            >
              <span className={`${styles.severityBadge} ${severityClass[s]}`}>
                {severityLabel[s]}
              </span>
              <span className={styles.severityHeadlineCount}>{severityCounts[s]}</span>
            </button>
          ))}
        </div>

        <SeverityLegend active={severityFilter} onToggle={toggleSeverity} />

        <div className={styles.toolbar}>
          <input
            type="text"
            className={styles.search}
            placeholder="Filter by format, fixture name, reason snippet, or annotation summary..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className={styles.buttons}>
            <button
              className={`button button--sm ${activeEngine === null ? "button--primary" : "button--outline button--secondary"}`}
              onClick={() => setActiveEngine(null)}
            >
              All engines
            </button>
            {engineNames.map((e) => (
              <button
                key={e}
                className={`button button--sm ${activeEngine === e ? "button--primary" : "button--outline button--secondary"}`}
                onClick={() => setActiveEngine(activeEngine === e ? null : e)}
              >
                {e}
              </button>
            ))}
            <span className={styles.separator}>|</span>
            {(Object.keys(tierFilterLabels) as TierFilter[]).map((t) => (
              <button
                key={t}
                className={`button button--sm ${tierFilter === t ? "button--primary" : "button--outline button--secondary"}`}
                onClick={() => setTierFilter(t)}
              >
                {tierFilterLabels[t]}
              </button>
            ))}
          </div>
        </div>

        <p className={styles.resultCount}>
          {visible.length} (format, engine) rows ·{" "}
          {visible.reduce((n, f) => n + filteredFixtures(f).length, 0)} fixtures matching
          {severityFilter.size > 0 && (
            <>
              {" "}
              · filtered by severity:{" "}
              {Array.from(severityFilter)
                .map((s) => severityLabel[s])
                .join(", ")}{" "}
              <button
                type="button"
                className={styles.clearSeverity}
                onClick={() => setSeverityFilter(new Set())}
              >
                clear
              </button>
            </>
          )}
        </p>

        {visible.map((f) => {
          const k = rowKey(f);
          const open = expanded[k] ?? false;
          const fixtures = filteredFixtures(f);
          return (
            <div key={k} className={styles.formatRow}>
              <div className={styles.formatHeader} onClick={() => toggle(k)}>
                <span className={styles.formatName}>{f.format}</span>
                <span className={styles.engineName}>{f.engine}</span>
                <span className={styles.tierCounts}>
                  <span>
                    <span className={`${styles.tierBadge} ${styles.tierByte}`}>byte</span>
                    {f.byte}
                  </span>
                  <span>
                    <span className={`${styles.tierBadge} ${styles.tierCanon}`}>canon</span>
                    {f.canon}
                  </span>
                  {f.sem > 0 && (
                    <span>
                      <span className={`${styles.tierBadge} ${styles.tierSem}`}>sem</span>
                      {f.sem}
                    </span>
                  )}
                  <span>
                    <span className={`${styles.tierBadge} ${styles.tierDiv}`}>div</span>
                    {f.div}
                  </span>
                  {f.skip > 0 && (
                    <span>
                      <span className={`${styles.tierBadge} ${styles.tierSkip}`}>skip</span>
                      {f.skip}
                    </span>
                  )}
                  <span>· {f.total} total</span>
                </span>
                <span className={styles.expandHint}>
                  {fixtures.length > 0 ? (open ? "▾ collapse" : `▸ ${fixtures.length}`) : ""}
                </span>
              </div>
              {open && fixtures.length > 0 && (
                <div className={styles.fixtureList}>
                  {fixtures.map((d) => {
                    const sev: Severity | null =
                      d.annotation?.severity ?? (isUnannotatedDivergence(d) ? "unknown" : null);
                    return (
                      <div key={d.fixture} className={styles.fixtureItem}>
                        <div className={styles.fixtureMeta}>
                          <div className={styles.fixtureName}>{d.fixture}</div>
                          <div className={styles.fixtureTiers}>
                            <span className={`${styles.tierBadge} ${tierClass[d.achieved] ?? ""}`}>
                              {tierShort[d.achieved] ?? d.achieved}
                            </span>
                            {sev && (
                              <span
                                className={`${styles.severityBadge} ${severityClass[sev]}`}
                                title={severityDescription[sev]}
                              >
                                {severityLabel[sev]}
                              </span>
                            )}
                            {d.annotation?.issue_url && (
                              <a
                                className={styles.annotationIssue}
                                href={d.annotation.issue_url}
                                target="_blank"
                                rel="noreferrer noopener"
                                title={`GitHub issue #${d.annotation.issue}`}
                              >
                                #{d.annotation.issue}
                              </a>
                            )}
                            {d.annotation?.spec_ref && (
                              <span className={styles.annotationSpec} title={d.annotation.spec_ref}>
                                {d.annotation.spec_ref}
                              </span>
                            )}
                          </div>
                          <div className={styles.fixtureSizes}>
                            <span>
                              got <b>{formatBytes(d.got_size)}</b>
                            </span>
                            <span>
                              ref <b>{formatBytes(d.ref_size)}</b>
                            </span>
                            <span
                              className={
                                d.delta > 0
                                  ? styles.deltaPositive
                                  : d.delta < 0
                                    ? styles.deltaNegative
                                    : undefined
                              }
                            >
                              Δ{" "}
                              <b>
                                {d.delta > 0 ? `+${formatBytes(d.delta)}` : formatBytes(d.delta)}
                              </b>
                            </span>
                          </div>
                          {d.annotation?.summary && (
                            <div className={styles.annotationSummary}>{d.annotation.summary}</div>
                          )}
                          {isUnannotatedDivergence(d) && (
                            <div className={styles.annotationMissing}>
                              ⚠ Unannotated divergence — add an entry to{" "}
                              <code>core/formats/{f.format}/parity-annotations.yaml</code> so the CI
                              gate stops failing.
                            </div>
                          )}
                        </div>
                        <div className={styles.fixtureDiff}>
                          <DiffView reason={d.reason} />
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
        {visible.length === 0 && <p>No (format, engine) rows match the current filters.</p>}
      </main>
    </Layout>
  );
}
