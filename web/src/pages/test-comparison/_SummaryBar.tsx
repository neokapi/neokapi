import { useMemo } from "react";
import type { FilterComparison, Summary, SkipCategory, StateFilter } from "./_types";
import { skipCategoryLabels, skipCategoryColors } from "./_types";
import styles from "./_index.module.css";

interface Props {
  summary: Summary;
  generatedAt: string;
  filters?: FilterComparison[];
  stateFilter?: StateFilter;
  onStateFilter?: (state: StateFilter) => void;
}

interface AggregateStats {
  implemented: number;
  notApplicable: number;
  okapiSkipped: number;
  pending: number;
  unmapped: number;
  bridgeAndNative: number;
  bridgeOnly: number;
  nativeOnly: number;
  categoryCounts: Record<string, number>;
  total: number;
}

function aggregateStats(filters: FilterComparison[]): AggregateStats {
  const stats: AggregateStats = {
    implemented: 0,
    notApplicable: 0,
    okapiSkipped: 0,
    pending: 0,
    unmapped: 0,
    bridgeAndNative: 0,
    bridgeOnly: 0,
    nativeOnly: 0,
    categoryCounts: {},
    total: 0,
  };

  for (const f of filters) {
    for (const tc of f.testCases) {
      stats.total++;
      switch (tc.testState) {
        case "implemented":
          stats.implemented++;
          break;
        case "pending":
          stats.pending++;
          break;
        case "skipped":
          stats.notApplicable++;
          break;
        default:
          // Check for not-applicable (has skipReason) vs truly unmapped
          if (tc.skipReason) {
            stats.notApplicable++;
          } else {
            stats.unmapped++;
          }
      }

      // Track bridge/native overlap
      const hasBridge = !!tc.bridgeTest;
      const hasNative = !!tc.nativeTest;
      if (hasBridge && hasNative) stats.bridgeAndNative++;
      else if (hasBridge) stats.bridgeOnly++;
      else if (hasNative) stats.nativeOnly++;

      // Track categories
      if (tc.skipCategory) {
        stats.categoryCounts[tc.skipCategory] = (stats.categoryCounts[tc.skipCategory] || 0) + 1;
      }
    }
  }

  // Merge in summary-level category counts if test-case level is empty
  if (
    Object.keys(stats.categoryCounts).length === 0 &&
    filters.some((f) => f.coverage?.categoryCounts)
  ) {
    for (const f of filters) {
      if (f.coverage?.categoryCounts) {
        for (const [cat, count] of Object.entries(f.coverage.categoryCounts)) {
          stats.categoryCounts[cat] = (stats.categoryCounts[cat] || 0) + count;
        }
      }
    }
  }

  return stats;
}

/** Stacked horizontal bar with colored segments. */
function StackedBar({
  segments,
  total,
  height = 20,
}: {
  segments: { value: number; color: string; label: string }[];
  total: number;
  height?: number;
}) {
  if (total === 0) return null;
  return (
    <div
      className={styles.stackedBar}
      style={{ height }}
      title={segments
        .filter((s) => s.value > 0)
        .map((s) => `${s.label}: ${s.value} (${((s.value / total) * 100).toFixed(1)}%)`)
        .join("\n")}
    >
      {segments
        .filter((s) => s.value > 0)
        .map((s, i) => (
          <div
            key={i}
            className={styles.stackedBarSegment}
            style={{
              width: `${(s.value / total) * 100}%`,
              backgroundColor: s.color,
            }}
            title={`${s.label}: ${s.value} (${((s.value / total) * 100).toFixed(1)}%)`}
          />
        ))}
    </div>
  );
}

/** Category breakdown mini-chart. */
function CategoryBreakdown({
  categoryCounts,
  total,
}: {
  categoryCounts: Record<string, number>;
  total: number;
}) {
  const sorted = Object.entries(categoryCounts).sort(([, a], [, b]) => b - a);
  if (sorted.length === 0) return null;

  return (
    <div className={styles.categoryBreakdown}>
      <div className={styles.categoryTitle}>Skip Reasons</div>
      <div className={styles.categoryList}>
        {sorted.map(([cat, count]) => {
          const label = skipCategoryLabels[cat as SkipCategory] ?? cat;
          const color = skipCategoryColors[cat as SkipCategory] ?? "#94a3b8";
          const pct = total > 0 ? ((count / total) * 100).toFixed(0) : "0";
          return (
            <div key={cat} className={styles.categoryItem}>
              <div className={styles.categoryBar}>
                <div
                  className={styles.categoryBarFill}
                  style={{
                    width: `${(count / sorted[0][1]) * 100}%`,
                    backgroundColor: color,
                  }}
                />
              </div>
              <span className={styles.categoryDot} style={{ backgroundColor: color }} />
              <span className={styles.categoryLabel}>{label}</span>
              <span className={styles.categoryCount}>
                {count} <span className={styles.categoryPct}>({pct}%)</span>
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

export default function SummaryBar({
  summary,
  generatedAt,
  filters,
  stateFilter,
  onStateFilter,
}: Props) {
  const stats = useMemo(() => (filters ? aggregateStats(filters) : null), [filters]);

  const coverageSegments = stats
    ? [
        { value: stats.implemented, color: "#2e8555", label: "Implemented" },
        {
          value: stats.notApplicable,
          color: "#94a3b8",
          label: "Not Applicable",
        },
        { value: stats.pending, color: "#e3a008", label: "Pending" },
        { value: stats.unmapped, color: "#dc2626", label: "Unmapped" },
      ]
    : [];

  const overlapSegments = stats
    ? [
        {
          value: stats.bridgeAndNative,
          color: "#2e8555",
          label: "Bridge + Native",
        },
        { value: stats.bridgeOnly, color: "#3b82f6", label: "Bridge Only" },
        { value: stats.nativeOnly, color: "#8b5cf6", label: "Native Only" },
      ]
    : [];

  return (
    <div className={styles.summaryWrap}>
      {/* Top row: key stats */}
      <div className={styles.summaryBar}>
        <div className={styles.statCard}>
          <div className={styles.statValue}>{summary.totalTestsOkapi}</div>
          <div className={styles.statLabel}>Okapi Tests</div>
        </div>
        {stats ? (
          <>
            <div
              className={`${styles.statCard} ${styles.statCardClickable} ${stateFilter === "implemented" ? styles.statCardActive : ""}`}
              onClick={() => onStateFilter?.("implemented")}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => e.key === "Enter" && onStateFilter?.("implemented")}
            >
              <div className={`${styles.statValue} ${styles.statGreen}`}>{stats.implemented}</div>
              <div className={styles.statLabel}>Implemented</div>
            </div>
            <div
              className={styles.statCard}
              title="Share of Okapi tests that are accounted for: mapped to a native test, in progress, or reviewed as not-applicable (i.e. not a bare gap)."
            >
              <div className={`${styles.statValue} ${styles.statGreen}`}>
                {stats.total > 0
                  ? `${(((stats.total - stats.unmapped) / stats.total) * 100).toFixed(1)}%`
                  : "—"}
              </div>
              <div className={styles.statLabel}>Reviewed</div>
            </div>
            <div
              className={`${styles.statCard} ${styles.statCardClickable} ${stateFilter === "not-applicable" ? styles.statCardActive : ""}`}
              onClick={() => onStateFilter?.("not-applicable")}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => e.key === "Enter" && onStateFilter?.("not-applicable")}
            >
              <div className={styles.statValue}>{stats.notApplicable}</div>
              <div className={styles.statLabel}>Not Applicable</div>
            </div>
            <div
              className={`${styles.statCard} ${styles.statCardClickable} ${stateFilter === "unmapped" ? styles.statCardActive : ""}`}
              onClick={() => onStateFilter?.("unmapped")}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => e.key === "Enter" && onStateFilter?.("unmapped")}
            >
              <div className={`${styles.statValue} ${stats.unmapped > 0 ? styles.statRed : ""}`}>
                {stats.unmapped}
              </div>
              <div className={styles.statLabel}>Unmapped</div>
            </div>
            {stats.pending > 0 && (
              <div
                className={`${styles.statCard} ${styles.statCardClickable} ${stateFilter === "pending" ? styles.statCardActive : ""}`}
                onClick={() => onStateFilter?.("pending")}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => e.key === "Enter" && onStateFilter?.("pending")}
              >
                <div className={`${styles.statValue} ${styles.statYellow}`}>{stats.pending}</div>
                <div className={styles.statLabel}>Pending</div>
              </div>
            )}
          </>
        ) : (
          <>
            <div className={styles.statCard}>
              <div className={styles.statValue}>
                {summary.totalFuncsBridge ?? summary.totalTestsBridge}
              </div>
              <div className={styles.statLabel}>Bridge Tests</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statValue}>
                {summary.totalFuncsNative ?? summary.totalTestsNative}
              </div>
              <div className={styles.statLabel}>Native Tests</div>
            </div>
          </>
        )}
        <div className={styles.statCard}>
          <div className={styles.statValue}>
            {summary.coveragePct > 0 ? `${summary.coveragePct.toFixed(1)}%` : "\u2014"}
          </div>
          <div className={styles.statLabel}>Coverage</div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Generated</div>
          <div className={styles.statDate}>{new Date(generatedAt).toLocaleDateString()}</div>
        </div>
      </div>

      {/* Coverage state bar */}
      {stats && stats.total > 0 && (
        <div className={styles.summaryBarsRow}>
          <div className={styles.summaryBarSection}>
            <div className={styles.barSectionLabel}>Coverage State</div>
            <StackedBar segments={coverageSegments} total={stats.total} />
            <div className={styles.barLegend}>
              {coverageSegments
                .filter((s) => s.value > 0)
                .map((s) => (
                  <span key={s.label} className={styles.legendItem}>
                    <span className={styles.legendDot} style={{ backgroundColor: s.color }} />
                    {s.label} ({s.value})
                  </span>
                ))}
            </div>
          </div>

          <div className={styles.summaryBarSection}>
            <div className={styles.barSectionLabel}>Implementation Coverage</div>
            <StackedBar
              segments={overlapSegments}
              total={stats.bridgeAndNative + stats.bridgeOnly + stats.nativeOnly}
            />
            <div className={styles.barLegend}>
              {overlapSegments
                .filter((s) => s.value > 0)
                .map((s) => (
                  <span key={s.label} className={styles.legendItem}>
                    <span className={styles.legendDot} style={{ backgroundColor: s.color }} />
                    {s.label} ({s.value})
                  </span>
                ))}
            </div>
          </div>
        </div>
      )}

      {/* Category breakdown */}
      {stats && Object.keys(stats.categoryCounts).length > 0 && stats.notApplicable > 0 && (
        <CategoryBreakdown categoryCounts={stats.categoryCounts} total={stats.notApplicable} />
      )}
    </div>
  );
}
