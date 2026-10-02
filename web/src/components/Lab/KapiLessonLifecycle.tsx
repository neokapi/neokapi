import React, { useState } from "react";
import data from "./KapiLessonLifecycleEvidence.json";
import { parseContextAnswer, parseCheckAnswer } from "./KapiLessonRecords";
import styles from "./KapiLessonEvidence.module.css";

interface Command {
  command: string[];
  exitCode: number;
  stdout: string;
  stderr: string;
  kind?: string;
}
interface Recording {
  recordedAt: string;
  sourceCommit: string;
  sourceDirty: boolean;
  binaryVersion: string;
  binarySha256: string;
  failure?: string;
  steps: Array<{
    label: string;
    source: string;
    sourceSha256: string;
    action: Command | null;
    context: Command;
    log: Command;
    check: Command;
    search: Command;
  }>;
}
const recording = data as unknown as Recording;
const explanations: Record<string, string> = {
  "Before observation":
    "The fixture has no observed wording rule. The native source-only run reads its content and records a fresh context projection.",
  Suggested:
    "The context answer includes a suggestion with its operation id and preferred wording. A suggestion reports and never fails a check.",
  Established:
    "Keeping the observation writes preferred and forbidden terms. Inspect the operation that links the decision to the observation.",
  "Source edited":
    "The fixture changes the forecast duration. The recorded wording rule still applies: a source edit and a change to governing context are separate events.",
  "Source read again":
    "The native run reads the changed source and refreshes the context graph. Compare source freshness, term occurrences and context revision: the governing rule stays the same.",
  Reverted:
    "Reverting this observation retracts the forbidden variants. In this recording the preferred term remains. The operation log preserves the observation and the decisions about it.",
};
const downloadHref = `data:application/json;charset=utf-8,${encodeURIComponent(JSON.stringify(data, null, 2))}`;

export default function KapiLessonLifecycle(): React.ReactElement {
  const [index, setIndex] = useState(0);
  const step = recording.steps[index];
  if (recording.failure || !step)
    return <p role="status">The lifecycle recording is unavailable or incomplete.</p>;
  const context = parseContextAnswer(step.context.stdout);
  const check = step.check && parseCheckAnswer(step.check.stdout);
  const search = step.search && parseContextAnswer(step.search.stdout);
  if (!context || !check || !search)
    return (
      <p role="status">
        The selected recording contains an invalid context answer or check report. Download the
        evidence or reproduce the fixture to inspect it.
      </p>
    );
  return (
    <>
      <p>
        <strong>Recorded native execution.</strong> This isolated fixture used no AI provider.
        Select a step to inspect the actual source, check report and operation log.
      </p>
      <div className={styles.controls}>
        <label>
          Recorded step
          <select value={index} onChange={(event) => setIndex(Number(event.target.value))}>
            {recording.steps.map((item, i) => (
              <option value={i} key={item.label}>
                {i + 1}. {item.label}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p role="status">{explanations[step.label]}</p>
      <div className={styles.grid}>
        <section className={styles.panel}>
          <h3>Source and action</h3>
          <pre>{step.source}</pre>
          <p className={styles.meta}>Source SHA-256: {step.sourceSha256}</p>
          {step.action ? (
            <>
              <h4>
                {step.action.kind === "fixture-file-edit" ? "Fixture edit" : "Command executed"}
              </h4>
              {step.action.command.length ? (
                <pre>
                  {["kapi", ...step.action.command]
                    .map((arg) => (/\s/.test(arg) ? JSON.stringify(arg) : arg))
                    .join(" ")}
                </pre>
              ) : null}
              <details>
                <summary>Recorded action output</summary>
                <pre>{step.action.stdout}</pre>
                <pre>{step.action.stderr}</pre>
              </details>
            </>
          ) : null}
        </section>
        <section className={styles.panel}>
          <h3>Resolved wording</h3>
          <p>Context revision: {context.provenance?.revision ?? "not recorded"}</p>
          {context.suggestions?.length ? (
            <ul>
              {context.suggestions.map((item) => (
                <li key={item.operation}>
                  {item.term} → {item.replacement} ({item.status}, operation {item.operation})
                </li>
              ))}
            </ul>
          ) : (
            <p>No suggestions returned.</p>
          )}
          {context.terms?.length ? (
            <ul>
              {context.terms.map((item, i) => (
                <li key={`${item.term}-${i}`}>
                  {item.term}: {item.status}
                  {item.replacement ? `; use ${item.replacement}` : ""}
                </li>
              ))}
            </ul>
          ) : (
            <p>No established terms returned.</p>
          )}
          {context.provenance?.stale_reason ? (
            <p>
              <strong>Recorded coverage limit:</strong> {context.provenance.stale_reason}
            </p>
          ) : null}
        </section>
      </div>
      <section className={styles.panel}>
        <h3>Recorded graph evidence</h3>
        <p>
          Source projection: {search.provenance?.stale ? "stale" : "fresh"}.{" "}
          {search.provenance?.stale_reason}
        </p>
        <p>
          The search resolves terms and their recorded occurrences in extracted blocks. A missing
          count means no usage count was returned.
        </p>
        {search.terms?.length ? (
          <ul>
            {search.terms.map((term) => (
              <li key={term.term}>
                <strong>{term.term}</strong>:{" "}
                {term.uses === undefined
                  ? "usage count not returned"
                  : `${term.uses} recorded occurrence(s)`}
                {term.top_uses?.length ? (
                  <ul>
                    {term.top_uses.map((use) => (
                      <li key={`${use.document}-${use.block_id}`}>
                        <code>{use.document}</code>, block <code>{use.block_id}</code>:{" "}
                        {use.snippet}
                      </li>
                    ))}
                  </ul>
                ) : null}
              </li>
            ))}
          </ul>
        ) : (
          <p>No established terms returned by this search.</p>
        )}
        <details>
          <summary>Full recorded context search</summary>
          <pre>{JSON.stringify(step.search.command)}</pre>
          <pre>{step.search.stdout}</pre>
        </details>
      </section>
      <section className={styles.panel}>
        <h3>Recorded check: {check.verdict}</h3>
        <p>
          Exit code {step.check.exitCode}. Findings: {check.summary.findings}; failing:{" "}
          {check.summary.failing}; reporting: {check.summary.reporting}.
        </p>
        {check.findings.length ? (
          <ul>
            {check.findings.map((finding, i) => (
              <li key={i}>
                <strong>{finding.fails ? "Fails the check" : "Reports only"}:</strong>{" "}
                {finding.message}
                {finding.suggestion ? `; ${finding.suggestion}` : ""}
              </li>
            ))}
          </ul>
        ) : (
          <p>No findings were recorded.</p>
        )}
        <details>
          <summary>Full check report and executed arguments</summary>
          <pre>{JSON.stringify(step.check.command)}</pre>
          <pre>{step.check.stdout}</pre>
          <pre>{step.check.stderr}</pre>
        </details>
      </section>
      <div className={styles.grid}>
        <details className={styles.panel}>
          <summary>Full context answer</summary>
          <pre>{step.context.stdout}</pre>
        </details>
        <details className={styles.panel}>
          <summary>Operation log</summary>
          <pre>{step.log.stdout}</pre>
        </details>
      </div>
      <details>
        <summary>Provenance and reproduction</summary>
        <p className={styles.meta}>
          Recorded {recording.recordedAt}. Source {recording.sourceCommit}
          {recording.sourceDirty ? " (with uncommitted changes)" : ""}. Binary{" "}
          {recording.binaryVersion}. SHA-256: {recording.binarySha256}.
        </p>
        <p>
          The{" "}
          <a href="https://github.com/neokapi/neokapi/tree/main/samples/context-lifecycle">
            fixture and runner
          </a>{" "}
          reproduce these commands in throwaway project and data directories. From a repository
          checkout:
        </p>
        <pre>
          {
            "make build\nnode samples/context-lifecycle/run.mjs --binary bin/kapi --output /tmp/context-lifecycle.json"
          }
        </pre>
        <a download="context-lifecycle-evidence.json" href={downloadHref}>
          Download the full recording
        </a>
      </details>
    </>
  );
}
