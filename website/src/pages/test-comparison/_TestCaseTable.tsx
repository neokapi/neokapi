import {useState} from 'react';
import type {TestCaseMatch} from './_types';
import styles from './_index.module.css';

interface Props {
  testCases: TestCaseMatch[];
}

type FilterMode = 'all' | 'mapped' | 'unmapped' | 'failing';
type SortMode = 'class' | 'status';

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

function isMapped(tc: TestCaseMatch): boolean {
  return tc.bridgeStatus !== '' || tc.nativeStatus !== '';
}

function isFailing(tc: TestCaseMatch): boolean {
  return (
    tc.bridgeStatus === 'fail' ||
    tc.bridgeStatus === 'error' ||
    tc.nativeStatus === 'fail' ||
    tc.nativeStatus === 'error'
  );
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

export default function TestCaseTable({testCases}: Props) {
  const [filter, setFilter] = useState<FilterMode>('all');
  const [sort, setSort] = useState<SortMode>('class');

  const filtered = testCases.filter((tc) => {
    switch (filter) {
      case 'mapped':
        return isMapped(tc);
      case 'unmapped':
        return !isMapped(tc);
      case 'failing':
        return isFailing(tc);
      default:
        return true;
    }
  });

  const sorted = [...filtered].sort((a, b) => {
    if (sort === 'status') {
      const aMin = Math.min(
        statusOrder(a.bridgeStatus),
        statusOrder(a.nativeStatus),
      );
      const bMin = Math.min(
        statusOrder(b.bridgeStatus),
        statusOrder(b.nativeStatus),
      );
      if (aMin !== bMin) return aMin - bMin;
    }
    const cmp = a.javaClass.localeCompare(b.javaClass);
    if (cmp !== 0) return cmp;
    return a.javaMethod.localeCompare(b.javaMethod);
  });

  const mappedCount = testCases.filter(isMapped).length;
  const failingCount = testCases.filter(isFailing).length;

  const filterButtons: {mode: FilterMode; label: string}[] = [
    {mode: 'all', label: `All (${testCases.length})`},
    {mode: 'mapped', label: `Mapped (${mappedCount})`},
    {mode: 'unmapped', label: `Unmapped (${testCases.length - mappedCount})`},
    {mode: 'failing', label: `Failing (${failingCount})`},
  ];

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
            className={`button button--sm ${sort === 'class' ? 'button--primary' : 'button--outline button--secondary'}`}
            onClick={(e) => {
              e.stopPropagation();
              setSort('class');
            }}>
            Class
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
            <th>Java Test</th>
            <th>Okapi</th>
            <th>Bridge</th>
            <th>Native</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((tc) => {
            const shortClass = tc.javaClass.split('.').pop() ?? tc.javaClass;
            return (
              <tr key={`${tc.javaClass}#${tc.javaMethod}`}>
                <td className={styles.testCaseName}>
                  <span className={styles.testCaseClass}>{shortClass}</span>
                  <span className={styles.testCaseMethod}>
                    #{tc.javaMethod}
                  </span>
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
