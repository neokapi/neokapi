import { useMemo, useState } from "react";
import Layout from "@theme/Layout";
import maturity from "@site/static/data/format-maturity.json";
import history from "@site/static/data/format-maturity-history.json";
import styles from "./index.module.css";
import {
  type MaturityData,
  type HistorySnapshot,
  type FormatRow,
  type Level,
  type DimScore,
  LEVELS,
  LEVEL_NAME,
} from "./_types";

const data = maturity as unknown as MaturityData;
const hist = history as unknown as HistorySnapshot[];

const lvlClass: Record<Level, string> = {
  L0: styles.lvlL0,
  L1: styles.lvlL1,
  L2: styles.lvlL2,
  L3: styles.lvlL3,
  L4: styles.lvlL4,
};

const dotClass: Record<DimScore, string> = {
  complete: styles.dotComplete,
  partial: styles.dotPartial,
  none: styles.dotNone,
  na: styles.dotNa,
};

const dotTitle: Record<DimScore, string> = {
  complete: "complete",
  partial: "partial",
  none: "missing",
  na: "not applicable",
};

function DistBar() {
  const { by_level, total } = data.summary;
  return (
    <>
      <div className={styles.distBar} role="img" aria-label="level distribution">
        {LEVELS.map((lv) => {
          const n = by_level[lv] ?? 0;
          if (!n) return null;
          const pct = (n / total) * 100;
          return (
            <div
              key={lv}
              className={`${styles.distSeg} ${lvlClass[lv]}`}
              style={{ width: `${pct}%` }}
              title={`${lv} ${LEVEL_NAME[lv]}: ${n}`}
            >
              {pct > 6 ? `${lv} · ${n}` : n}
            </div>
          );
        })}
      </div>
      <div className={styles.legend}>
        {LEVELS.map((lv) => (
          <span key={lv} className={styles.legendItem}>
            <span className={`${styles.swatch} ${lvlClass[lv]}`} />
            <strong>{lv}</strong> {LEVEL_NAME[lv]} ({by_level[lv] ?? 0})
          </span>
        ))}
      </div>
    </>
  );
}

function Trend() {
  if (!hist || hist.length === 0) return null;
  const maxTotal = Math.max(...hist.map((h) => h.total), 1);
  return (
    <>
      <h2>Progress over time</h2>
      <p className={styles.subtitle}>
        Each bar is one run of the <code>format-triage</code> workflow. Watch the green (L3) and
        teal (L4) grow as formats are hardened.
      </p>
      <div className={styles.trend}>
        {hist.map((h) => (
          <div className={styles.trendCol} key={h.date} title={h.date}>
            {LEVELS.map((lv) => {
              const n = h.by_level[lv] ?? 0;
              if (!n) return null;
              return (
                <div
                  key={lv}
                  className={`${styles.trendCell} ${lvlClass[lv]}`}
                  style={{ height: `${(n / maxTotal) * 100}%` }}
                  title={`${h.date} · ${lv}: ${n}`}
                />
              );
            })}
            <span className={styles.trendDate}>{h.date.slice(5)}</span>
          </div>
        ))}
      </div>
    </>
  );
}

export default function FormatMaturity() {
  const [search, setSearch] = useState("");
  const [level, setLevel] = useState<Level | null>(null);
  const [type, setType] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  const types = useMemo(() => Array.from(new Set(data.formats.map((f) => f.type))).sort(), []);

  const rows = useMemo(() => {
    const q = search.trim().toLowerCase();
    return data.formats
      .filter((f) => (level ? f.level === level : true))
      .filter((f) => (type ? f.type === type : true))
      .filter((f) => (q ? f.id.includes(q) : true))
      .sort((a, b) => a.level.localeCompare(b.level) || a.id.localeCompare(b.id));
  }, [search, level, type]);

  return (
    <Layout
      title="Format Maturity"
      description="Maturity level (L0–L4) of every neokapi format against the format maturity rubric."
    >
      <main className={styles.page}>
        <div className={styles.header}>
          <h1>Format Maturity</h1>
          <p className={styles.subtitle}>
            Where every neokapi format sits against the{" "}
            <a href="https://github.com/neokapi/neokapi/blob/main/docs/internals/format-maturity.md">
              maturity rubric
            </a>{" "}
            (L0 experimental → L4 rock-solid). Target:{" "}
            <span className={`${styles.levelBadge} ${lvlClass[data.target_level]}`}>
              {data.target_level}
            </span>
          </p>
          <p className={styles.meta}>
            {data.summary.total} formats · generated {data.generated_at} · source: {data.source}
          </p>
        </div>

        <DistBar />
        <Trend />

        <div className={styles.controls}>
          <input
            className={styles.search}
            placeholder="Filter formats…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          {LEVELS.map((lv) => (
            <button
              key={lv}
              className={`${styles.chip} ${level === lv ? styles.chipActive : ""}`}
              onClick={() => setLevel(level === lv ? null : lv)}
            >
              {lv} ({data.summary.by_level[lv] ?? 0})
            </button>
          ))}
          {types.map((t) => (
            <button
              key={t}
              className={`${styles.chip} ${type === t ? styles.chipActive : ""}`}
              onClick={() => setType(type === t ? null : t)}
            >
              {t}
            </button>
          ))}
        </div>

        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>Format</th>
                <th>Level</th>
                {data.dimensions.map((d) => (
                  <th key={d} className={styles.dimHead}>
                    {data.dimension_labels[d] ?? d}
                  </th>
                ))}
                <th>Next gap</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((f: FormatRow) => (
                <RowGroup
                  key={f.id}
                  f={f}
                  open={expanded === f.id}
                  onToggle={() => setExpanded(expanded === f.id ? null : f.id)}
                />
              ))}
            </tbody>
          </table>
        </div>
        {rows.length === 0 && <p>No formats match the current filters.</p>}
      </main>
    </Layout>
  );
}

function RowGroup({ f, open, onToggle }: { f: FormatRow; open: boolean; onToggle: () => void }) {
  const colSpan = 3 + data.dimensions.length;
  return (
    <>
      <tr className={styles.rowExpand} onClick={onToggle}>
        <td>
          <span className={styles.fmtId}>{f.id}</span>
          <div className={styles.typeTag}>
            {f.type}
            {f.okapi_counterpart ? ` · ${f.okapi_counterpart}` : ""}
          </div>
        </td>
        <td>
          <span className={`${styles.levelBadge} ${lvlClass[f.level]}`}>{f.level}</span>
        </td>
        {data.dimensions.map((d) => {
          const s = (f.dimensions[d] ?? "none") as DimScore;
          return (
            <td key={d} style={{ textAlign: "center" }}>
              <span
                className={`${styles.dot} ${dotClass[s]}`}
                title={`${data.dimension_labels[d] ?? d}: ${dotTitle[s]}`}
              />
            </td>
          );
        })}
        <td className={styles.gapCell}>{f.blocking_gaps[0] ?? "—"}</td>
      </tr>
      {open && (
        <tr className={styles.detail}>
          <td colSpan={colSpan}>
            <strong>
              {f.id} — {f.level} → {f.next_level}
            </strong>
            {f.blocking_gaps.length > 0 && (
              <ul>
                {f.blocking_gaps.map((g, i) => (
                  <li key={i}>{g}</li>
                ))}
              </ul>
            )}
            {f.top_risk && (
              <p className={styles.riskText}>
                <strong>Top risk:</strong> {f.top_risk}
              </p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}
