import { useId, useState } from "react";
import { ChevronDown } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { formats } from "@neokapi/reference-data";
import current from "../../../static/data/content-capabilities.json";
import observations from "../../../static/data/content-capabilities-history.json";
import { operationChanges, type History, type Profile } from "./_history";
import styles from "./_CapabilityEvidence.module.css";

type Group = "outputs" | "connectors";
type Capability = { id: string; status: string; evidence: string[]; detail: string };
type Subject = { id: string; capabilities: Capability[] };
type InventoryProps = { group: Group; initialStatus?: string; evidenceBaseUrl: string };

const repo = "https://github.com/neokapi/neokapi";
const outputNames = new Map(
  formats.entries
    .filter((entry) => entry.source === "built-in")
    .map((entry) => [entry.id, entry.displayName]),
);

function subjectName(group: Group, id: string) {
  if (group === "outputs") {
    if (id === "json-catalog") return t("JSON catalog", "output format");
    if (id === "yaml-catalog") return t("YAML catalog", "output format");
    return outputNames.get(id) ?? id;
  }
  const connectors: Record<string, string> = {
    "bowrain-source": t("Bowrain source", "connector name"),
    figma: "Figma",
    file: t("Local files", "connector name"),
    forge: t("Git forge", "connector name"),
    git: "Git",
    hubspot: "HubSpot",
    wordpress: "WordPress",
  };
  return connectors[id] ?? id;
}

function operationName(id: string) {
  const names: Record<string, string> = {
    "project-text": t("Convert text", "output operation"),
    "project-cells": t("Convert spreadsheet cells", "output operation"),
    "project-cues": t("Convert subtitle cues", "output operation"),
    email: t("Convert email", "output operation"),
    structure: t("Preserve document structure", "output operation"),
    fetch: t("Fetch content", "connector operation"),
    list: t("List content", "connector operation"),
    publish: t("Publish content", "connector operation"),
    registration: t("Register connector", "connector operation"),
    pull: t("Pull content", "connector operation"),
    push: t("Push content", "connector operation"),
  };
  return names[id] ?? id;
}

function statusNames(): Record<string, string> {
  return {
    failed: t("Failed", "capability state"),
    unverified: t("Unverified", "capability state"),
    verified: t("Verified", "capability state"),
    declared: t("Declared", "capability state"),
    unsupported: t("Unsupported", "capability state"),
  };
}

function State({ status }: { status: string }) {
  return (
    <span className={styles.state} data-state={status}>
      {statusNames()[status] ?? status}
    </span>
  );
}

function SubjectRow({
  name,
  operations,
  evidenceBaseUrl,
}: {
  name: string;
  operations: Capability[];
  evidenceBaseUrl: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const id = useId();
  const first = operations[0];
  return (
    <li className={styles.row}>
      <button
        className={styles.disclosure}
        type="button"
        aria-expanded={expanded}
        aria-controls={id}
        onClick={() => setExpanded(!expanded)}
      >
        <strong>{name}</strong>
        <span className={styles.operation}>{operationName(first.id)}</span>
        <State status={first.status} />
        <ChevronDown aria-hidden="true" size={16} className={styles.chevron} />
      </button>
      <div hidden={!expanded} id={id}>
        {expanded && (
          <ul className={styles.operations}>
            {operations.map((cap) => (
              <li key={cap.id}>
                <div className={styles.operationHeading}>
                  <strong>{operationName(cap.id)}</strong>
                  <State status={cap.status} />
                </div>
                <p>{cap.detail}</p>
                {cap.evidence.length > 0 && (
                  <ul className={styles.files}>
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

function Inventory({ group, initialStatus = "", evidenceBaseUrl }: InventoryProps) {
  const labels = statusNames();
  const [filter, setFilter] = useState("");
  const [status, setStatus] = useState(initialStatus in labels ? initialStatus : "");
  const priority = (state: string) => {
    const order = Object.keys(labels).indexOf(state);
    return order < 0 ? Object.keys(labels).length : order;
  };
  const subjects: Subject[] = current[group];
  const query = filter.toLowerCase().trim();
  const rows = subjects
    .flatMap((subject) => {
      const name = subjectName(group, subject.id);
      const subjectMatch = (name + " " + subject.id).toLowerCase().includes(query);
      const operations = subject.capabilities
        .filter(
          (cap) =>
            (!status || cap.status === status) &&
            (subjectMatch || (operationName(cap.id) + " " + cap.id).toLowerCase().includes(query)),
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
    <section className={styles.section} aria-labelledby="capability-evidence">
      <h2 id="capability-evidence">
        {group === "outputs"
          ? t("Output formats", "capability inventory")
          : t("Connectors", "capability inventory")}
      </h2>
      <div className={styles.controls}>
        <label>
          {group === "outputs"
            ? t("Find an output format or operation", "capability filter")
            : t("Find a connector or operation", "capability filter")}
          <input type="search" value={filter} onChange={(event) => setFilter(event.target.value)} />
        </label>
        <label>
          {t("Evidence status", "capability filter")}
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="">{t("All statuses", "capability filter")}</option>
            {Object.entries(labels).map(([id, label]) => (
              <option key={id} value={id}>
                {label}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className={styles.meta} role="status">
        {group === "outputs"
          ? t("{shown} of {total} output formats", "inventory count", {
              shown: rows.length,
              total: subjects.length,
            })
          : t("{shown} of {total} connectors", "inventory count", {
              shown: rows.length,
              total: subjects.length,
            })}
      </p>
      {rows.length ? (
        <ul className={styles.subjects}>
          {rows.map(({ id, name, operations }) => (
            <SubjectRow
              key={id}
              name={name}
              operations={operations}
              evidenceBaseUrl={evidenceBaseUrl}
            />
          ))}
        </ul>
      ) : (
        <div className={styles.empty}>
          <p>{t("No operations match these filters.", "capability filter")}</p>
          {(filter || status) && (
            <button
              type="button"
              onClick={() => {
                setFilter("");
                setStatus("");
              }}
            >
              {t("Clear filters", "capability filter")}
            </button>
          )}
        </div>
      )}
    </section>
  );
}

export default function CapabilityEvidence(props: InventoryProps) {
  return <Inventory key={props.group + "/" + (props.initialStatus ?? "")} {...props} />;
}

function changeCount(count: number) {
  return count === 1
    ? t("1 change", "observation history")
    : t("{count} changes", "observation history", { count });
}

function coverage(group: Group, profile?: Profile, previous?: Profile) {
  if (!profile) return t("No observation", "capability history");
  const checks = Object.values(profile).flatMap((states) => Object.values(states));
  const counts = {
    subjects: Object.keys(profile).length,
    verified: checks.filter((state) => state === "verified").length,
    failed: checks.filter((state) => state === "failed").length,
  };
  const summary =
    counts.subjects === 1
      ? t("1 subject · {verified} verified · {failed} failed", "capability history", counts)
      : t(
          "{subjects} subjects · {verified} verified · {failed} failed",
          "capability history",
          counts,
        );
  const labels = statusNames();
  const absent = t("absent", "historical observation");
  const differences = operationChanges(profile, previous).map(
    ({ subject, operation, before, after }) =>
      t("{subject} / {operation} · {before} → {after}", "capability change", {
        subject: subjectName(group, subject),
        operation: operationName(operation),
        before: labels[before] ?? before ?? absent,
        after: labels[after] ?? after ?? absent,
      }),
  );
  if (!differences.length) return summary;
  return (
    <details>
      <summary>
        {summary} · {changeCount(differences.length)}
      </summary>
      <ul>
        {differences.map((change) => (
          <li key={change}>{change}</li>
        ))}
      </ul>
    </details>
  );
}

function formatChanges(entry: History, previous?: History) {
  if (!previous) return t("First observation", "capability history");
  const ids = [
    ...new Set([...Object.keys(previous.formats), ...Object.keys(entry.formats)]),
  ].sort();
  const changes = ids.filter((id) => previous.formats[id] !== entry.formats[id]);
  if (!changes.length) return t("No Engine grade changes", "format history");
  const absent = t("absent", "historical observation");
  return (
    <details>
      <summary>{changeCount(changes.length)}</summary>
      <ul>
        {changes.map((id) => (
          <li key={id}>
            {subjectName("outputs", id)} · {previous.formats[id] ?? absent} →{" "}
            {entry.formats[id] ?? absent}
          </li>
        ))}
      </ul>
    </details>
  );
}

export function ObservationHistory({ dataBaseUrl }: { dataBaseUrl: string }) {
  const history = observations as History[];
  const headingId = useId();
  return (
    <section className={styles.section} aria-labelledby={headingId}>
      <h2 id={headingId}>{t("Observation history", "maturity dashboard")}</h2>
      <p>
        {t(
          "Each observation compares with the preceding published date. Output and connector measurements begin with the first operation audit.",
          "capability history",
        )}
      </p>
      <ol className={styles.history}>
        {history
          .map((entry, index) => {
            const previous = history[index - 1];
            return (
              <li key={entry.date}>
                <h3>
                  {entry.revision ? (
                    <a
                      href={
                        repo + "/blob/" + entry.revision + "/web/static/data/format-maturity.json"
                      }
                    >
                      <time dateTime={entry.date}>{entry.date}</time>
                    </a>
                  ) : (
                    <time dateTime={entry.date}>{entry.date}</time>
                  )}
                </h3>
                <dl>
                  <div>
                    <dt>{t("Engine grades", "observation history")}</dt>
                    <dd>{formatChanges(entry, previous)}</dd>
                  </div>
                  <div>
                    <dt>{t("Output formats", "capability inventory")}</dt>
                    <dd>{coverage("outputs", entry.outputs, previous?.outputs)}</dd>
                  </div>
                  <div>
                    <dt>{t("Connectors", "capability inventory")}</dt>
                    <dd>{coverage("connectors", entry.connectors, previous?.connectors)}</dd>
                  </div>
                </dl>
              </li>
            );
          })
          .reverse()}
      </ol>
      <h3>{t("Source data", "observation history")}</h3>
      <ul className={styles.datasets}>
        <li>
          <a href={dataBaseUrl + "content-capabilities.json"}>
            {t("Current capability evidence (JSON)", "observation history")}
          </a>
        </li>
        <li>
          <a href={dataBaseUrl + "content-capabilities-history.json"}>
            {t("Historical observations (JSON)", "observation history")}
          </a>
        </li>
        <li>
          <a href={dataBaseUrl + "format-maturity-history.json"}>
            {t("Format grade totals by axis and date (JSON)", "observation history")}
          </a>
        </li>
      </ul>
    </section>
  );
}
