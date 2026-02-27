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
        <div className={styles.statValue}>{summary.totalFiltersBoth}</div>
        <div className={styles.statLabel}>Shared Filters</div>
      </div>
      <div className={styles.statCard}>
        <div className={styles.statValue}>{summary.totalFiltersOkapi}</div>
        <div className={styles.statLabel}>Okapi Filters</div>
      </div>
      <div className={styles.statCard}>
        <div className={styles.statValue}>{summary.totalFiltersGokapi}</div>
        <div className={styles.statLabel}>Gokapi Filters</div>
      </div>
      <div className={styles.statCard}>
        <div className={styles.statValue}>{summary.totalTestsOkapi}</div>
        <div className={styles.statLabel}>Okapi Tests</div>
      </div>
      <div className={styles.statCard}>
        <div className={styles.statValue}>{summary.totalTestsGokapi}</div>
        <div className={styles.statLabel}>Gokapi Tests</div>
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
