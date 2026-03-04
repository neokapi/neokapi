import type {FilterComparison, Summary} from './_types';
import styles from './_index.module.css';

interface Props {
  summary: Summary;
  generatedAt: string;
  filters?: FilterComparison[];
}

export default function SummaryBar({
  summary,
  generatedAt,
  filters,
}: Props) {
  // Aggregate state counts from all filters
  let implementedCount = 0;
  let pendingCount = 0;
  let skippedCount = 0;
  let unmappedCount = 0;
  let hasStateData = false;

  if (filters) {
    for (const f of filters) {
      if (f.coverage) {
        if (f.coverage.skippedCount || f.coverage.pendingCount) {
          hasStateData = true;
        }
        skippedCount += f.coverage.skippedCount ?? 0;
        pendingCount += f.coverage.pendingCount ?? 0;
      }
      if (f.testCases) {
        for (const tc of f.testCases) {
          if (tc.testState === 'implemented') implementedCount++;
          else if (tc.testState === 'unmapped' || !tc.testState)
            unmappedCount++;
        }
      }
    }
  }

  return (
    <div className={styles.summaryBar}>
      <div className={styles.statCard}>
        <div className={styles.statValue}>{summary.totalTestsOkapi}</div>
        <div className={styles.statLabel}>Okapi Tests</div>
      </div>
      {hasStateData ? (
        <>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{implementedCount}</div>
            <div className={styles.statLabel}>Implemented</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{pendingCount}</div>
            <div className={styles.statLabel}>Pending</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{skippedCount}</div>
            <div className={styles.statLabel}>Skipped</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{unmappedCount}</div>
            <div className={styles.statLabel}>Unmapped</div>
          </div>
        </>
      ) : (
        <>
          <div className={styles.statCard}>
            <div className={styles.statValue}>
              {summary.totalFuncsBridge ?? summary.totalTestsBridge}
            </div>
            <div className={styles.statLabel}>Bridge Tests</div>
            {summary.totalFuncsBridge != null &&
              summary.totalFuncsBridge < summary.totalTestsBridge && (
                <div
                  className={styles.subtestCount}
                  title={`${summary.totalTestsBridge} including subtests`}>
                  ({summary.totalTestsBridge} incl. subtests)
                </div>
              )}
          </div>
          <div className={styles.statCard}>
            <div className={styles.statValue}>
              {summary.totalFuncsNative ?? summary.totalTestsNative}
            </div>
            <div className={styles.statLabel}>Native Tests</div>
            {summary.totalFuncsNative != null &&
              summary.totalFuncsNative < summary.totalTestsNative && (
                <div
                  className={styles.subtestCount}
                  title={`${summary.totalTestsNative} including subtests`}>
                  ({summary.totalTestsNative} incl. subtests)
                </div>
              )}
          </div>
        </>
      )}
      <div className={styles.statCard}>
        <div className={styles.statValue}>
          {summary.coveragePct > 0
            ? `${summary.coveragePct.toFixed(1)}%`
            : '\u2014'}
        </div>
        <div className={styles.statLabel}>Coverage</div>
        {summary.coveragePct > 0 && (
          <div className={styles.coverageBarWrap}>
            <div
              className={styles.coverageBarFill}
              style={{width: `${Math.min(summary.coveragePct, 100)}%`}}
            />
          </div>
        )}
      </div>
      <div className={styles.statCard}>
        <div className={styles.statLabel}>Generated</div>
        <div className={styles.statDate}>
          {new Date(generatedAt).toLocaleDateString()}
        </div>
      </div>
    </div>
  );
}
