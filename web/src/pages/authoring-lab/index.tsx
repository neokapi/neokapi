import { useState } from "react";
import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import data from "./_authoringlab.json";

// The authoring lab: what a coordinate does to a document, shown rather than
// scored.
//
// Same product, same feature, same brand voice, same facts. Two coordinates
// differing only in what the reader is assumed to know, each written twice —
// once from the task alone, once with the governance bound at that point — on
// four models.
//
// Nothing here is a score. Nobody has yet written down what a good user guide
// is, and a rubric invented for this page would be measuring the rubric. What
// the page does instead is put the four documents where a reader can compare
// them, and mark the words each coordinate bans so the difference is visible
// without counting anything.
//
// Regenerate with `make authoring-lab`.

interface Doc {
  model: string;
  resolved?: string;
  audience: string;
  bare: string;
  governed: string;
  bareFile: string;
  governedFile: string;
  error?: string;
}
interface Report {
  generated: string;
  brief: string;
  guides: Record<string, string>;
  tasks: Record<string, string>;
  labels: Record<string, string>;
  runner: string;
  docs: Doc[];
}

const report = data as Report;

// The words each coordinate bans, marked in the prose so a reader sees them
// appear and disappear rather than being told a count.
//
// Kept beside the corpus that declares them (scripts/authoringlab/corpus.go)
// rather than derived from it: this page highlights, it does not check, and a
// highlighter that silently drifted from the profile would be worse than one
// that is obviously a reading aid.
const banned: Record<string, RegExp> = {
  "end-user":
    /\b(endpoints?|payloads?|webhooks?|HTTP POST|JSON|HMAC(?:-SHA256)?|API|X-Harbourlight-Signature|exponential backoff|utilizes?|configur\w+)\b/gi,
  developer: /\b(simply|just|easy|easily|straightforward|utilizes?|seamless(?:ly)?)\b/gi,
};

const mono = "var(--ifm-font-family-monospace)";

const s: Record<string, CSSProperties> = {
  lede: { maxWidth: "60ch", color: "var(--ifm-color-emphasis-800)" },
  h: {
    fontSize: ".7rem",
    textTransform: "uppercase",
    letterSpacing: ".08em",
    color: "var(--ifm-color-emphasis-700)",
    marginBottom: ".3rem",
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
    fontSize: ".85rem",
    lineHeight: 1.6,
    whiteSpace: "pre-wrap",
    wordBreak: "break-word",
    maxHeight: "34rem",
    overflow: "auto",
    margin: 0,
    fontFamily: "var(--ifm-font-family-base)",
    minWidth: 0,
  },
  mark: {
    background: "var(--ifm-color-warning-contrast-background)",
    color: "var(--ifm-color-warning-contrast-foreground)",
    borderRadius: 3,
    padding: "0 .15em",
    fontWeight: 600,
  },
  pre: {
    fontFamily: mono,
    fontSize: ".78rem",
    lineHeight: 1.5,
    margin: 0,
    padding: ".6rem .75rem",
    background: "var(--ifm-color-emphasis-100)",
    borderRadius: 5,
    overflowX: "auto",
    whiteSpace: "pre-wrap",
    minWidth: 0,
  },
};

// Marked splits prose on the banned words and highlights them.
function Marked({ text, pattern }: { text: string; pattern: RegExp }): ReactElement {
  const re = new RegExp(pattern.source, pattern.flags);
  const out: (string | ReactElement)[] = [];
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) out.push(text.slice(last, m.index));
    out.push(
      <mark key={`${m.index}`} style={s.mark}>
        {m[0]}
      </mark>,
    );
    last = m.index + m[0].length;
    if (m[0].length === 0) re.lastIndex++;
  }
  out.push(text.slice(last));
  return <>{out}</>;
}

function count(text: string, pattern: RegExp): number {
  return (text.match(new RegExp(pattern.source, pattern.flags)) ?? []).length;
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
  const pattern = banned[audience] ?? /$^/g;
  const docBase = useBaseUrl("/authoring-lab/");

  return (
    <Layout
      title="Authoring lab"
      description="The same document written at two coordinates, with and without the governance bound there, across four models."
    >
      <main className="container margin-vert--lg" style={{ maxWidth: "80rem" }}>
        <h1>What a coordinate does to a document</h1>
        <p style={s.lede}>
          One product, one feature, one brand voice, one set of facts. Two coordinates that differ
          only in what the reader is assumed to know. Each written twice: from the task alone, and
          with the voice profile bound at that point. Four models.
        </p>
        <p style={s.lede}>
          Nothing here is scored. What a good user guide contains is not written down anywhere, and
          a rubric invented for this page would be measuring the rubric. The documents are the
          result; the highlighting marks the words each coordinate bans so you can see them go.
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
              {report.labels[a]}
            </button>
          ))}
        </div>

        <div style={{ ...s.panel, marginBottom: "1.2rem" }}>
          <div style={s.h}>The task, given to both arms</div>
          <p style={{ margin: "0 0 .8rem", fontSize: ".88rem" }}>{report.tasks[audience]}</p>
          <div style={s.h}>What the governed arm additionally received</div>
          <pre style={s.pre}>{report.guides[audience]}</pre>
        </div>

        {!doc && <p>No document for this cell.</p>}
        {doc?.error && <p style={{ color: "var(--ifm-color-danger)" }}>{doc.error}</p>}
        {doc && !doc.error && (
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fit, minmax(24rem, 1fr))",
              gap: "1rem",
            }}
          >
            {(
              [
                ["Bare", doc.bare, doc.bareFile],
                ["Governed by the coordinate", doc.governed, doc.governedFile],
              ] as const
            ).map(([label, body, file]) => (
              <div key={label} style={s.panel}>
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
                  <span style={s.sub}>
                    {count(body, pattern)} banned term{count(body, pattern) === 1 ? "" : "s"} ·{" "}
                    {body.split(/\s+/).length} words
                  </span>
                  <a href={docBase + file} style={{ ...s.sub, marginLeft: "auto" }} download>
                    open the file
                  </a>
                </div>
                <div style={s.doc}>
                  <Marked text={body} pattern={pattern} />
                </div>
              </div>
            ))}
          </div>
        )}

        <details style={{ marginTop: "1.6rem" }}>
          <summary style={{ cursor: "pointer", fontWeight: 600 }}>
            The brief, identical in every cell
          </summary>
          <pre style={{ ...s.pre, marginTop: ".6rem" }}>{report.brief}</pre>
        </details>
      </main>
    </Layout>
  );
}
