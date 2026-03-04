import {useState} from 'react';
import type {TestCaseRow, TestState} from './_types';
import styles from './_index.module.css';

interface Props {
  testCases: TestCaseRow[];
  filterName: string;
  goCommitSHA?: string;
  okapiTag?: string;
}

type FilterMode =
  | 'all'
  | 'okapi'
  | 'bridge'
  | 'native'
  | 'failing'
  | 'implemented'
  | 'pending'
  | 'skipped'
  | 'unmapped';
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

/** CSS class for test state row background. */
function stateRowClass(state?: TestState): string {
  switch (state) {
    case 'implemented':
      return styles.stateImplemented ?? '';
    case 'pending':
      return styles.statePending ?? '';
    case 'skipped':
      return styles.stateSkipped ?? '';
    default:
      return '';
  }
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

/** Build a GitHub source URL for a Go test file+line. */
function goSourceUrl(
  file: string | undefined,
  line: number | undefined,
  commitSHA: string | undefined,
  filterName: string,
  kind: 'bridge' | 'native',
): string {
  const ref = commitSHA || 'main';
  if (file) {
    const base = `https://github.com/gokapi/gokapi/blob/${ref}/${file}`;
    return line ? `${base}#L${line}` : base;
  }
  // Fallback to directory
  const dir =
    kind === 'bridge'
      ? `core/plugin/bridge/filters/okf_${filterName}/`
      : `core/formats/${filterName}/`;
  return `https://github.com/gokapi/gokapi/tree/${ref}/${dir}`;
}

/** Build a GitLab source URL for an Okapi Java test file. */
function okapiSourceUrl(
  okapiFile: string | undefined,
  okapiTag: string | undefined,
): string | null {
  if (!okapiFile) return null;
  const ref = okapiTag || 'master';
  return `https://gitlab.com/okapiframework/Okapi/-/blob/${ref}/${okapiFile}?ref_type=tags`;
}

export default function TestCaseTable({
  testCases,
  filterName,
  goCommitSHA,
  okapiTag,
}: Props) {
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
      case 'implemented':
        return tc.testState === 'implemented';
      case 'pending':
        return tc.testState === 'pending';
      case 'skipped':
        return tc.testState === 'skipped';
      case 'unmapped':
        return tc.testState === 'unmapped' || !tc.testState;
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

  // State-based counts
  const hasTestState = testCases.some((tc) => tc.testState);
  const implementedCount = testCases.filter(
    (tc) => tc.testState === 'implemented',
  ).length;
  const pendingCount = testCases.filter(
    (tc) => tc.testState === 'pending',
  ).length;
  const skippedCount = testCases.filter(
    (tc) => tc.testState === 'skipped',
  ).length;
  const unmappedCount = testCases.filter(
    (tc) => tc.testState === 'unmapped' || !tc.testState,
  ).length;

  const filterButtons: {mode: FilterMode; label: string}[] = [
    {mode: 'all', label: `All (${testCases.length})`},
    ...(hasTestState
      ? [
          {
            mode: 'implemented' as FilterMode,
            label: `Implemented (${implementedCount})`,
          },
          {
            mode: 'pending' as FilterMode,
            label: `Pending (${pendingCount})`,
          },
          {
            mode: 'skipped' as FilterMode,
            label: `Skipped (${skippedCount})`,
          },
          {
            mode: 'unmapped' as FilterMode,
            label: `Unmapped (${unmappedCount})`,
          },
        ]
      : [
          {mode: 'okapi' as FilterMode, label: `Okapi (${okapiCount})`},
          {mode: 'bridge' as FilterMode, label: `Bridge (${bridgeCount})`},
          {mode: 'native' as FilterMode, label: `Native (${nativeCount})`},
        ]),
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
                  className={`${styles.testCaseRow} ${isExpanded ? styles.testCaseRowExpanded : ''} ${stateRowClass(tc.testState)}`}
                  title={tc.skipReason ? `${tc.testState === 'skipped' ? 'Skipped' : 'Unmapped'}: ${tc.skipReason}` : undefined}
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
                    {tc.bridgeSubtests != null && tc.bridgeSubtests > 0 && (
                      <span
                        className={styles.subtestCount}
                        title={`${tc.bridgeSubtests} subtests`}>
                        +{tc.bridgeSubtests}
                      </span>
                    )}
                  </td>
                  <td className={styles.testCaseStatus}>
                    <StatusCell status={tc.nativeStatus} />
                    {tc.nativeSubtests != null && tc.nativeSubtests > 0 && (
                      <span
                        className={styles.subtestCount}
                        title={`${tc.nativeSubtests} subtests`}>
                        +{tc.nativeSubtests}
                      </span>
                    )}
                  </td>
                </tr>
                {isExpanded && (
                  <tr key={`${rowKey}-detail`} className={styles.detailRow}>
                    <td colSpan={4}>
                      <div className={styles.detailContent}>
                        {tc.okapiStatus && tc.javaClass && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Okapi:</span>
                            {(() => {
                              const url = okapiSourceUrl(tc.okapiFile, okapiTag);
                              return url ? (
                                <a
                                  href={url}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  onClick={(e) => e.stopPropagation()}>
                                  <code>
                                    {tc.javaClass}#{tc.testName}
                                  </code>
                                </a>
                              ) : (
                                <code>
                                  {tc.javaClass}#{tc.testName}
                                </code>
                              );
                            })()}
                            {tc.okapiFile && (
                              <span className={styles.detailPath}>
                                {tc.okapiFile}
                              </span>
                            )}
                          </div>
                        )}
                        {(tc.bridgeTest || tc.bridgeStatus) && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Bridge:</span>
                            <a
                              href={goSourceUrl(
                                tc.bridgeFile,
                                tc.bridgeLine,
                                goCommitSHA,
                                filterName,
                                'bridge',
                              )}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}>
                              <code>{tc.bridgeTest || tc.testName}</code>
                            </a>
                            {tc.bridgeFile && (
                              <span className={styles.detailPath}>
                                {tc.bridgeFile}
                                {tc.bridgeLine ? `:${tc.bridgeLine}` : ''}
                              </span>
                            )}
                          </div>
                        )}
                        {(tc.nativeTest || tc.nativeStatus) && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Native:</span>
                            <a
                              href={goSourceUrl(
                                tc.nativeFile,
                                tc.nativeLine,
                                goCommitSHA,
                                filterName,
                                'native',
                              )}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}>
                              <code>{tc.nativeTest || tc.testName}</code>
                            </a>
                            {tc.nativeFile && (
                              <span className={styles.detailPath}>
                                {tc.nativeFile}
                                {tc.nativeLine ? `:${tc.nativeLine}` : ''}
                              </span>
                            )}
                          </div>
                        )}
                        {tc.skipReason && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Reason:</span>
                            <span>{tc.skipReason}</span>
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
