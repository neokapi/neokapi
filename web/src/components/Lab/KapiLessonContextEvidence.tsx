import React, { useState } from "react";
import data from "../../pages/content-lab/_evidence.json";
import type { AudienceEvidence, RecordedCase } from "../../pages/content-lab/_types";
import styles from "./KapiLessonEvidence.module.css";

const evidence = data as unknown as AudienceEvidence;
const audiences = [...new Set(evidence.results.map((row) => row.audience))];
function Context({ row }: { row: RecordedCase }): React.ReactElement {
  return (
    <section className={styles.panel}>
      <h3>{row.input.audience}: resolved context</h3>
      <p>
        <code>{row.file}</code>
      </p>
      <dl>
        {Object.entries(row.resolvedContext.point.coordinates ?? {}).map(([key, value]) => (
          <React.Fragment key={key}>
            <dt>{key}</dt>
            <dd>{value}</dd>
          </React.Fragment>
        ))}
      </dl>
      <details open>
        <summary>Applicable voice guide</summary>
        <pre>{row.resolvedContext.voice?.guide ?? "No voice guide recorded."}</pre>
      </details>
      <details>
        <summary>Full recorded context answer</summary>
        <pre>{JSON.stringify(row.resolvedContext, null, 2)}</pre>
      </details>
    </section>
  );
}
const downloadHref = `data:application/json;charset=utf-8,${encodeURIComponent(JSON.stringify(evidence, null, 2))}`;

export default function KapiLessonContextEvidence(): React.ReactElement {
  const [audience, setAudience] = useState(
    audiences.find((value) => value !== "child") ?? audiences[0],
  );
  const baseline = evidence.results.find((row) => row.id === "child/clean");
  const selected = evidence.results.find((row) => row.id === `${audience}/clean`);
  if (!baseline || !selected) return <p role="status">Recorded context evidence is unavailable.</p>;
  return (
    <>
      <p>
        <strong>Recorded native execution.</strong> These answers came from the same fixture with
        different audience coordinates. Changing this selector retrieves a recorded answer; it does
        not query a browser context graph.
      </p>
      <div className={styles.controls}>
        <label>
          Compare the child audience with
          <select value={audience} onChange={(event) => setAudience(event.target.value)}>
            {audiences.map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p role="status">Comparing child with {audience}.</p>
      <div className={styles.grid}>
        <Context row={baseline} />
        <Context row={selected} />
      </div>
      <details>
        <summary>Recording and reproduction</summary>
        <p className={styles.meta}>
          Recorded {evidence.recordedAt}. Source {evidence.sourceCommit}
          {evidence.sourceDirty ? " (with uncommitted changes)" : ""}. Binary:{" "}
          {evidence.binaryVersion}.
        </p>
        <p>
          The{" "}
          <a href="https://github.com/neokapi/neokapi/tree/main/samples/audience-context">
            audience fixture
          </a>{" "}
          includes its source files, shared constraints and an isolated runner. From a repository
          checkout:
        </p>
        <pre>
          {
            "make build\nnode samples/audience-context/run.mjs --binary bin/kapi --output /tmp/audience-results.json"
          }
        </pre>
        <a download="audience-context-evidence.json" href={downloadHref}>
          Download the complete recorded evidence
        </a>
      </details>
    </>
  );
}
