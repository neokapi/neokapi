import type { ReactNode } from "react";
import type {
  DivergenceKind,
  SpecConfigDriftEntry,
  SpecDriftEntry,
  SpecExample,
  SpecExampleStatus,
  SpecSummary,
} from "./_types";
import {
  divergenceColors,
  divergenceDescriptions,
  divergenceLabels,
} from "./_types";
import styles from "./_index.module.css";

interface Props {
  spec: SpecSummary;
  drift?: SpecDriftEntry[];
  configDrift?: SpecConfigDriftEntry[];
}

const statusBadge: Record<
  SpecExampleStatus,
  { label: string; className: string }
> = {
  pass: { label: "pass", className: "badge badge--success" },
  fail: { label: "fail", className: "badge badge--danger" },
  skip: { label: "skip", className: "badge badge--secondary" },
  // xfail / parity_warn are documented divergences, not alarms — render the
  // status badge neutral and let the per-example divergence chip carry the
  // severity (only `native-bug` is colored as danger).
  expected_fail: { label: "xfail", className: "badge badge--info" },
  parity_warn: { label: "parity warn", className: "badge badge--info" },
};

function totalExamples(spec: SpecSummary): number {
  return (
    spec.pass + spec.fail + spec.skip + spec.expectedFail + spec.parityWarn
  );
}

/** SpecSection renders the per-feature outcomes inside an expanded
 * filter card. Features are listed in author-declared order; each row
 * shows the feature id, an example status badge per example, a fault
 * attribution chip for divergences, and the full divergence reason.
 */
export default function SpecSection({ spec, drift, configDrift }: Props) {
  const total = totalExamples(spec);
  const hasFeatures = spec.features.length > 0;
  const totalDrift = (drift?.length ?? 0) + (configDrift?.length ?? 0);
  const hasDivergences = spec.expectedFail > 0 || spec.parityWarn > 0;
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
            <span className="badge badge--info">
              {spec.parityWarn} parity warn
            </span>
          )}
          {spec.expectedFail > 0 && (
            <span className="badge badge--info">{spec.expectedFail} xfail</span>
          )}
          {spec.skip > 0 && (
            <span className="badge badge--secondary">{spec.skip} skip</span>
          )}
          {totalDrift > 0 && (
            <span
              className="badge badge--warning"
              title="spec.yaml entries (okapi_refs and/or config keys) that don't match the pinned Okapi version or bridge schema"
            >
              {totalDrift} drift
            </span>
          )}
          {hasFeatures && (
            <span className={styles.specSummaryTotal}>({total} total)</span>
          )}
        </span>
      </h4>
      {hasFeatures && hasDivergences && <DivergenceLegend />}
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
                <ExampleRow
                  key={`${f.id}-${ex.name}`}
                  feature={f.id}
                  ex={ex}
                  first={i === 0}
                />
              )),
            )}
          </tbody>
        </table>
      )}
      {drift && drift.length > 0 && <DriftBlock drift={drift} />}
      {configDrift && configDrift.length > 0 && (
        <ConfigDriftBlock drift={configDrift} />
      )}
    </div>
  );
}

/** Categories shown in the legend, in display order. */
const legendOrder: DivergenceKind[] = [
  "native-bug",
  "bridge-gap",
  "okapi-bug",
  "default-diff",
  "scope-diff",
  "missing-filter",
  "fixture",
  "contract",
];

/** Short explainer above the spec table: what an xfail means and how to
 * read the fault-attribution chips. */
function DivergenceLegend() {
  return (
    <div className={styles.specLegend}>
      <span className={styles.specLegendTitle}>
        xfail = a documented divergence from Okapi.
      </span>{" "}
      The category chip shows which side differs and why. neokapi is the correct
      side except where marked <DivergenceChip kind="native-bug" />.
      <div className={styles.specLegendChips}>
        {legendOrder.map((k) => (
          <DivergenceChip key={k} kind={k} />
        ))}
      </div>
    </div>
  );
}

/** A single colored fault-attribution chip with a descriptive tooltip. */
function DivergenceChip({ kind }: { kind: DivergenceKind }) {
  return (
    <span
      className={styles.divergenceChip}
      style={{ backgroundColor: divergenceColors[kind] }}
      title={divergenceDescriptions[kind]}
    >
      {divergenceLabels[kind]}
    </span>
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
  const isDivergence =
    ex.status === "expected_fail" || ex.status === "parity_warn";
  return (
    <tr>
      <td className={styles.specFeatureCell}>{first ? feature : ""}</td>
      <td className={styles.specExampleCell}>{ex.name}</td>
      <td>{ex.mode ?? ""}</td>
      <td>
        <span className={badge.className}>{badge.label}</span>
      </td>
      <td className={styles.specDetailCell}>
        {isDivergence ? (
          <div className={styles.specDetailRow}>
            {ex.divergence && <DivergenceChip kind={ex.divergence} />}
            {ex.detail && (
              <span className={styles.specDetailFull}>
                {linkifyIssues(ex.detail)}
              </span>
            )}
          </div>
        ) : ex.detail ? (
          <span className={styles.specDetailFull}>
            {linkifyIssues(ex.detail)}
          </span>
        ) : (
          ""
        )}
      </td>
    </tr>
  );
}

const issueRefRE = /#(\d+)/g;

/** Linkify any `#NNN` issue reference in a detail string to the neokapi
 * GitHub issue, returning a mix of text and anchor nodes. */
function linkifyIssues(detail: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  let lastIndex = 0;
  let key = 0;
  for (const match of detail.matchAll(issueRefRE)) {
    const start = match.index ?? 0;
    if (start > lastIndex) {
      nodes.push(detail.slice(lastIndex, start));
    }
    const issueNo = match[1];
    nodes.push(
      <a
        key={`issue-${issueNo}-${key++}`}
        className={styles.issueLink}
        href={`https://github.com/neokapi/neokapi/issues/${issueNo}`}
        target="_blank"
        rel="noreferrer"
      >
        #{issueNo}
      </a>,
    );
    lastIndex = start + match[0].length;
  }
  if (lastIndex < detail.length) {
    nodes.push(detail.slice(lastIndex));
  }
  return nodes;
}
