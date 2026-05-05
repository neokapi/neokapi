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
                        <td className={styles.reasonCell}>{d.reason}</td>
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
