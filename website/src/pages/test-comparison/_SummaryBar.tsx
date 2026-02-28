import type {Summary} from './_types';
import styles from './_index.module.css';

interface Props {
  summary: Summary;
  generatedAt: string;
}

export default function SummaryBar({summary, generatedAt}: Props) {
  return (
    <div className={styles.summaryBar}>
      <div className={styles.statCard}>
        <div className={styles.statValue}>{summary.totalTestsOkapi}</div>
        <div className={styles.statLabel}>Okapi Tests</div>
      </div>
      <div className={styles.statCard}>
        <div className={styles.statValue}>{summary.totalTestsBridge}</div>
        <div className={styles.statLabel}>Bridge Mapped</div>
      </div>
      <div className={styles.statCard}>
        <div className={styles.statValue}>{summary.totalTestsNative}</div>
        <div className={styles.statLabel}>Native Mapped</div>
      </div>
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
