import { useState } from "react";
import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import data from "./_authoringlab.json";

// The authoring lab: what a coordinate does to a document, shown rather than
// scored.
//
// An agent reads a real repository — ripgrep, pinned — and documents one part of
// it, twice: once from the task alone, once with the governance bound at a
// coordinate. Four models, two coordinates.
//
// Nothing here is a score. Nobody has written down what a good user guide is,
// and a rubric invented for this page would be measuring the rubric. What the
// page does instead is show three things a reader needs to judge for themselves:
// the documents, rendered as Markdown rather than shown as source; the exact
// context each arm was given; and which files the agent opened, which is most of
// the difference between a document grounded in the source and one recalled.
//
// Regenerate with `./scripts/fetch-lab-repo.sh` then `make authoring-lab`.

interface AgentRun {
  text: string;
  filesRead?: string[];
  searches?: string[];
  messages: number;
  durationMs: number;
  inputTokens?: number;
  outputTokens?: number;
  costUsd?: number;
  model?: string;
  error?: string;
}
interface Doc {
  model: string;
  audience: string;
  bare: AgentRun;
  governed: AgentRun;
  bareFile: string;
  governedFile: string;
}
interface Report {
  generated: string;
  repo: string;
  guides: Record<string, string>;
  tasks: Record<string, string>;
  labels: Record<string, string>;
  profile: string;
  runner: string;
  docs: Doc[];
}

const report = data as Report;
const mono = "var(--ifm-font-family-monospace)";

const s: Record<string, CSSProperties> = {
  lede: { maxWidth: "62ch", color: "var(--ifm-color-emphasis-800)" },
  h: {
    fontSize: ".7rem",
    textTransform: "uppercase",
    letterSpacing: ".08em",
    color: "var(--ifm-color-emphasis-700)",
    marginBottom: ".35rem",
  },
  sub: { fontSize: ".8rem", color: "var(--ifm-color-emphasis-700)" },
  tab: {
    font: "inherit",
    fontSize: ".85rem",
    padding: ".35rem .85rem",
    borderRadius: 6,
    border: "1px solid var(--ifm-color-emphasis-300)",
    background: "var(--ifm-background-surface-color)",
    color: "var(--ifm-font-color-base)",
    cursor: "pointer",
  },
  tabOn: {
    borderColor: "var(--ifm-color-primary)",
    color: "var(--ifm-color-primary)",
    fontWeight: 600,
  },
  panel: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    background: "var(--ifm-background-surface-color)",
    padding: "1rem 1.1rem",
    minWidth: 0,
  },
  doc: {
    fontSize: ".88rem",
    lineHeight: 1.6,
    maxHeight: "40rem",
    overflow: "auto",
    minWidth: 0,
    // Rendered Markdown, so the document reads as a document. Headings inside
    // it are content, not page structure, so they are scaled down to sit under
    // the panel's own heading.
    ["--ifm-h1-font-size" as string]: "1.25rem",
    ["--ifm-h2-font-size" as string]: "1.1rem",
    ["--ifm-h3-font-size" as string]: "1rem",
  },
  pre: {
    fontFamily: mono,
    fontSize: ".76rem",
    lineHeight: 1.5,
    margin: 0,
    padding: ".6rem .75rem",
    background: "var(--ifm-color-emphasis-100)",
    borderRadius: 5,
    maxHeight: "22rem",
    overflow: "auto",
    whiteSpace: "pre-wrap",
    minWidth: 0,
  },
  file: {
    fontFamily: mono,
    fontSize: ".74rem",
    background: "var(--ifm-color-emphasis-100)",
    borderRadius: 4,
    padding: ".1rem .4rem",
  },
};

function fmtTokens(n?: number): string {
  if (!n) return "—";
  return n >= 1000 ? `${Math.round(n / 1000)}k` : String(n);
}

// Reading shows what the agent opened before it wrote. A document from a model
// that read nothing is a document about what it already believed.
function Reading({ run }: { run: AgentRun }): ReactElement {
  const files = run.filesRead ?? [];
  return (
    <div style={{ marginBottom: ".7rem" }}>
      <div style={s.sub}>
        {files.length} file{files.length === 1 ? "" : "s"} read · {fmtTokens(run.inputTokens)}{" "}
        context · {run.messages} messages · {(run.durationMs / 1000).toFixed(0)}s
        {run.costUsd ? ` · $${run.costUsd.toFixed(2)}` : ""}
      </div>
      {files.length > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: ".3rem", marginTop: ".3rem" }}>
          {files.map((f) => (
            <span key={f} style={s.file}>
              {f}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

function Arm({ label, run, file }: { label: string; run: AgentRun; file: string }): ReactElement {
  const base = useBaseUrl("/authoring-lab/");
  return (
    <div style={s.panel}>
      <div
        style={{
          display: "flex",
          alignItems: "baseline",
          gap: ".6rem",
          marginBottom: ".5rem",
          flexWrap: "wrap",
        }}
      >
        <strong>{label}</strong>
        {file && (
          <a href={base + file} download style={{ ...s.sub, marginLeft: "auto" }}>
            open the file
          </a>
        )}
      </div>
      <Reading run={run} />
      {run.error ? (
        <p style={{ color: "var(--ifm-color-danger)", fontSize: ".85rem", margin: 0 }}>
          {run.error}
        </p>
      ) : (
        <div style={s.doc}>
          <Markdown remarkPlugins={[remarkGfm]}>{run.text}</Markdown>
        </div>
      )}
    </div>
  );
}

export default function AuthoringLab(): ReactElement {
  const models = [...new Set(report.docs.map((d) => d.model))];
  // From the documents, not from report.labels: that is a Go map, so its JSON
  // key order is whatever the runtime felt like, and the default tab would move
  // between runs of the generator.
  const audiences = [...new Set(report.docs.map((d) => d.audience))];
  const [model, setModel] = useState(models[0] ?? "");
  const [audience, setAudience] = useState(audiences[0] ?? "");
  const doc = report.docs.find((d) => d.model === model && d.audience === audience);

  return (
    <Layout
      title="Authoring lab"
      description="An agent reads ripgrep and documents it, with and without the governance bound at a coordinate, across four models."
    >
      <main className="container margin-vert--lg" style={{ maxWidth: "84rem" }}>
        <h1>What a coordinate does to a document</h1>
        <p style={s.lede}>
          An agent is given a real repository — <code>{report.repo}</code>, pinned — and asked to
          document one part of it. It reads whatever it needs. The same task runs twice: once from
          the task alone, once with the voice profile resolved at a coordinate appended to its
          system prompt. Two coordinates, four models.
        </p>
        <p style={s.lede}>
          Nothing here is scored. What a good user guide contains is not written down anywhere, and
          a rubric invented for this page would be measuring the rubric. What is shown instead is
          the document, the exact context that produced it, and which files the agent opened before
          writing — a document from a run that read nothing is a document about what the model
          already believed.
        </p>
        <p style={s.sub}>
          {report.runner} Generated {report.generated}.
        </p>

        <div style={{ display: "flex", gap: ".5rem", flexWrap: "wrap", margin: "1.4rem 0 .6rem" }}>
          {models.map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setModel(m)}
              style={{ ...s.tab, ...(m === model ? s.tabOn : {}) }}
            >
              {m}
            </button>
          ))}
        </div>
        <div style={{ display: "flex", gap: ".5rem", flexWrap: "wrap", marginBottom: "1.2rem" }}>
          {audiences.map((a) => (
            <button
              key={a}
              type="button"
              onClick={() => setAudience(a)}
              style={{ ...s.tab, ...(a === audience ? s.tabOn : {}) }}
            >
              {report.labels[a] ?? a}
            </button>
          ))}
        </div>

        <div style={{ ...s.panel, marginBottom: "1.2rem" }}>
          <div style={s.h}>The task, given to both arms</div>
          <p style={{ margin: "0 0 .9rem", fontSize: ".88rem", lineHeight: 1.55 }}>
            {report.tasks[audience]}
          </p>
          <div style={s.h}>What the governed arm additionally received</div>
          <pre style={s.pre}>{report.guides[audience]}</pre>
          <p style={{ ...s.sub, margin: ".5rem 0 0" }}>
            Rendered by <code>kapi voice guide</code> from ripgrep&apos;s own voice profile,
            resolved at this coordinate. The profile itself is at the bottom of the page.
          </p>
        </div>

        {!doc && <p>No document for this cell.</p>}
        {doc && (
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fit, minmax(26rem, 1fr))",
              gap: "1rem",
            }}
          >
            <Arm label="Bare" run={doc.bare} file={doc.bareFile} />
            <Arm label="Governed by the coordinate" run={doc.governed} file={doc.governedFile} />
          </div>
        )}

        <details style={{ marginTop: "1.6rem" }}>
          <summary style={{ cursor: "pointer", fontWeight: 600 }}>
            ripgrep&apos;s voice profile, inferred by kapi from its own docs and corrected by hand
          </summary>
          <pre style={{ ...s.pre, marginTop: ".6rem", maxHeight: "34rem" }}>{report.profile}</pre>
        </details>
      </main>
    </Layout>
  );
}
