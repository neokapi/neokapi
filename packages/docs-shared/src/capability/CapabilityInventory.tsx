import { useId, useState, type ReactNode } from "react";
import { operationChanges, type OperationChange, type Profile } from "./history";
import "./capability.css";

export type Capability = { id: string; status: string; evidence: string[]; detail: string };
export type CapabilitySubject = { id: string; capabilities: Capability[] };

/**
 * The words an inventory shows. Each site supplies them through its own
 * translation API, so this component holds no prose of its own.
 */
export type CapabilityLabels = {
  heading: string;
  filter: string;
  status: string;
  allStatuses: string;
  empty: string;
  clear: string;
  count: (shown: number, total: number) => string;
  /** Status names, weakest evidence first. The order sorts the rows. */
  states: Record<string, string>;
  subject: (id: string) => string;
  operation: (id: string) => string;
};

export type CapabilityInventoryProps = {
  subjects: CapabilitySubject[];
  labels: CapabilityLabels;
  evidenceBaseUrl: string;
  initialStatus?: string;
};

export function CapabilityState({ status, labels }: { status: string; labels: CapabilityLabels }) {
  return (
    <span className="nk-cap-state" data-state={status}>
      {labels.states[status] ?? status}
    </span>
  );
}

function Chevron() {
  return (
    <svg
      className="nk-cap-chevron"
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}

function SubjectRow({
  name,
  operations,
  labels,
  evidenceBaseUrl,
}: {
  name: string;
  operations: Capability[];
  labels: CapabilityLabels;
  evidenceBaseUrl: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const id = useId();
  const first = operations[0];
  return (
    <li className="nk-cap-row">
      <button
        className="nk-cap-disclosure"
        type="button"
        aria-expanded={expanded}
        aria-controls={id}
        onClick={() => setExpanded(!expanded)}
      >
        <strong>{name}</strong>
        <span className="nk-cap-operation">{labels.operation(first.id)}</span>
        <CapabilityState status={first.status} labels={labels} />
        <Chevron />
      </button>
      <div hidden={!expanded} id={id}>
        {expanded && (
          <ul className="nk-cap-operations">
            {operations.map((cap) => (
              <li key={cap.id}>
                <div className="nk-cap-operation-heading">
                  <strong>{labels.operation(cap.id)}</strong>
                  <CapabilityState status={cap.status} labels={labels} />
                </div>
                <p>{cap.detail}</p>
                {cap.evidence.length > 0 && (
                  <ul className="nk-cap-files">
                    {cap.evidence.map((ref) => {
                      const [file, test] = ref.split("#");
                      return (
                        <li key={ref}>
                          <a href={evidenceBaseUrl + file}>{test || file}</a>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </li>
  );
}

function Inventory({
  subjects,
  labels,
  evidenceBaseUrl,
  initialStatus = "",
}: CapabilityInventoryProps) {
  const order = Object.keys(labels.states);
  const [filter, setFilter] = useState("");
  const [status, setStatus] = useState(order.includes(initialStatus) ? initialStatus : "");
  const headingId = useId();
  const priority = (state: string) => {
    const rank = order.indexOf(state);
    return rank < 0 ? order.length : rank;
  };
  const query = filter.toLowerCase().trim();
  const rows = subjects
    .flatMap((subject) => {
      const name = labels.subject(subject.id);
      const subjectMatch = (name + " " + subject.id).toLowerCase().includes(query);
      const operations = subject.capabilities
        .filter(
          (cap) =>
            (!status || cap.status === status) &&
            (subjectMatch ||
              (labels.operation(cap.id) + " " + cap.id).toLowerCase().includes(query)),
        )
        .sort((a, b) => priority(a.status) - priority(b.status));
      return operations.length ? [{ id: subject.id, name, operations }] : [];
    })
    .sort(
      (a, b) =>
        priority(a.operations[0].status) - priority(b.operations[0].status) ||
        a.name.localeCompare(b.name),
    );
  return (
    <section className="nk-cap-section" aria-labelledby={headingId}>
      <h2 id={headingId}>{labels.heading}</h2>
      <div className="nk-cap-controls">
        <label>
          {labels.filter}
          <input type="search" value={filter} onChange={(event) => setFilter(event.target.value)} />
        </label>
        <label>
          {labels.status}
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="">{labels.allStatuses}</option>
            {Object.entries(labels.states).map(([id, label]) => (
              <option key={id} value={id}>
                {label}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className="nk-cap-meta" role="status">
        {labels.count(rows.length, subjects.length)}
      </p>
      {rows.length ? (
        <ul className="nk-cap-subjects">
          {rows.map(({ id, name, operations }) => (
            <SubjectRow
              key={id}
              name={name}
              operations={operations}
              labels={labels}
              evidenceBaseUrl={evidenceBaseUrl}
            />
          ))}
        </ul>
      ) : (
        <div className="nk-cap-empty">
          <p>{labels.empty}</p>
          {(filter || status) && (
            <button
              type="button"
              onClick={() => {
                setFilter("");
                setStatus("");
              }}
            >
              {labels.clear}
            </button>
          )}
        </div>
      )}
    </section>
  );
}

/** A filterable list of subjects, each disclosing its operations and evidence. */
export default function CapabilityInventory(props: CapabilityInventoryProps) {
  return <Inventory key={props.initialStatus ?? ""} {...props} />;
}

export type Observation = {
  date: string;
  href?: string;
  items: { label: string; value: ReactNode }[];
};

export type ObservationListProps = {
  heading: string;
  intro: string;
  observations: Observation[];
  datasetsHeading: string;
  datasets: { href: string; label: string }[];
};

/** Dated observations, newest first, and links to the published data. */
export function ObservationList({
  heading,
  intro,
  observations,
  datasetsHeading,
  datasets,
}: ObservationListProps) {
  const headingId = useId();
  return (
    <section className="nk-cap-section" aria-labelledby={headingId}>
      <h2 id={headingId}>{heading}</h2>
      <p>{intro}</p>
      <ol className="nk-cap-history">
        {[...observations].reverse().map((entry) => (
          <li key={entry.date}>
            <h3>
              {entry.href ? (
                <a href={entry.href}>
                  <time dateTime={entry.date}>{entry.date}</time>
                </a>
              ) : (
                <time dateTime={entry.date}>{entry.date}</time>
              )}
            </h3>
            <dl>
              {entry.items.map((item) => (
                <div key={item.label}>
                  <dt>{item.label}</dt>
                  <dd>{item.value}</dd>
                </div>
              ))}
            </dl>
          </li>
        ))}
      </ol>
      <h3>{datasetsHeading}</h3>
      <ul className="nk-cap-datasets">
        {datasets.map((dataset) => (
          <li key={dataset.href}>
            <a href={dataset.href}>{dataset.label}</a>
          </li>
        ))}
      </ul>
    </section>
  );
}

export type CoverageLabels = {
  none: string;
  summary: (counts: { subjects: number; verified: number; failed: number }) => string;
  changes: (count: number) => string;
  change: (change: OperationChange) => string;
};

/**
 * One observation's coverage: subject and state counts, and the operations
 * whose state changed since the previous observation.
 */
export function CapabilityCoverage({
  profile,
  previous,
  labels,
}: {
  profile?: Profile;
  previous?: Profile;
  labels: CoverageLabels;
}) {
  if (!profile) return <>{labels.none}</>;
  const checks = Object.values(profile).flatMap((states) => Object.values(states));
  const summary = labels.summary({
    subjects: Object.keys(profile).length,
    verified: checks.filter((state) => state === "verified").length,
    failed: checks.filter((state) => state === "failed").length,
  });
  const differences = operationChanges(profile, previous).map(labels.change);
  if (!differences.length) return <>{summary}</>;
  return (
    <details>
      <summary>
        {summary} · {labels.changes(differences.length)}
      </summary>
      <ul>
        {differences.map((change) => (
          <li key={change}>{change}</li>
        ))}
      </ul>
    </details>
  );
}
