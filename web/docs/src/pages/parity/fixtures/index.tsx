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
  divergent?: FixtureEntry[];
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

function rowKey(f: FormatBreakdown): string {
  return `${f.engine}/${f.format}`;
}

function formatBytes(n: number): string {
  return n.toLocaleString();
}

function formatOffset(raw: number, norm?: number, normalizer?: string): string {
  if (raw < 0 && (norm === undefined || norm < 0)) return "—";
  if (normalizer && norm !== undefined && norm >= 0) {
    return `raw@${raw}, norm@${norm}`;
  }
  return `@${raw}`;
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

// unescapeGoVisual reverses the Go %q escapes that exist purely as
// quote-safety noise — `\"` and `\\` — so the dashboard shows the
// actual file content. Newlines/tabs/control chars stay as `\n` /
// `\t` / `\xNN` literals so a single-line layout is preserved
// (otherwise the diff rows would break across the cell).
function unescapeGoVisual(s: string): string {
  let out = "";
  for (let i = 0; i < s.length; i++) {
    if (s[i] === "\\" && i + 1 < s.length) {
      const n = s[i + 1];
      if (n === '"' || n === "\\") {
        out += n;
        i++;
        continue;
      }
    }
    out += s[i];
  }
  return out;
}

type DiffSeg = { type: "common" | "del" | "ins"; text: string };

// diffChars returns the character-level diff between a (got) and b (ref)
// as a list of segments using a standard LCS DP backtrace. Segments are
// coalesced so a run of common (or all-del or all-ins) chars renders as
// one span. Bounded to ~256 chars per side — beyond that the snippets
// themselves are too long to be useful.
function diffChars(a: string, b: string): DiffSeg[] {
  const m = a.length;
  const n = b.length;
  const limit = 256;
  if (m > limit || n > limit) {
    // Fall back to whole-replacement view to keep the page snappy.
    const out: DiffSeg[] = [];
    if (a) out.push({ type: "del", text: a });
    if (b) out.push({ type: "ins", text: b });
    return out;
  }
  const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      if (a[i - 1] === b[j - 1]) {
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
    if (i > 0 && j > 0 && a[i - 1] === b[j - 1]) {
      segs.push({ type: "common", text: a[i - 1] });
      i--;
      j--;
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      segs.push({ type: "ins", text: b[j - 1] });
      j--;
    } else {
      segs.push({ type: "del", text: a[i - 1] });
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
  const got = unescapeGoVisual(parsed.got ?? "");
  const ref = unescapeGoVisual(parsed.ref ?? "");
  return (
    <div className={styles.diffBox}>
      {(parsed.zipEntry || parsed.context || parsed.normalizer || parsed.offset !== undefined) && (
        <div className={styles.diffMeta}>
          {parsed.offset !== undefined && (
            <span className={styles.diffOffset}>@{parsed.offset.toLocaleString()}</span>
          )}
          {parsed.context && <span className={styles.diffContext}>{parsed.context}</span>}
          {parsed.zipEntry && (
            <span className={styles.diffEntryChip}>zip:{parsed.zipEntry}</span>
          )}
          {parsed.normalizer && (
            <span className={styles.diffNormChip} title={parsed.normalizer}>
              norm:{parsed.normalizer.length > 60
                ? parsed.normalizer.slice(0, 57) + "…"
                : parsed.normalizer}
            </span>
          )}
        </div>
      )}
      <div className={styles.diffStack}>
        <div className={styles.diffSideRow}>
          <span className={styles.diffLabelGot}>got</span>
          <span className={styles.diffLine}>
            {diffChars(got, ref).map((seg, i) => {
              if (seg.type === "common") {
                return (
                  <span key={i} className={styles.diffCommon}>
                    {seg.text}
                  </span>
                );
              }
              if (seg.type === "del") {
                return (
                  <span key={i} className={styles.diffDelGot}>
                    {seg.text}
                  </span>
                );
              }
              // ins: render as transparent padding on the got line so
              // columns line up with the ref line below.
              return (
                <span key={i} className={styles.diffPad} aria-hidden="true">
                  {seg.text}
                </span>
              );
            })}
          </span>
        </div>
        <div className={styles.diffSideRow}>
          <span className={styles.diffLabelRef}>ref</span>
          <span className={styles.diffLine}>
            {diffChars(got, ref).map((seg, i) => {
              if (seg.type === "common") {
                return (
                  <span key={i} className={styles.diffCommon}>
                    {seg.text}
                  </span>
                );
              }
              if (seg.type === "ins") {
                return (
                  <span key={i} className={styles.diffDelRef}>
                    {seg.text}
                  </span>
                );
              }
              // del: render as transparent padding on the ref line.
              return (
                <span key={i} className={styles.diffPad} aria-hidden="true">
                  {seg.text}
                </span>
              );
            })}
          </span>
        </div>
      </div>
    </div>
  );
}


export default function ParityFixturesDashboard() {
  const [search, setSearch] = useState("");
  const [activeEngine, setActiveEngine] = useState<string | null>("native");
  const [onlyDivergent, setOnlyDivergent] = useState<boolean>(true);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const engineNames = useMemo(() => {
    const set = new Set<string>();
    for (const f of dataset.formats) set.add(f.engine);
    return Array.from(set).sort();
  }, []);

  const visible = useMemo<FormatBreakdown[]>(() => {
    const q = search.trim().toLowerCase();
    return dataset.formats
      .filter((f) => (activeEngine ? f.engine === activeEngine : true))
      .filter((f) => (onlyDivergent ? f.div > 0 : true))
      .filter((f) => {
        if (!q) return true;
        if (f.format.toLowerCase().includes(q)) return true;
        if (f.divergent?.some((d) => d.fixture.toLowerCase().includes(q))) return true;
        if (f.divergent?.some((d) => d.reason.toLowerCase().includes(q))) return true;
        return false;
      })
      .sort((a, b) => {
        if (a.div !== b.div) return b.div - a.div;
        if (a.format !== b.format) return a.format.localeCompare(b.format);
        return a.engine.localeCompare(b.engine);
      });
  }, [search, activeEngine, onlyDivergent]);

  const filteredFixtures = (f: FormatBreakdown): FixtureEntry[] => {
    const q = search.trim().toLowerCase();
    if (!q) return f.divergent ?? [];
    return (f.divergent ?? []).filter(
      (d) => d.fixture.toLowerCase().includes(q) || d.reason.toLowerCase().includes(q),
    );
  };

  const toggle = (k: string): void =>
    setExpanded((prev) => ({ ...prev, [k]: !prev[k] }));

  return (
    <Layout
      title="Parity Fixtures Drill-Down"
      description="Per-fixture parity divergence detail (first-diff offset, byte delta, snippet)"
    >
      <main className="container margin-vert--lg">
        <h1>Parity Fixtures Drill-Down</h1>
        <p className={styles.subtitle}>
          Per (format, engine, fixture) divergence detail produced by{" "}
          <code>make parity-fixtures</code>. Each expandable row lists the
          divergent fixtures with raw/normalized first-diff offsets and the
          comparison snippet so you can scan for patterns (line endings,
          whitespace, encoding…) without re-running the test. Generated{" "}
          <strong>{dataset.generated_at}</strong>. See also the higher-level{" "}
          <a href="/parity">/parity dashboard</a>.
        </p>

        <div className={styles.totals}>
          {Object.entries(dataset.engines).map(([eng, t]) => (
            <div key={eng} className={styles.totalCard}>
              <h2>{eng}</h2>
              <div className={styles.headline}>
                {t.byte} / {t.total - t.skip}{" "}
                <span className={styles.headlineSuffix}>
                  byte-equal ({t.byte_pct.toFixed(1)}%)
                </span>
              </div>
              <ul className={styles.totalBreakdown}>
                <li>
                  <span className={`${styles.tierBadge} ${styles.tierByte}`}>byte</span>{" "}
                  {t.byte}
                </li>
                <li>
                  <span className={`${styles.tierBadge} ${styles.tierCanon}`}>canon</span>{" "}
                  {t.canon}
                </li>
                {t.sem > 0 && (
                  <li>
                    <span className={`${styles.tierBadge} ${styles.tierSem}`}>sem</span>{" "}
                    {t.sem}
                  </li>
                )}
                <li>
                  <span className={`${styles.tierBadge} ${styles.tierDiv}`}>div</span>{" "}
                  {t.div}
                </li>
                {t.skip > 0 && (
                  <li>
                    <span className={`${styles.tierBadge} ${styles.tierSkip}`}>skip</span>{" "}
                    {t.skip}
                  </li>
                )}
              </ul>
            </div>
          ))}
        </div>

        <div className={styles.toolbar}>
          <input
            type="text"
            className={styles.search}
            placeholder="Filter by format, fixture name, or reason snippet..."
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
            <button
              className={`button button--sm ${onlyDivergent ? "button--primary" : "button--outline button--secondary"}`}
              onClick={() => setOnlyDivergent((v) => !v)}
            >
              Divergent only
            </button>
          </div>
        </div>

        <p className={styles.resultCount}>
          {visible.length} (format, engine) rows
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
                  {f.div > 0 ? (open ? "▾ collapse" : "▸ expand") : ""}
                </span>
              </div>
              {open && f.div > 0 && (
                <table className={styles.fixtureTable}>
                  <thead>
                    <tr>
                      <th>Fixture</th>
                      <th>Required</th>
                      <th>Achieved</th>
                      <th className={styles.numCell}>got</th>
                      <th className={styles.numCell}>ref</th>
                      <th className={styles.numCell}>Δ</th>
                      <th>First diff</th>
                      <th>Reason</th>
                    </tr>
                  </thead>
                  <tbody>
                    {fixtures.map((d) => (
                      <tr key={d.fixture}>
                        <td className={styles.fixtureName}>{d.fixture}</td>
                        <td>
                          <span className={`${styles.tierBadge} ${tierClass[d.required] ?? ""}`}>
                            {tierShort[d.required] ?? d.required}
                          </span>
                        </td>
                        <td>
                          <span className={`${styles.tierBadge} ${tierClass[d.achieved] ?? ""}`}>
                            {tierShort[d.achieved] ?? d.achieved}
                          </span>
                        </td>
                        <td className={styles.numCell}>{formatBytes(d.got_size)}</td>
                        <td className={styles.numCell}>{formatBytes(d.ref_size)}</td>
                        <td
                          className={`${styles.deltaCell} ${d.delta > 0 ? styles.deltaPositive : d.delta < 0 ? styles.deltaNegative : ""}`}
                        >
                          {d.delta > 0 ? `+${formatBytes(d.delta)}` : formatBytes(d.delta)}
                        </td>
                        <td className={styles.offsetCell}>
                          {formatOffset(d.raw_diff_offset, d.norm_diff_offset, d.normalizer)}
                        </td>
                        <td className={styles.reasonCell}>
                          <DiffView reason={d.reason} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          );
        })}
        {visible.length === 0 && <p>No (format, engine) rows match the current filters.</p>}
      </main>
    </Layout>
  );
}
