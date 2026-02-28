import {useState} from 'react';
import type {TestCaseRow} from './_types';
import styles from './_index.module.css';

interface Props {
  testCases: TestCaseRow[];
  filterName: string;
}

type FilterMode = 'all' | 'okapi' | 'bridge' | 'native' | 'failing';
type SortMode = 'name' | 'status';

const statusBadgeClass: Record<string, string> = {
  pass: 'badge badge--success',
  fail: 'badge badge--danger',
  error: 'badge badge--danger',
  skip: 'badge badge--secondary',
};

function StatusCell({status}: {status: string}) {
  if (!status) {
    return <span className={styles.statusDash}>&mdash;</span>;
  }
  return <span className={statusBadgeClass[status]}>{status}</span>;
}

function statusOrder(s: string): number {
  switch (s) {
    case 'fail':
      return 0;
    case 'error':
      return 1;
    case 'skip':
      return 2;
    case 'pass':
      return 3;
    default:
      return 4;
  }
}

/** Construct a GitHub source link for a bridge test. */
function bridgeSrcPath(filterName: string): string {
  return `core/plugin/bridge/filters/okf_${filterName}/`;
}

/** Construct a path for a native format test. */
function nativeSrcPath(filterName: string): string {
  return `core/formats/${filterName}/`;
}

/** GitHub repo base for source links. */
const REPO_BASE = 'https://github.com/gokapi/gokapi/tree/main/';

export default function TestCaseTable({testCases, filterName}: Props) {
  const [filter, setFilter] = useState<FilterMode>('all');
  const [sort, setSort] = useState<SortMode>('name');
  const [expandedRow, setExpandedRow] = useState<string | null>(null);

  const filtered = testCases.filter((tc) => {
    switch (filter) {
      case 'okapi':
        return tc.okapiStatus !== '';
      case 'bridge':
        return tc.bridgeStatus !== '';
      case 'native':
        return tc.nativeStatus !== '';
      case 'failing':
        return (
          tc.okapiStatus === 'fail' ||
          tc.okapiStatus === 'error' ||
          tc.bridgeStatus === 'fail' ||
          tc.bridgeStatus === 'error' ||
          tc.nativeStatus === 'fail' ||
          tc.nativeStatus === 'error'
        );
      default:
        return true;
    }
  });

  const sorted = [...filtered].sort((a, b) => {
    if (sort === 'status') {
      const aMin = Math.min(
        statusOrder(a.okapiStatus),
        statusOrder(a.bridgeStatus),
        statusOrder(a.nativeStatus),
      );
      const bMin = Math.min(
        statusOrder(b.okapiStatus),
        statusOrder(b.bridgeStatus),
        statusOrder(b.nativeStatus),
      );
      if (aMin !== bMin) return aMin - bMin;
    }
    return a.testName.localeCompare(b.testName);
  });

  const okapiCount = testCases.filter((tc) => tc.okapiStatus !== '').length;
  const bridgeCount = testCases.filter((tc) => tc.bridgeStatus !== '').length;
  const nativeCount = testCases.filter((tc) => tc.nativeStatus !== '').length;
  const failingCount = testCases.filter(
    (tc) =>
      tc.okapiStatus === 'fail' ||
      tc.okapiStatus === 'error' ||
      tc.bridgeStatus === 'fail' ||
      tc.bridgeStatus === 'error' ||
      tc.nativeStatus === 'fail' ||
      tc.nativeStatus === 'error',
  ).length;

  const filterButtons: {mode: FilterMode; label: string}[] = [
    {mode: 'all', label: `All (${testCases.length})`},
    {mode: 'okapi', label: `Okapi (${okapiCount})`},
    {mode: 'bridge', label: `Bridge (${bridgeCount})`},
    {mode: 'native', label: `Native (${nativeCount})`},
    ...(failingCount > 0
      ? [{mode: 'failing' as FilterMode, label: `Failing (${failingCount})`}]
      : []),
  ];

  const toggleRow = (key: string) => {
    setExpandedRow(expandedRow === key ? null : key);
  };

  return (
    <div className={styles.testCaseTableWrap}>
      <div className={styles.testCaseToolbar}>
        <div className={styles.testCaseFilterButtons}>
          {filterButtons.map((fb) => (
            <button
              key={fb.mode}
              className={`button button--sm ${filter === fb.mode ? 'button--primary' : 'button--outline button--secondary'}`}
              onClick={(e) => {
                e.stopPropagation();
                setFilter(fb.mode);
              }}>
              {fb.label}
            </button>
          ))}
        </div>
        <div className={styles.testCaseSortButtons}>
          <span className={styles.sortLabel}>Sort:</span>
          <button
            className={`button button--sm ${sort === 'name' ? 'button--primary' : 'button--outline button--secondary'}`}
            onClick={(e) => {
              e.stopPropagation();
              setSort('name');
            }}>
            Name
          </button>
          <button
            className={`button button--sm ${sort === 'status' ? 'button--primary' : 'button--outline button--secondary'}`}
            onClick={(e) => {
              e.stopPropagation();
              setSort('status');
            }}>
            Status
          </button>
        </div>
      </div>
      <table className={styles.testCaseTable}>
        <thead>
          <tr>
            <th>Test</th>
            <th className={styles.testCaseStatusHeader}>Okapi</th>
            <th className={styles.testCaseStatusHeader}>Bridge</th>
            <th className={styles.testCaseStatusHeader}>Native</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((tc, i) => {
            const rowKey = `${tc.testName}-${i}`;
            const isExpanded = expandedRow === rowKey;
            return (
              <>
                <tr
                  key={rowKey}
                  className={`${styles.testCaseRow} ${isExpanded ? styles.testCaseRowExpanded : ''}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    toggleRow(rowKey);
                  }}>
                  <td className={styles.testCaseName}>
                    {tc.javaClass ? (
                      <>
                        <span className={styles.testCaseClass}>
                          {tc.javaClass}
                        </span>
                        <span className={styles.testCaseMethod}>
                          #{tc.testName}
                        </span>
                      </>
                    ) : (
                      <span className={styles.testCaseGoName}>
                        {tc.testName}
                      </span>
                    )}
                  </td>
                  <td className={styles.testCaseStatus}>
                    <StatusCell status={tc.okapiStatus} />
                  </td>
                  <td className={styles.testCaseStatus}>
                    <StatusCell status={tc.bridgeStatus} />
                  </td>
                  <td className={styles.testCaseStatus}>
                    <StatusCell status={tc.nativeStatus} />
                  </td>
                </tr>
                {isExpanded && (
                  <tr key={`${rowKey}-detail`} className={styles.detailRow}>
                    <td colSpan={4}>
                      <div className={styles.detailContent}>
                        {tc.okapiStatus && tc.javaClass && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Okapi:</span>
                            <code>
                              {tc.javaClass}#{tc.testName}
                            </code>
                          </div>
                        )}
                        {tc.bridgeTest && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Bridge:</span>
                            <a
                              href={`${REPO_BASE}${bridgeSrcPath(filterName)}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}>
                              <code>{tc.bridgeTest}</code>
                            </a>
                            <span className={styles.detailPath}>
                              {bridgeSrcPath(filterName)}
                            </span>
                          </div>
                        )}
                        {tc.bridgeStatus && !tc.bridgeTest && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Bridge:</span>
                            <a
                              href={`${REPO_BASE}${bridgeSrcPath(filterName)}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}>
                              <code>{tc.testName}</code>
                            </a>
                            <span className={styles.detailPath}>
                              {bridgeSrcPath(filterName)}
                            </span>
                          </div>
                        )}
                        {tc.nativeTest && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Native:</span>
                            <a
                              href={`${REPO_BASE}${nativeSrcPath(filterName)}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}>
                              <code>{tc.nativeTest}</code>
                            </a>
                            <span className={styles.detailPath}>
                              {nativeSrcPath(filterName)}
                            </span>
                          </div>
                        )}
                        {tc.nativeStatus && !tc.nativeTest && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Native:</span>
                            <a
                              href={`${REPO_BASE}${nativeSrcPath(filterName)}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}>
                              <code>{tc.testName}</code>
                            </a>
                            <span className={styles.detailPath}>
                              {nativeSrcPath(filterName)}
                            </span>
                          </div>
                        )}
                        {!tc.okapiStatus &&
                          !tc.bridgeTest &&
                          !tc.bridgeStatus &&
                          !tc.nativeTest &&
                          !tc.nativeStatus && (
                            <span className={styles.noData}>
                              No source mapping available.
                            </span>
                          )}
                      </div>
                    </td>
                  </tr>
                )}
              </>
            );
          })}
          {sorted.length === 0 && (
            <tr>
              <td colSpan={4} className={styles.noData}>
                No test cases match the current filter.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
