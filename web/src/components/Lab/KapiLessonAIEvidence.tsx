import React, { useState } from "react";
import Markdown from "react-markdown";
import data from "../../pages/authoring-lab/_authoringlab.json";
import styles from "./KapiLessonEvidence.module.css";

interface Run {
  text: string;
  filesRead?: string[];
  kapiCommands?: string[];
  error?: string;
}
interface RecordedAuthoring {
  generated: string;
  repo: string;
  runner: string;
  guides: Record<string, string>;
  tasks: Record<string, string>;
  labels: Record<string, string>;
  docs: Array<{ model: string; audience: string; bare: Run; governed: Run; pulled?: Run }>;
}
const report = data as unknown as RecordedAuthoring;
const models = [...new Set(report.docs.map((doc) => doc.model))];
const audiences = [...new Set(report.docs.map((doc) => doc.audience))];
function Arm({ title, run }: { title: string; run: Run }): React.ReactElement {
  return (
    <section className={styles.panel}>
      <h3>{title}</h3>
      {run.error ? <p role="status">Recorded error: {run.error}</p> : null}
      <details>
        <summary>Files read ({run.filesRead?.length ?? 0})</summary>
        <ul>
          {(run.filesRead ?? []).map((file) => (
            <li key={file}>
              <code>{file}</code>
            </li>
          ))}
        </ul>
      </details>
      <details>
        <summary>Recorded kapi commands</summary>
        <pre>{run.kapiCommands?.join("\n") || "No kapi commands recorded for this arm."}</pre>
      </details>
      <Markdown>{run.text || "No document was produced in this recorded run."}</Markdown>
    </section>
  );
}
const downloadHref = `data:application/json;charset=utf-8,${encodeURIComponent(JSON.stringify(report, null, 2))}`;

export default function KapiLessonAIEvidence({
  authoringHref = "/authoring-lab",
}: {
  authoringHref?: string;
}): React.ReactElement {
  const [model, setModel] = useState(models[0]);
  const [audience, setAudience] = useState(audiences[0]);
  const [delivery, setDelivery] = useState<"governed" | "pulled">("governed");
  const doc = report.docs.find((row) => row.model === model && row.audience === audience);
  if (!doc) return <p role="status">Recorded authoring evidence is unavailable.</p>;
  const comparison = doc[delivery];
  return (
    <>
      <p>
        <strong>Recorded model execution.</strong> These documents come from the same task in the
        pinned {report.repo} repository. The selector opens an existing result. It makes no model
        calls.
      </p>
      <div className={styles.controls}>
        <label>
          Model
          <select value={model} onChange={(event) => setModel(event.target.value)}>
            {models.map((value) => (
              <option key={value}>{value}</option>
            ))}
          </select>
        </label>
        <label>
          Audience
          <select value={audience} onChange={(event) => setAudience(event.target.value)}>
            {audiences.map((value) => (
              <option key={value} value={value}>
                {report.labels[value] ?? value}
              </option>
            ))}
          </select>
        </label>
        <label>
          Context delivery
          <select
            value={delivery}
            onChange={(event) => setDelivery(event.target.value as "governed" | "pulled")}
          >
            <option value="governed">Guide supplied in the prompt</option>
            <option value="pulled">Guide available in the workspace</option>
          </select>
        </label>
      </div>
      <p role="status">
        Showing {model}, {report.labels[audience] ?? audience},{" "}
        {delivery === "governed" ? "supplied guidance" : "workspace guidance"}.
      </p>
      <details open>
        <summary>Task shared by both runs</summary>
        <Markdown>{report.tasks[audience]}</Markdown>
      </details>
      <details>
        <summary>Applicable guide</summary>
        <Markdown>{report.guides[audience]}</Markdown>
      </details>
      <div className={styles.grid}>
        <Arm title="Task without supplied context" run={doc.bare} />
        {comparison ? (
          <Arm
            title={
              delivery === "governed"
                ? "Guide supplied in the prompt"
                : "Guide available for retrieval"
            }
            run={comparison}
          />
        ) : (
          <p role="status">No workspace-guidance run was recorded for this selection.</p>
        )}
      </div>
      <p className={styles.meta}>
        Recorded {report.generated}. {report.runner}
      </p>
      <p>
        <a href={authoringHref}>Inspect full sessions and reproduction details</a>. These examples
        carry no independent quality scores. Ranking models or delivery methods in general needs
        repeated, independently assessed runs.
      </p>
      <a download="authoring-evidence.json" href={downloadHref}>
        Download the recorded authoring evidence
      </a>
    </>
  );
}
