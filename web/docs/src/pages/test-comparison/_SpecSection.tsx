import type {
  SpecConfigDriftEntry,
  SpecDriftEntry,
  SpecExample,
  SpecExampleStatus,
  SpecSummary,
} from "./_types";
import styles from "./_index.module.css";

interface Props {
  spec: SpecSummary;
  drift?: SpecDriftEntry[];
  configDrift?: SpecConfigDriftEntry[];
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
export default function SpecSection({ spec, drift, configDrift }: Props) {
  const total = totalExamples(spec);
  const hasFeatures = spec.features.length > 0;
  const totalDrift = (drift?.length ?? 0) + (configDrift?.length ?? 0);
  return (
    <div className={styles.specSection}>
      <h4 className={styles.specHeading}>
        Spec features
        <span className={styles.specSummaryBadges}>
          {spec.pass > 0 && <span className="badge badge--success">{spec.pass} pass</span>}
          {spec.fail > 0 && <span className="badge badge--danger">{spec.fail} fail</span>}
          {spec.parityWarn > 0 && (
            <span className="badge badge--warning">{spec.parityWarn} parity warn</span>
          )}
          {spec.expectedFail > 0 && (
            <span className="badge badge--warning">{spec.expectedFail} xfail</span>
          )}
          {spec.skip > 0 && <span className="badge badge--secondary">{spec.skip} skip</span>}
          {totalDrift > 0 && (
            <span
              className="badge badge--warning"
              title="spec.yaml entries (okapi_refs and/or config keys) that don't match the pinned Okapi version or bridge schema"
            >
              {totalDrift} drift
            </span>
          )}
          {hasFeatures && <span className={styles.specSummaryTotal}>({total} total)</span>}
        </span>
      </h4>
      {hasFeatures && (
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
              f.examples.map((ex, i) => (
                <ExampleRow key={`${f.id}-${ex.name}`} feature={f.id} ex={ex} first={i === 0} />
              )),
            )}
          </tbody>
        </table>
      )}
      {drift && drift.length > 0 && <DriftBlock drift={drift} />}
      {configDrift && configDrift.length > 0 && <ConfigDriftBlock drift={configDrift} />}
    </div>
  );
}

/** DriftBlock surfaces stale okapi_refs as a list under the spec table.
 * Refs are grouped visually by feature so the location in spec.yaml is
 * obvious to anyone needing to repoint or remove them.
 */
function DriftBlock({ drift }: { drift: SpecDriftEntry[] }) {
  return (
    <div className={styles.specDrift}>
      <p className={styles.specDriftHeading}>
        Stale okapi_refs (no match in pinned Okapi @Test set)
      </p>
      <ul className={styles.specDriftList}>
        {drift.map((d, i) => (
          <li key={`${d.featureId}-${d.okapiRef}-${i}`}>
            <span className={styles.specDriftFeature}>{d.featureId}:</span>
            {d.okapiRef}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** ConfigDriftBlock surfaces spec.config[] keys that don't appear in
 * the bridge composite JSON Schema. Each row also shows the underlying
 * Java field (okapi_param) when known so the spec author has both ends
 * of the rename.
 */
function ConfigDriftBlock({ drift }: { drift: SpecConfigDriftEntry[] }) {
  return (
    <div className={styles.specDrift}>
      <p className={styles.specDriftHeading}>
        Config keys not in bridge schema (rename or removal upstream)
      </p>
      <ul className={styles.specDriftList}>
        {drift.map((d, i) => (
          <li key={`${d.key}-${i}`}>
            <span className={styles.specDriftFeature}>{d.key}</span>
            {d.okapiParam && <span> ← okapi_param: {d.okapiParam}</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}

function ExampleRow({ feature, ex, first }: { feature: string; ex: SpecExample; first: boolean }) {
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
