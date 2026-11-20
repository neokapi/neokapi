import { useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import current from "../../../static/data/content-capabilities.json";
import observations from "../../../static/data/content-capabilities-history.json";
import styles from "./index.module.css";

type Capability = { id: string; status: string; evidence: string[]; detail: string };
type Subject = { id: string; capabilities: Capability[] };
type History = {
  date: string;
  revision?: string;
  formats: Partial<Record<string, string>>;
  outputs?: Record<string, Record<string, string>>;
  connectors?: Record<string, Record<string, string>>;
};

const groups = {
  outputs: t("Output formats", "capability inventory"),
  connectors: t("Connectors", "capability inventory"),
};
const labels: Record<string, string> = {
  verified: t("verified", "capability state"),
  declared: t("declared", "capability state"),
  unverified: t("unverified", "capability state"),
  failed: t("failed", "capability state"),
  unsupported: t("unsupported", "capability state"),
};
const absent = t("absent", "historical observation");
const meanings: Record<string, string> = {
  verified: t("The operation passed its behavioral check.", "capability evidence"),
  declared: t("The registry declares this capability.", "capability evidence"),
  unverified: t("The audit has no passing check for this operation.", "capability evidence"),
  failed: t("The operation failed its behavioral check.", "capability evidence"),
  unsupported: t("The implementation explicitly excludes this operation.", "capability evidence"),
};
const repo = "https://github.com/neokapi/neokapi";

function Files({ evidence }: { evidence: string[] }) {
  return evidence.map((ref) => {
    const [file, test] = ref.split("#");
    return (
      <div key={ref}>
        <a href={repo + "/blob/main/" + file}>{test || file}</a>
      </div>
    );
  });
}

type Profile = Record<string, Record<string, string>>;

function coverage(profile?: Profile, previous?: Profile) {
  if (!profile) return t("No observation", "capability history");
  const checks = Object.values(profile).flatMap((states) => Object.values(states));
  const verified = checks.filter((state) => state === "verified").length;
  const failed = checks.filter((state) => state === "failed").length;
  const summary = t(
    "{subjects} subjects · {verified} verified · {failed} failed",
    "capability history",
    {
      subjects: Object.keys(profile).length,
      verified,
      failed,
    },
  );
  if (!previous) return summary;
  const differences = [];
  for (const subject of [...new Set([...Object.keys(profile), ...Object.keys(previous)])].sort()) {
    const now = profile[subject] ?? {};
    const before = previous[subject] ?? {};
    for (const operation of [...new Set([...Object.keys(now), ...Object.keys(before)])].sort()) {
      if (before[operation] !== now[operation]) {
        differences.push(
          t("{subject} / {operation} · {before} → {after}", "capability change", {
            subject,
            operation,
            before: labels[before[operation]] ?? absent,
            after: labels[now[operation]] ?? absent,
          }),
        );
      }
    }
  }
  if (!differences.length) return summary;
  return (
    <details>
      <summary>
        {summary} · {t("{count} changes", "observation history", { count: differences.length })}
      </summary>
      <ul>
        {differences.map((change) => (
          <li key={change}>{change}</li>
        ))}
      </ul>
    </details>
  );
}

export default function CapabilityEvidence({ dataBaseUrl }: { dataBaseUrl: string }) {
  const [group, setGroup] = useState<keyof typeof groups>("outputs");
  const [filter, setFilter] = useState("");
  const [status, setStatus] = useState("");
  const history = observations as History[];
  const subjects: Subject[] = current[group];
  const query = filter.toLowerCase().trim();
  const rows = subjects.flatMap((row) =>
    row.capabilities
      .filter(
        (cap) =>
          (!status || cap.status === status) && (row.id.includes(query) || cap.id.includes(query)),
      )
      .map((cap) => ({ subject: row.id, ...cap })),
  );
  return (
    <section id="capability-evidence">
      <h2>Output formats and connectors</h2>
      <p>
        The audit records each operation separately. Open the evidence for the file or test behind a
        result.
      </p>
      <div className={styles.controls}>
        <label>
          Inventory{" "}
          <select
            value={group}
            onChange={(event) => setGroup(event.target.value as keyof typeof groups)}
          >
            {Object.entries(groups).map(([id, label]) => (
              <option key={id} value={id}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label>
          Evidence{" "}
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="">All states</option>
            {Object.keys(meanings).map((id) => (
              <option key={id} value={id}>
                {labels[id]}
              </option>
            ))}
          </select>
        </label>
        <label>
          Find{" "}
          <input
            className={styles.search}
            value={filter}
            placeholder="Subject or operation"
            onChange={(event) => setFilter(event.target.value)}
          />
        </label>
      </div>
      <p className={styles.meta} aria-live="polite">
        {t("{operations} operations · measured {date}", "capability audit", {
          operations: rows.length,
          date: current.date,
        })}
      </p>
      <div className={styles.tableWrap}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Subject</th>
              <th>Operation</th>
              <th>State</th>
              <th>Coverage</th>
              <th>Evidence</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.subject + "/" + row.id}>
                <td>
                  <code>{row.subject}</code>
                </td>
                <td>{row.id}</td>
                <td title={meanings[row.status]}>
                  <strong>{labels[row.status]}</strong>
                </td>
                <td>{row.detail}</td>
                <td>
                  <Files evidence={row.evidence} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {!rows.length && <p>No operations match these filters.</p>}
      <h2 id="observation-history">Observation history</h2>
      <p>
        Earlier publications recorded format grades. Output and connector counts start with the
        first operation audit.
      </p>
      <div className={styles.tableWrap}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Date</th>
              <th>Format changes since the preceding observation</th>
              <th>Output formats</th>
              <th>Connectors</th>
            </tr>
          </thead>
          <tbody>
            {history
              .map((entry, index) => {
                const previous = history[index - 1]?.formats;
                const changes = previous
                  ? [...new Set([...Object.keys(previous), ...Object.keys(entry.formats)])]
                      .sort()
                      .filter((id) => previous[id] !== entry.formats[id])
                  : [];
                return (
                  <tr key={entry.date}>
                    <td>
                      {entry.revision ? (
                        <a
                          href={
                            repo +
                            "/blob/" +
                            entry.revision +
                            "/web/static/data/format-maturity.json"
                          }
                        >
                          {entry.date}
                        </a>
                      ) : (
                        entry.date
                      )}
                    </td>
                    <td>
                      {!previous ? (
                        t("First observation", "capability history")
                      ) : changes.length ? (
                        <details>
                          <summary>
                            {t("{count} changes", "observation history", { count: changes.length })}
                          </summary>
                          <ul>
                            {changes.map((id) => (
                              <li key={id}>
                                {id} · {previous[id] ?? absent} → {entry.formats[id] ?? absent}
                              </li>
                            ))}
                          </ul>
                        </details>
                      ) : (
                        t("No Engine grade changes", "format history")
                      )}
                    </td>
                    <td>{coverage(entry.outputs, history[index - 1]?.outputs)}</td>
                    <td>{coverage(entry.connectors, history[index - 1]?.connectors)}</td>
                  </tr>
                );
              })
              .reverse()}
          </tbody>
        </table>
      </div>
      <p>
        <a href={dataBaseUrl + "content-capabilities.json"}>Current capability evidence (JSON)</a>
        {" · "}
        <a href={dataBaseUrl + "content-capabilities-history.json"}>Historical observations (JSON)</a>
      </p>
    </section>
  );
}
