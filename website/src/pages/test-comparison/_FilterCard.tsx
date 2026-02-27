import {useState} from 'react';
import type {FilterComparison, FilterResult} from './_types';
import TestList from './_TestList';
import styles from './_index.module.css';

interface Props {
  filter: FilterComparison;
}

function statusColor(filter: FilterComparison): string {
  if (filter.okapi && filter.gokapi) {
    const hasFails =
      filter.okapi.failed +
        filter.okapi.errors +
        filter.gokapi.failed +
        filter.gokapi.errors >
      0;
    return hasFails ? styles.statusRed : styles.statusGreen;
  }
  return styles.statusYellow;
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

export default function FilterCard({filter}: Props) {
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
          <span className={styles.expandIcon}>{expanded ? '▾' : '▸'}</span>
        </div>
        <div className={styles.filterHeaderRight}>
          <div className={styles.sideColumn}>
            <span className={styles.sideLabel}>Okapi</span>
            <Counts result={filter.okapi} label="Okapi" />
            <ProgressMini result={filter.okapi} />
          </div>
          <div className={styles.sideColumn}>
            <span className={styles.sideLabel}>Gokapi</span>
            <Counts result={filter.gokapi} label="Gokapi" />
            <ProgressMini result={filter.gokapi} />
          </div>
        </div>
      </div>
      {expanded && (
        <div className="card__body">
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
              <h4>Gokapi</h4>
              {filter.gokapi ? (
                <TestList result={filter.gokapi} />
              ) : (
                <p className={styles.noData}>No tests</p>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
