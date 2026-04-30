import type { SpecExample, SpecExampleStatus, SpecSummary } from "./_types";
import styles from "./_index.module.css";

interface Props {
  spec: SpecSummary;
}

const statusBadge: Record<SpecExampleStatus, { label: string; className: string }> = {
  pass: { label: "pass", className: "badge badge--success" },
  fail: { label: "fail", className: "badge badge--danger" },
  skip: { label: "skip", className: "badge badge--secondary" },
  expected_fail: { label: "xfail", className: "badge badge--warning" },
  parity_warn: { label: "parity warn", className: "badge badge--warning" },
};

function totalExamples(spec: SpecSummary): number {
  return spec.pass + spec.fail + spec.skip + spec.expectedFail + spec.parityWarn;
}

/** SpecSection renders the per-feature outcomes inside an expanded
 * filter card. Features are listed in author-declared order; each row
 * shows the feature id, an example status badge per example, and the
 * expected_fail / parity_warn detail when present.
 */
export default function SpecSection({ spec }: Props) {
  const total = totalExamples(spec);
  return (
    <div className={styles.specSection}>
      <h4 className={styles.specHeading}>
        Spec features
        <span className={styles.specSummaryBadges}>
          {spec.pass > 0 && (
            <span className="badge badge--success">{spec.pass} pass</span>
          )}
          {spec.fail > 0 && (
            <span className="badge badge--danger">{spec.fail} fail</span>
          )}
          {spec.parityWarn > 0 && (
            <span className="badge badge--warning">{spec.parityWarn} parity warn</span>
          )}
          {spec.expectedFail > 0 && (
            <span className="badge badge--warning">{spec.expectedFail} xfail</span>
          )}
          {spec.skip > 0 && (
            <span className="badge badge--secondary">{spec.skip} skip</span>
          )}
          <span className={styles.specSummaryTotal}>({total} total)</span>
        </span>
      </h4>
      <table className={styles.specTable}>
        <thead>
          <tr>
            <th>Feature</th>
            <th>Example</th>
            <th>Mode</th>
            <th>Status</th>
            <th>Detail</th>
          </tr>
        </thead>
        <tbody>
          {spec.features.flatMap((f) =>
            f.examples.map((ex, i) => <ExampleRow key={`${f.id}-${ex.name}`} feature={f.id} ex={ex} first={i === 0} />),
          )}
        </tbody>
      </table>
    </div>
  );
}

function ExampleRow({
  feature,
  ex,
  first,
}: {
  feature: string;
  ex: SpecExample;
  first: boolean;
}) {
  const badge = statusBadge[ex.status] ?? statusBadge.pass;
  return (
    <tr>
      <td className={styles.specFeatureCell}>{first ? feature : ""}</td>
      <td className={styles.specExampleCell}>{ex.name}</td>
      <td>{ex.mode ?? ""}</td>
      <td>
        <span className={badge.className}>{badge.label}</span>
      </td>
      <td className={styles.specDetailCell}>
        {ex.detail ? <span title={ex.detail}>{truncate(ex.detail, 80)}</span> : ""}
      </td>
    </tr>
  );
}

function truncate(s: string, n: number): string {
  const flat = s.replace(/\n/g, " ").trim();
  if (flat.length <= n) return flat;
  return flat.slice(0, n - 1) + "…";
}
