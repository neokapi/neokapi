import { useState, useMemo } from "react";
import type { FilterComparison, FilterResult, SpecSummary } from "./_types";
import TestCaseTable from "./_TestCaseTable";
import SpecSection from "./_SpecSection";
import styles from "./_index.module.css";

/** Fallback summary used when a filter has spec drift but no parity-run spec results. */
const emptySpecSummary: SpecSummary = {
  features: [],
  pass: 0,
  fail: 0,
  skip: 0,
  parityWarn: 0,
  expectedFail: 0,
};

interface Props {
  filter: FilterComparison;
  goCommitSHA?: string;
  okapiTag?: string;
  defaultExpanded?: boolean;
  defaultTestFilter?: string;
}

function statusColor(filter: FilterComparison): string {
  const sides = [filter.okapi, filter.bridge, filter.native].filter(Boolean);
  if (sides.length === 0) return styles.statusYellow;
  const hasFails = sides.some((s) => s != null && s.failed + s.errors > 0);
  if (hasFails) return styles.statusRed;
  if (sides.length === 1) return styles.statusYellow;
  return styles.statusGreen;
}

function StatColumn({ result }: { result: FilterResult | null }) {
  if (!result) {
    return (
      <div className={styles.sideColumn}>
        <span className={styles.noTests}>&mdash;</span>
      </div>
    );
  }
  const pct = result.total > 0 ? (result.passed / result.total) * 100 : 0;
  const hasFuncs = result.funcs != null && result.funcs < result.total;
  return (
    <div className={styles.sideColumn}>
      <div className={styles.badgeRow}>
        {hasFuncs ? (
          <>
            {result.failed + result.errors > 0 ? (
              <span
                className="badge badge--danger"
                title={`${result.failed + result.errors} failing (of ${result.total} incl. subtests)`}
              >
                {result.failed + result.errors} failing
              </span>
            ) : (
              <span
                className="badge badge--success"
                title={`${result.passed} passed (of ${result.total} incl. subtests)`}
              >
                all passing
              </span>
            )}
            {result.skipped > 0 && (
              <span
                className="badge badge--warning"
                title={`${result.skipped} skipped (of ${result.total} incl. subtests)`}
              >
                {result.skipped} skipped
              </span>
            )}
          </>
        ) : (
          <>
            <span
              className="badge badge--success"
              title={`${result.passed} passed`}
            >
              {result.passed}
            </span>
            {result.failed > 0 && (
              <span
                className="badge badge--danger"
                title={`${result.failed} failed`}
              >
                {result.failed}
              </span>
            )}
            {result.errors > 0 && (
              <span
                className="badge badge--danger"
                title={`${result.errors} errors`}
              >
                {result.errors} err
              </span>
            )}
            {result.skipped > 0 && (
              <span
                className="badge badge--warning"
                title={`${result.skipped} skipped`}
              >
                {result.skipped}
              </span>
            )}
          </>
        )}
      </div>
      <div className={styles.barRow}>
        <div className={styles.progressBar}>
          <div className={styles.progressFill} style={{ width: `${pct}%` }} />
        </div>
        <span className={styles.totalLabel}>
          {hasFuncs ? (
            <>
              {result.funcs}
              <span
                className={styles.subtestCount}
                title={`${result.total} including subtests`}
              >
                ({result.total})
              </span>
            </>
          ) : (
            result.total
          )}
        </span>
      </div>
    </div>
  );
}

/** Segmented coverage bar showing test state breakdown. */
function CoverageBar({ filter }: { filter: FilterComparison }) {
  const stats = useMemo(() => {
    if (!filter.testCases || filter.testCases.length === 0) return null;
    let implemented = 0;
    let notApplicable = 0;
    let pending = 0;
    let unmapped = 0;
    for (const tc of filter.testCases) {
      switch (tc.testState) {
        case "implemented":
          implemented++;
          break;
        case "pending":
          pending++;
          break;
        case "skipped":
          notApplicable++;
          break;
        default:
          if (tc.skipReason) notApplicable++;
          else unmapped++;
      }
    }
    return { implemented, notApplicable, pending, unmapped };
  }, [filter.testCases]);

  if (!stats) {
    // Fallback to old simple bar
    if (!filter.coverage || filter.coverage.totalOkapi === 0) return null;
    const pct = filter.coverage.coveragePct;
    return (
      <div className={styles.coverageMini}>
        <div className={styles.coverageMiniBar}>
          <div
            className={styles.coverageMiniBarFill}
            style={{ width: `${Math.min(pct, 100)}%` }}
          />
        </div>
        <span className={styles.coverageMiniLabel}>{pct.toFixed(0)}%</span>
      </div>
    );
  }

  const total = filter.testCases.length;
  if (total === 0) return null;

  const segments = [
    { value: stats.implemented, color: "#2e8555" },
    { value: stats.notApplicable, color: "#94a3b8" },
    { value: stats.pending, color: "#e3a008" },
    { value: stats.unmapped, color: "#dc2626" },
  ];

  const implementedPct = ((stats.implemented / total) * 100).toFixed(0);

  return (
    <div
      className={styles.coverageMini}
      title={`Implemented: ${stats.implemented}, Not Applicable: ${stats.notApplicable}, Pending: ${stats.pending}, Unmapped: ${stats.unmapped}`}
    >
      <div className={styles.segmentedBar}>
        {segments
          .filter((s) => s.value > 0)
          .map((s, i) => (
            <div
              key={i}
              className={styles.segmentedBarPart}
              style={{
                width: `${(s.value / total) * 100}%`,
                backgroundColor: s.color,
              }}
            />
          ))}
      </div>
      <span className={styles.coverageMiniLabel}>{implementedPct}%</span>
    </div>
  );
}

/** Column headings row -- rendered once above the filter list. */
export function FilterColumnHeadings() {
  return (
    <div className={`${styles.filterHeader} ${styles.columnHeadings}`}>
      <div className={styles.filterHeaderLeft}>
        <span className={styles.columnHeading}>Filter</span>
      </div>
      <div className={styles.filterHeaderRight}>
        <div className={styles.sideColumn}>
          <span className={styles.columnHeading}>Okapi</span>
        </div>
        <div className={styles.sideColumn}>
          <span className={styles.columnHeading}>Bridge</span>
        </div>
        <div className={styles.sideColumn}>
          <span className={styles.columnHeading}>Native</span>
        </div>
      </div>
    </div>
  );
}

export default function FilterCard({
  filter,
  goCommitSHA,
  okapiTag,
  defaultExpanded,
  defaultTestFilter,
}: Props) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? false);

  return (
    <div className={`card ${styles.filterCard}`}>
      <div
        className={`card__header ${styles.filterHeader}`}
        onClick={() => setExpanded(!expanded)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => e.key === "Enter" && setExpanded(!expanded)}
      >
        <div className={styles.filterHeaderLeft}>
          <span className={`${styles.statusDot} ${statusColor(filter)}`} />
          <strong>{filter.filterName}</strong>
          {filter.okapiFilterIds && filter.okapiFilterIds.length > 0 && (
            <span
              className={styles.nativeAlias}
              title={`Okapi filter${filter.okapiFilterIds.length > 1 ? "s" : ""}: ${filter.okapiFilterIds.join(", ")}`}
            >
              (okf_{filter.okapiFilterIds.join(", okf_")})
            </span>
          )}
          <CoverageBar filter={filter} />
          <span className={styles.expandIcon}>
            {expanded ? "\u25BE" : "\u25B8"}
          </span>
        </div>
        <div className={styles.filterHeaderRight}>
          <StatColumn result={filter.okapi} />
          <StatColumn result={filter.bridge} />
          <StatColumn result={filter.native} />
        </div>
      </div>
      {expanded && (
        <div className="card__body">
          {(filter.spec ||
            (filter.specDrift && filter.specDrift.length > 0) ||
            (filter.specConfigDrift && filter.specConfigDrift.length > 0)) && (
            <SpecSection
              spec={filter.spec ?? emptySpecSummary}
              drift={filter.specDrift}
              configDrift={filter.specConfigDrift}
            />
          )}
          {filter.testCases.length > 0 ? (
            <TestCaseTable
              testCases={filter.testCases}
              filterName={filter.filterName}
              goCommitSHA={goCommitSHA}
              okapiTag={okapiTag}
              defaultFilter={defaultTestFilter}
            />
          ) : (
            <p className={styles.noData}>No test data available.</p>
          )}
        </div>
      )}
    </div>
  );
}
