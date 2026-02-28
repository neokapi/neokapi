import {useState} from 'react';
import type {FilterComparison, FilterResult} from './_types';
import TestList from './_TestList';
import TestCaseTable from './_TestCaseTable';
import styles from './_index.module.css';

interface Props {
  filter: FilterComparison;
}

function statusColor(filter: FilterComparison): string {
  const sides = [filter.okapi, filter.bridge, filter.native].filter(Boolean);
  if (sides.length === 0) return styles.statusYellow;

  const hasFails = sides.some(
    (s) => s != null && s.failed + s.errors > 0,
  );
  if (hasFails) return styles.statusRed;

  // Yellow if only one side has tests
  if (sides.length === 1) return styles.statusYellow;

  return styles.statusGreen;
}

function Counts({
  result,
  label,
}: {
  result: FilterResult | null;
  label: string;
}) {
  if (!result) {
    return <span className={styles.noTests}>No {label} tests</span>;
  }
  return (
    <span className={styles.counts}>
      <span className="badge badge--success">{result.passed}</span>
      {result.failed > 0 && (
        <span className="badge badge--danger">{result.failed}</span>
      )}
      {result.errors > 0 && (
        <span className="badge badge--danger">{result.errors} err</span>
      )}
      {result.skipped > 0 && (
        <span className="badge badge--secondary">{result.skipped}</span>
      )}
      <span className={styles.countTotal}>{result.total} total</span>
    </span>
  );
}

function ProgressMini({result}: {result: FilterResult | null}) {
  if (!result || result.total === 0) return null;
  const pct = (result.passed / result.total) * 100;
  return (
    <div className={styles.progressMini}>
      <div className={styles.progressFill} style={{width: `${pct}%`}} />
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

export default function FilterCard({filter}: Props) {
  const [expanded, setExpanded] = useState(false);

  const hasTestCases = filter.testCases && filter.testCases.length > 0;

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
          <span className={styles.expandIcon}>{expanded ? '\u25BE' : '\u25B8'}</span>
        </div>
        <div className={styles.filterHeaderRight}>
          <div className={styles.sideColumn}>
            <span className={styles.sideLabel}>Okapi</span>
            <Counts result={filter.okapi} label="Okapi" />
            <ProgressMini result={filter.okapi} />
          </div>
          <div className={styles.sideColumn}>
            <span className={styles.sideLabel}>Bridge</span>
            <Counts result={filter.bridge} label="Bridge" />
            <ProgressMini result={filter.bridge} />
          </div>
          <div className={styles.sideColumn}>
            <span className={styles.sideLabel}>Native</span>
            <Counts result={filter.native} label="Native" />
            <ProgressMini result={filter.native} />
          </div>
        </div>
      </div>
      {expanded && (
        <div className="card__body">
          {hasTestCases ? (
            <TestCaseTable testCases={filter.testCases} />
          ) : (
            <div className={styles.sideBySide}>
              <div className={styles.sidePanel}>
                <h4>Okapi</h4>
                {filter.okapi ? (
                  <TestList result={filter.okapi} />
                ) : (
                  <p className={styles.noData}>No tests</p>
                )}
              </div>
              <div className={styles.sidePanel}>
                <h4>Bridge</h4>
                {filter.bridge ? (
                  <TestList result={filter.bridge} />
                ) : (
                  <p className={styles.noData}>No tests</p>
                )}
              </div>
              {filter.native && (
                <div className={styles.sidePanel}>
                  <h4>Native</h4>
                  <TestList result={filter.native} />
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
