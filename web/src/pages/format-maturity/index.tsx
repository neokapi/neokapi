import { useMemo, useState } from "react";
import Layout from "@theme/Layout";
import maturity from "@site/static/data/format-maturity.json";
import history from "@site/static/data/format-maturity-history.json";
import styles from "./index.module.css";
import {
  type MaturityData,
  type HistorySnapshot,
  type FormatRow,
  type DimScore,
  type AxisId,
  type Grade,
  type SupportTier,
  type TierInfo,
  LEVELS,
  LEVEL_NAME,
  AXIS_IDS,
  AXIS_LABEL,
  AXIS_GRADES,
  AXIS_DIMS,
  GRADE_NAME,
  FAMILY_ORDER,
  FAMILY_LABEL,
  FAMILY_TAGLINE,
  FAMILY_AXES,
  TIER_ORDER,
  TIER_LABEL,
  TIER_STALE_DAYS,
  TIER_DECAY_DAYS,
} from "./_types";

// The committed dataset is still scorer v1 (no `scorer_version`; rows carry
// no `levels`/`dims`/`tier`), so the double-cast must stay: resolveJsonModule
// widens every string literal to `string`, which can never satisfy the
// `Level`/`DimScore` unions, regardless of dataset version. All v2/v3 fields
// are optional in `MaturityData`, so this cast is the single unchecked seam —
// every usage site below typechecks against the unions and guards the
// additive fields.
const data = maturity as unknown as MaturityData;
const hist = history as unknown as HistorySnapshot[];

const gradeClass: Record<Grade, string> = {
  L0: styles.lvlL0,
  L1: styles.lvlL1,
  L2: styles.lvlL2,
  L3: styles.lvlL3,
  L4: styles.lvlL4,
  V0: styles.lvlV0,
  V1: styles.lvlV1,
  V2: styles.lvlV2,
  V3: styles.lvlV3,
  E0: styles.lvlE0,
  E1: styles.lvlE1,
  E2: styles.lvlE2,
  E3: styles.lvlE3,
  E4: styles.lvlE4,
  K0: styles.lvlK0,
  K1: styles.lvlK1,
  K2: styles.lvlK2,
  K3: styles.lvlK3,
  C0: styles.lvlC0,
  C1: styles.lvlC1,
  C2: styles.lvlC2,
  C3: styles.lvlC3,
  S0: styles.lvlS0,
  S1: styles.lvlS1,
  S2: styles.lvlS2,
  S3: styles.lvlS3,
  S4: styles.lvlS4,
  G0: styles.lvlG0,
  G1: styles.lvlG1,
  G2: styles.lvlG2,
  G3: styles.lvlG3,
  G4: styles.lvlG4,
};

const tierClass: Record<SupportTier, string> = {
  supported: styles.tierSupported,
  maintained: styles.tierMaintained,
  available: styles.tierAvailable,
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

/** The published grade of a row on one axis. Engine always exists (`level`
 * mirrors it in every scorer version); other axes only on v3 rows. */
function gradeOf(f: FormatRow, axis: AxisId): Grade | undefined {
  return axis === "engine" ? f.level : f.levels?.[axis];
}

function axisLabel(a: AxisId): string {
  return data.axis_labels?.[a] ?? AXIS_LABEL[a];
}

/** Dimension columns for one axis: dataset `dimension_axes` metadata when
 * present (v3); else the legacy flat list for engine (v1/v2), or whatever
 * per-axis grids the rows actually carry, in canonical order. */
function dimsForAxis(axis: AxisId): string[] {
  const da = data.dimension_axes;
  if (da) {
    const mapped = data.dimensions.filter((d) => da[d] === axis);
    if (mapped.length > 0) return mapped;
  }
  if (axis === "engine") return data.dimensions;
  const seen = new Set<string>();
  for (const f of data.formats) {
    for (const d of Object.keys(f.dims?.[axis] ?? {})) seen.add(d);
  }
  const canon = AXIS_DIMS[axis].filter((d) => seen.has(d));
  const extra = Array.from(seen)
    .filter((d) => !AXIS_DIMS[axis].includes(d))
    .sort();
  return [...canon, ...extra];
}

function dimScore(f: FormatRow, axis: AxisId, d: string): DimScore {
  if (axis === "engine") return f.dims?.engine?.[d] ?? f.dimensions[d] ?? "none";
  return f.dims?.[axis]?.[d] ?? "none";
}

// ── support-tier staleness (client-side, rubric §1) ─────────────────────────

function daysBetween(fromISO: string, toISO: string): number | null {
  const from = Date.parse(fromISO);
  const to = Date.parse(toISO);
  if (Number.isNaN(from) || Number.isNaN(to)) return null;
  return Math.floor((to - from) / 86_400_000);
}

type TierState = "fresh" | "stale" | "decayed";

/** Staleness from `generated_at − last_certified`. A null/absent
 * `last_certified` (never certified — bootstrap/grandfathered) has no
 * baseline, so no decay is computed. */
function tierState(tier: TierInfo, generatedAt: string): { state: TierState; days: number | null } {
  if (!tier.last_certified) return { state: "fresh", days: null };
  const days = daysBetween(tier.last_certified, generatedAt);
  if (days === null) return { state: "fresh", days: null };
  if (days > TIER_DECAY_DAYS) return { state: "decayed", days };
  if (days > TIER_STALE_DAYS) return { state: "stale", days };
  return { state: "fresh", days };
}

/** One tier down the demotion ladder. `available` has no dashboard tier below
 * it (retirement is a product decision, not an audit outcome), so it floors. */
function decayTier(t: SupportTier): SupportTier {
  const i = TIER_ORDER.indexOf(t);
  return TIER_ORDER[Math.min(i + 1, TIER_ORDER.length - 1)] ?? t;
}

function TierBadge({ tier }: { tier?: TierInfo }) {
  if (!tier) return <span className={styles.gradeMissing}>—</span>;
  const { state, days } = tierState(tier, data.generated_at);
  const certified = tier.last_certified
    ? `last certified ${tier.last_certified}${days !== null ? ` (${days}d before this snapshot)` : ""}`
    : "never certified";
  if (state === "decayed") {
    const shown = decayTier(tier.declared);
    const title = `Declared ${TIER_LABEL[tier.declared]} · ${certified} — older than ${TIER_DECAY_DAYS}d, displayed one tier down; tier-review due.`;
    if (shown === tier.declared) {
      return (
        <span className={styles.tierCell} title={title}>
          <span className={`${styles.tierBadge} ${tierClass[tier.declared]} ${styles.tierStale}`}>
            {TIER_LABEL[tier.declared]}
          </span>
          <span className={styles.decayNote}>decayed</span>
        </span>
      );
    }
    return (
      <span className={styles.tierCell} title={title}>
        <span
          className={`${styles.tierBadge} ${tierClass[tier.declared]} ${styles.tierDecayedDeclared}`}
        >
          {TIER_LABEL[tier.declared]}
        </span>
        <span className={`${styles.tierBadge} ${tierClass[shown]} ${styles.tierDecayed}`}>
          {TIER_LABEL[shown]}
        </span>
      </span>
    );
  }
  return (
    <span
      className={styles.tierCell}
      title={`${TIER_LABEL[tier.declared]}${tier.since ? ` since ${tier.since}` : ""} · ${certified}${
        state === "stale" ? ` — stale (>${TIER_STALE_DAYS}d)` : ""
      }`}
    >
      <span
        className={`${styles.tierBadge} ${tierClass[tier.declared]} ${
          state === "stale" ? styles.tierStale : ""
        }`}
      >
        {TIER_LABEL[tier.declared]}
      </span>
      {state === "stale" && <span className={styles.staleNote}>stale</span>}
    </span>
  );
}

// ── summary distributions ────────────────────────────────────────────────────

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
              className={`${styles.distSeg} ${gradeClass[lv]}`}
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
            <span className={`${styles.swatch} ${gradeClass[lv]}`} />
            <strong>{lv}</strong> {LEVEL_NAME[lv]} ({by_level[lv] ?? 0})
          </span>
        ))}
      </div>
    </>
  );
}

/** Per-axis mini distribution bars from `summary.by_axis` (v3+ datasets only;
 * the engine distribution already has the headline bar above), grouped under
 * the three axis families. Each family renders only the axes the dataset
 * actually carries, so a v3 (6-axis) dataset still groups cleanly. */
function AxisBars() {
  const byAxis = data.summary.by_axis ?? {};
  // Engine is excluded (its distribution is the headline bar).
  const families = FAMILY_ORDER.map((fam) => ({
    fam,
    axes: FAMILY_AXES[fam].filter((a) => a !== "engine" && byAxis[a]),
  })).filter((x) => x.axes.length > 0);
  if (families.length === 0) return null;
  return (
    <div className={styles.axisBars}>
      {families.map(({ fam, axes }) => (
        <div key={fam} className={styles.axisBarFamily}>
          <span className={styles.familyHeader} title={FAMILY_TAGLINE[fam]}>
            {FAMILY_LABEL[fam]}
          </span>
          {axes.map((a) => {
            const dist = byAxis[a] ?? {};
            return (
              <div key={a} className={styles.axisBarRow}>
                <span className={styles.axisBarLabel}>{axisLabel(a)}</span>
                <div
                  className={styles.miniBar}
                  role="img"
                  aria-label={`${axisLabel(a)} distribution`}
                >
                  {AXIS_GRADES[a].map((g) => {
                    const n = dist[g] ?? 0;
                    if (!n) return null;
                    const pct = (n / data.summary.total) * 100;
                    return (
                      <div
                        key={g}
                        className={`${styles.miniSeg} ${gradeClass[g]}`}
                        style={{ width: `${pct}%` }}
                        title={`${g} ${GRADE_NAME[g]}: ${n}`}
                      >
                        {pct > 8 ? `${g} · ${n}` : ""}
                      </div>
                    );
                  })}
                </div>
              </div>
            );
          })}
        </div>
      ))}
    </div>
  );
}

// ── trends ──────────────────────────────────────────────────────────────────

function Trend() {
  if (!hist || hist.length === 0) return null;
  const maxTotal = Math.max(...hist.map((h) => h.total), 1);
  // Axis lanes start at the first multi-axis snapshot; older entries carry no
  // by_axis and simply contribute nothing.
  const axisSnaps = hist.filter((h) => h.by_axis);
  const trendAxes = AXIS_IDS.filter((a) => a !== "engine" && axisSnaps.some((h) => h.by_axis?.[a]));
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
                  className={`${styles.trendCell} ${gradeClass[lv]}`}
                  style={{ height: `${(n / maxTotal) * 100}%` }}
                  title={`${h.date} · ${lv}: ${n}`}
                />
              );
            })}
            <span className={styles.trendDate}>{h.date.slice(5)}</span>
          </div>
        ))}
      </div>
      {trendAxes.length > 0 && (
        <div className={styles.axisTrends}>
          {trendAxes.map((a) => (
            <div className={styles.axisTrendRow} key={a}>
              <span className={styles.axisBarLabel}>{axisLabel(a)}</span>
              <div className={styles.axisTrendLane}>
                {axisSnaps.map((h) => (
                  <div className={styles.axisTrendCol} key={h.date} title={h.date}>
                    {AXIS_GRADES[a].map((g) => {
                      const n = h.by_axis?.[a]?.[g] ?? 0;
                      if (!n) return null;
                      return (
                        <div
                          key={g}
                          className={`${styles.trendCell} ${gradeClass[g]}`}
                          style={{ height: `${(n / maxTotal) * 100}%` }}
                          title={`${h.date} · ${g}: ${n}`}
                        />
                      );
                    })}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </>
  );
}

// ── page ────────────────────────────────────────────────────────────────────

export default function FormatMaturity() {
  const [search, setSearch] = useState("");
  const [axis, setAxis] = useState<AxisId>("engine");
  const [grade, setGrade] = useState<Grade | null>(null);
  const [type, setType] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  // v3-only affordances: hidden entirely on v1/v2 datasets so the legacy
  // rendering path is byte-for-byte what it was.
  const hasAxes = useMemo(
    () => data.formats.some((f) => f.levels) || Boolean(data.summary.by_axis),
    [],
  );
  const hasTiers = useMemo(() => data.formats.some((f) => f.tier), []);

  // Axes the dataset actually carries (engine is always present; the rest only
  // on v3+ rows / summaries). A v3 (6-axis) dataset omits `structure`, so it
  // never surfaces in the selector, families, or legend.
  const presentAxes = useMemo<AxisId[]>(
    () =>
      AXIS_IDS.filter(
        (a) =>
          a === "engine" ||
          Boolean(data.axes?.[a]) ||
          Boolean(data.summary.by_axis?.[a]) ||
          data.formats.some((f) => f.levels?.[a]),
      ),
    [],
  );

  // Family-grouped grade-range legend, derived from the present axes so it
  // never drifts from `AXIS_GRADES` and carries no hardcoded counts.
  const axisLegend = useMemo(
    () =>
      FAMILY_ORDER.map((fam) => {
        const axes = FAMILY_AXES[fam].filter((a) => presentAxes.includes(a));
        if (axes.length === 0) return null;
        const parts = axes.map((a) => {
          const g = AXIS_GRADES[a];
          return `${axisLabel(a)} ${g[0]}–${g[g.length - 1]}`;
        });
        return `${FAMILY_LABEL[fam]} (${parts.join(", ")})`;
      })
        .filter(Boolean)
        .join("; "),
    [presentAxes],
  );

  const types = useMemo(() => Array.from(new Set(data.formats.map((f) => f.type))).sort(), []);

  const dimCols = useMemo(() => dimsForAxis(axis), [axis]);

  const gradeCounts = useMemo(() => {
    const counts: Partial<Record<Grade, number>> = {};
    for (const f of data.formats) {
      const g = gradeOf(f, axis);
      if (g) counts[g] = (counts[g] ?? 0) + 1;
    }
    return counts;
  }, [axis]);

  const rows = useMemo(() => {
    const q = search.trim().toLowerCase();
    // Rank strictly within the selected axis's ladder — grade strings from
    // different alphabets (V2 vs L2) must never be compared.
    const rank = (f: FormatRow) => {
      const g = gradeOf(f, axis);
      return g ? AXIS_GRADES[axis].indexOf(g) : -1;
    };
    return data.formats
      .filter((f) => (grade ? gradeOf(f, axis) === grade : true))
      .filter((f) => (type ? f.type === type : true))
      .filter((f) => (q ? f.id.includes(q) : true))
      .sort((a, b) => rank(a) - rank(b) || a.id.localeCompare(b.id));
  }, [search, grade, type, axis]);

  return (
    <Layout
      title="Format Maturity"
      description="Maturity of every neokapi format against the format maturity rubric — comprehension, assurance, and enablement axis families plus the declared support tier."
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
            <span className={`${styles.levelBadge} ${gradeClass[data.target_level]}`}>
              {data.target_level}
            </span>
            {hasAxes && <> Axis grades by family — {axisLegend}.</>}
          </p>
          <p className={styles.meta}>
            {data.summary.total} formats · generated {data.generated_at} · source: {data.source}
            {data.scorer_version ? ` · scorer v${data.scorer_version}` : ""}
          </p>
        </div>

        <DistBar />
        <AxisBars />
        <Trend />

        {hasAxes && (
          <div className={styles.axisSelect}>
            <span className={styles.axisSelectLabel}>Axis</span>
            {FAMILY_ORDER.map((fam) => {
              const axes = FAMILY_AXES[fam].filter((a) => presentAxes.includes(a));
              if (axes.length === 0) return null;
              return (
                <div key={fam} className={styles.axisFamily}>
                  <span className={styles.familyLabel} title={FAMILY_TAGLINE[fam]}>
                    {FAMILY_LABEL[fam]}
                  </span>
                  {axes.map((a) => (
                    <button
                      key={a}
                      className={`${styles.chip} ${axis === a ? styles.chipActive : ""}`}
                      onClick={() => {
                        setAxis(a);
                        setGrade(null);
                      }}
                    >
                      {axisLabel(a)}
                    </button>
                  ))}
                </div>
              );
            })}
          </div>
        )}

        <div className={styles.controls}>
          <input
            className={styles.search}
            placeholder="Filter formats…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          {AXIS_GRADES[axis].map((g) => (
            <button
              key={g}
              className={`${styles.chip} ${grade === g ? styles.chipActive : ""}`}
              onClick={() => setGrade(grade === g ? null : g)}
            >
              {g} ({gradeCounts[g] ?? 0})
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
                <th>{axis === "engine" ? "Level" : axisLabel(axis)}</th>
                {hasTiers && <th>Tier</th>}
                {dimCols.map((d) => (
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
                  axis={axis}
                  dimCols={dimCols}
                  hasTiers={hasTiers}
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

function RowGroup({
  f,
  axis,
  dimCols,
  hasTiers,
  open,
  onToggle,
}: {
  f: FormatRow;
  axis: AxisId;
  dimCols: string[];
  hasTiers: boolean;
  open: boolean;
  onToggle: () => void;
}) {
  const colSpan = 3 + (hasTiers ? 1 : 0) + dimCols.length;
  const g = gradeOf(f, axis);
  const next = axis === "engine" ? f.next_level : (f.next?.[axis] ?? "—");
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
          {g ? (
            <span className={`${styles.levelBadge} ${gradeClass[g]}`}>{g}</span>
          ) : (
            <span className={styles.gradeMissing}>—</span>
          )}
          {f.levels && (
            <div className={styles.axisProfile}>
              {FAMILY_ORDER.map((fam) => {
                const badges = FAMILY_AXES[fam]
                  .map((a) => ({ a, ag: gradeOf(f, a) }))
                  .filter((x): x is { a: AxisId; ag: Grade } => Boolean(x.ag));
                if (badges.length === 0) return null;
                return (
                  <span
                    key={fam}
                    className={styles.axisProfileGroup}
                    title={`${FAMILY_LABEL[fam]} — ${FAMILY_TAGLINE[fam]}`}
                  >
                    {badges.map(({ a, ag }) => (
                      <span
                        key={a}
                        className={`${styles.miniBadge} ${gradeClass[ag]}`}
                        title={`${axisLabel(a)}: ${ag} ${GRADE_NAME[ag]}`}
                      >
                        {ag}
                      </span>
                    ))}
                  </span>
                );
              })}
            </div>
          )}
        </td>
        {hasTiers && (
          <td>
            <TierBadge tier={f.tier} />
          </td>
        )}
        {dimCols.map((d) => {
          const s = dimScore(f, axis, d);
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
              {f.id} — {g ?? "—"} → {next}
            </strong>
            {f.blocking_gaps.length > 0 && (
              <ul>
                {f.blocking_gaps.map((gap, i) => (
                  <li key={i}>{gap}</li>
                ))}
              </ul>
            )}
            {f.top_risk && (
              <p className={styles.riskText}>
                <strong>Top risk:</strong> {f.top_risk}
              </p>
            )}
            {f.tier && f.tier.gates && f.tier.gates.length > 0 && (
              <p className={styles.riskText}>
                <strong>Tier gates:</strong> {f.tier.gates.join(", ")}
              </p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}
