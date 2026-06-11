import type { FilterResult } from "./_types";
import styles from "./_index.module.css";

interface Props {
  result: FilterResult;
}

const statusBadge: Record<string, string> = {
  pass: "badge badge--success",
  fail: "badge badge--danger",
  error: "badge badge--danger",
  skip: "badge badge--secondary",
};

export default function TestList({ result }: Props) {
  return (
    <div>
      {result.suites.map((suite) => (
        <div key={suite.name} className={styles.suiteBlock}>
          <div className={styles.suiteName}>{suite.name}</div>
          <table className={styles.testTable}>
            <thead>
              <tr>
                <th>Test</th>
                <th>Status</th>
                <th>Duration</th>
              </tr>
            </thead>
            <tbody>
              {suite.tests.map((tc) => (
                <tr key={tc.name}>
                  <td className={styles.testName}>{tc.name}</td>
                  <td>
                    <span className={statusBadge[tc.status]}>{tc.status}</span>
                  </td>
                  <td className={styles.duration}>{tc.durationMs.toFixed(0)}ms</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </div>
  );
}
