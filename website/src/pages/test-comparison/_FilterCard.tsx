import {useState} from 'react';
import type {FilterComparison, FilterResult} from './_types';
import TestCaseTable from './_TestCaseTable';
import styles from './_index.module.css';

interface Props {
  filter: FilterComparison;
  goCommitSHA?: string;
  okapiTag?: string;
}

function statusColor(filter: FilterComparison): string {
  const sides = [filter.okapi, filter.bridge, filter.native].filter(Boolean);
  if (sides.length === 0) return styles.statusYellow;
  const hasFails = sides.some((s) => s != null && s.failed + s.errors > 0);
  if (hasFails) return styles.statusRed;
  if (sides.length === 1) return styles.statusYellow;
  return styles.statusGreen;
}

function StatColumn({result}: {result: FilterResult | null}) {
  if (!result) {
    return (
      <div className={styles.sideColumn}>
        <span className={styles.noTests}>&mdash;</span>
      </div>
    );
  }
  const pct = result.total > 0 ? (result.passed / result.total) * 100 : 0;
  return (
    <div className={styles.sideColumn}>
      <div className={styles.badgeRow}>
        <span
          className="badge badge--success"
          title={`${result.passed} passed`}>
          {result.passed}
        </span>
        {result.failed > 0 && (
          <span
            className="badge badge--danger"
            title={`${result.failed} failed`}>
            {result.failed}
          </span>
        )}
        {result.errors > 0 && (
          <span
            className="badge badge--danger"
            title={`${result.errors} errors`}>
            {result.errors} err
          </span>
        )}
        {result.skipped > 0 && (
          <span
            className="badge badge--warning"
            title={`${result.skipped} skipped`}>
            {result.skipped}
          </span>
        )}
      </div>
      <div className={styles.barRow}>
        <div className={styles.progressBar}>
          <div className={styles.progressFill} style={{width: `${pct}%`}} />
        </div>
        <span className={styles.totalLabel}>{result.total}</span>
      </div>
    </div>
  );
}

function CoverageBar({filter}: {filter: FilterComparison}) {
  if (!filter.coverage || filter.coverage.totalOkapi === 0) return null;
  const pct = filter.coverage.coveragePct;
  return (
    <div className={styles.coverageMini}>
      <div className={styles.coverageMiniBar}>
        <div
          className={styles.coverageMiniBarFill}
          style={{width: `${Math.min(pct, 100)}%`}}
        />
      </div>
      <span className={styles.coverageMiniLabel}>{pct.toFixed(0)}%</span>
    </div>
  );
}

/** Column headings row — rendered once above the filter list. */
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

export default function FilterCard({filter, goCommitSHA, okapiTag}: Props) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className={`card ${styles.filterCard}`}>
      <div
        className={`card__header ${styles.filterHeader}`}
        onClick={() => setExpanded(!expanded)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => e.key === 'Enter' && setExpanded(!expanded)}>
        <div className={styles.filterHeaderLeft}>
          <span className={`${styles.statusDot} ${statusColor(filter)}`} />
          <strong>{filter.filterName}</strong>
          <CoverageBar filter={filter} />
          <span className={styles.expandIcon}>
            {expanded ? '\u25BE' : '\u25B8'}
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
          {filter.testCases.length > 0 ? (
            <TestCaseTable
              testCases={filter.testCases}
              filterName={filter.filterName}
              goCommitSHA={goCommitSHA}
              okapiTag={okapiTag}
            />
          ) : (
            <p className={styles.noData}>No test data available.</p>
          )}
        </div>
      )}
    </div>
  );
}
