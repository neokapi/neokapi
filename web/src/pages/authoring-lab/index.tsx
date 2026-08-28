import { useEffect, useState } from "react";
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
  toolCalls?: Record<string, number>;
  inputTokens?: number;
  outputTokens?: number;
  costUsd?: number;
  model?: string;
  sandboxed?: boolean;
  transcript?: string;
  error?: string;
}
// One step of a recorded session, the same shape the skill eval publishes.
interface SessionEvent {
  kind: "text" | "tool";
  name?: string;
  text?: string;
  input?: string;
  output?: string;
  failed?: boolean;
}
interface LabSession {
  model: string;
  audience: string;
  arm: string;
  prompt: string;
  events: SessionEvent[];
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
  cmd: {
    fontFamily: mono,
    fontSize: ".78rem",
    padding: ".35rem .6rem",
    background: "var(--ifm-color-emphasis-100)",
    borderRadius: 4,
    borderLeft: "3px solid var(--ifm-color-primary)",
    overflowX: "auto",
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
  const tools = Object.entries(run.toolCalls ?? {}).sort((a, b) => b[1] - a[1]);
  return (
    <div style={{ marginBottom: ".7rem" }}>
      <div style={s.sub}>
        {files.length} file{files.length === 1 ? "" : "s"} read · {fmtTokens(run.inputTokens)}{" "}
        context · {run.messages} messages · {(run.durationMs / 1000).toFixed(0)}s
        {run.costUsd ? ` · $${run.costUsd.toFixed(2)}` : ""}
      </div>
      {/* How it read matters as much as how much. Models do not agree: some
          call the Read tool, opus-5 opens a source tree with `ls` and `cat`.
          Counting one and not the other reported zero files for a run that
          spent 1.7M tokens on the repository. */}
      {tools.length > 0 && (
        <div style={{ ...s.sub, marginTop: ".2rem" }}>
          {tools.map(([name, n]) => `${name}×${n}`).join(" · ")}
        </div>
      )}
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

function Arm({
  label,
  run,
  file,
  onOpenSession,
}: {
  label: string;
  run: AgentRun;
  file: string;
  onOpenSession: () => void;
}): ReactElement {
  const base = useBaseUrl("/authoring-lab/");
  return (
    <div style={s.panel}>
      <div
        style={{
          display: "flex",
          alignItems: "baseline",
          gap: ".6rem",
          marginBottom: ".6rem",
          flexWrap: "wrap",
        }}
      >
        <strong>{label}</strong>
        {run.sandboxed && (
          <span style={s.sub} title="Claude Code's sandbox confined this run">
            sandboxed
          </span>
        )}
      </div>

      {/* What went in. Boxed and labelled, because it used to run straight into
          the document below it and a reader could not tell where the inputs
          ended and the model's writing began. */}
      <div
        style={{
          background: "var(--ifm-color-emphasis-100)",
          borderRadius: 6,
          padding: ".6rem .75rem",
          marginBottom: ".9rem",
        }}
      >
        <div style={s.h}>What it read</div>
        <Reading run={run} />
        {run.transcript && (
          <button
            type="button"
            onClick={onOpenSession}
            style={{
              font: "inherit",
              fontSize: ".78rem",
              marginTop: ".45rem",
              padding: ".2rem .65rem",
              borderRadius: 6,
              border: "1px solid var(--ifm-color-emphasis-300)",
              background: "var(--ifm-background-surface-color)",
              color: "var(--ifm-font-color-base)",
              cursor: "pointer",
            }}
          >
            Full session
          </button>
        )}
      </div>

      {/* What came out. */}
      <div
        style={{
          display: "flex",
          alignItems: "baseline",
          gap: ".6rem",
          marginBottom: ".4rem",
          borderTop: "1px solid var(--ifm-color-emphasis-300)",
          paddingTop: ".7rem",
        }}
      >
        <div style={s.h}>The document it wrote</div>
        {file && (
          <a href={base + file} download style={{ ...s.sub, marginLeft: "auto" }}>
            open the file
          </a>
        )}
      </div>
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
  const [session, setSession] = useState<string | null>(null);
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
            <Arm
              label="Bare"
              run={doc.bare}
              file={doc.bareFile}
              onOpenSession={() => setSession(doc.bare.transcript ?? null)}
            />
            <Arm
              label="Governed by the coordinate"
              run={doc.governed}
              file={doc.governedFile}
              onOpenSession={() => setSession(doc.governed.transcript ?? null)}
            />
          </div>
        )}

        {session && <SessionModal file={session} onClose={() => setSession(null)} />}

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

// SessionModal draws one arm's whole session over the page.
//
// The lab drives a sixty-turn agent and showed none of it: a file count says
// the agent read ten files, and only the session says which ten and what it
// made of them. Fetched on demand — the dataset is imported into the bundle and
// a session per cell would be paid for by every reader of the summary.
function SessionModal({ file, onClose }: { file: string; onClose: () => void }): ReactElement {
  const dir = useBaseUrl("/authoring-lab/transcripts/");
  const [sess, setSess] = useState<LabSession | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let live = true;
    fetch(dir + file)
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((b: LabSession) => {
        if (live) setSess(b);
      })
      .catch(() => {
        if (live) setFailed(true);
      });
    return () => {
      live = false;
    };
  }, [dir, file]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    const prior = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prior;
    };
  }, [onClose]);

  return (
    <div
      role="presentation"
      onClick={onClose}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,.55)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: "clamp(.5rem, 4vw, 3rem)",
        zIndex: 400,
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`Session transcript: ${file}`}
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "var(--ifm-background-surface-color)",
          border: "1px solid var(--ifm-color-emphasis-300)",
          borderRadius: 10,
          width: "min(100%, 62rem)",
          maxHeight: "100%",
          minWidth: 0,
          display: "flex",
          flexDirection: "column",
          boxShadow: "0 18px 48px rgba(0,0,0,.35)",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "flex-start",
            gap: "1rem",
            padding: "1rem 1.2rem",
            borderBottom: "1px solid var(--ifm-color-emphasis-200)",
          }}
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontWeight: 700 }}>
              {sess ? `${sess.model} · ${sess.audience} · ${sess.arm}` : "Session"}
            </div>
            {sess && <div style={{ ...s.sub, marginTop: ".15rem" }}>{sess.prompt}</div>}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close the transcript"
            style={{
              font: "inherit",
              fontSize: "1.1rem",
              lineHeight: 1,
              padding: ".3rem .6rem",
              borderRadius: 6,
              border: "1px solid var(--ifm-color-emphasis-300)",
              background: "var(--ifm-background-surface-color)",
              color: "var(--ifm-font-color-base)",
              cursor: "pointer",
            }}
          >
            ×
          </button>
        </div>
        <div style={{ overflow: "auto", padding: "1.1rem 1.2rem", minWidth: 0 }}>
          {failed && (
            <p style={{ ...s.sub, margin: 0 }}>
              The transcript could not be loaded. It is written by the run that produced this
              column.
            </p>
          )}
          {!sess && !failed && <p style={s.sub}>Loading the transcript…</p>}
          {sess && (
            <div style={{ minWidth: 0 }}>
              <Turn role="user">{sess.prompt}</Turn>
              {sess.events.map((e, i) => (
                <EventView key={i} e={e} />
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// Turn is one side of the conversation, labelled the way a session labels it.
function Turn({
  role,
  children,
}: {
  role: "user" | "assistant";
  children: React.ReactNode;
}): ReactElement {
  const isUser = role === "user";
  return (
    <div style={{ margin: "0 0 1rem", minWidth: 0 }}>
      <div
        style={{
          ...s.h,
          marginBottom: ".3rem",
          color: isUser ? "var(--ifm-color-primary)" : "var(--ifm-color-emphasis-700)",
        }}
      >
        {isUser ? "User" : "Claude"}
      </div>
      <div
        style={{
          fontSize: ".9rem",
          lineHeight: 1.6,
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
          padding: isUser ? ".65rem .8rem" : 0,
          background: isUser ? "var(--ifm-color-emphasis-100)" : "transparent",
          borderRadius: isUser ? 6 : 0,
          minWidth: 0,
        }}
      >
        {children}
      </div>
    </div>
  );
}

// EventView is one step: a message Claude wrote, or a tool it called.
function EventView({ e }: { e: SessionEvent }): ReactElement {
  const [open, setOpen] = useState(false);
  if (e.kind === "text") return <Turn role="assistant">{e.text}</Turn>;

  const out = e.output ?? "";
  const lines = out ? out.split("\n") : [];
  const long = lines.length > 6 || out.length > 500;
  const preview = long && !open ? lines.slice(0, 6).join("\n") : out;

  return (
    <div
      style={{
        margin: "0 0 .9rem",
        paddingLeft: ".8rem",
        borderLeft: `2px solid ${e.failed ? "var(--ifm-color-danger)" : "var(--ifm-color-emphasis-300)"}`,
        minWidth: 0,
      }}
    >
      <div
        style={{
          fontFamily: mono,
          fontSize: ".74rem",
          fontWeight: 700,
          letterSpacing: ".04em",
          marginBottom: ".25rem",
          color: e.failed ? "var(--ifm-color-danger)" : "var(--ifm-color-emphasis-700)",
        }}
      >
        {e.name ?? "tool"}
        {e.failed ? " · returned an error" : ""}
      </div>
      {e.input && <ToolInput name={e.name ?? ""} input={e.input} />}
      {out && (
        <pre style={{ ...s.pre, marginTop: ".3rem", maxHeight: open ? "none" : "11rem" }}>
          {preview}
        </pre>
      )}
      {long && (
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          style={{
            font: "inherit",
            fontSize: ".75rem",
            padding: ".1rem 0",
            border: "none",
            background: "none",
            color: "var(--ifm-color-primary)",
            cursor: "pointer",
          }}
        >
          {open ? "show less" : `show all ${lines.length} lines`}
        </button>
      )}
    </div>
  );
}

// ToolInput renders a call the way the tool itself would read, rather than as
// the JSON the agent sent.
function ToolInput({ name, input }: { name: string; input: string }): ReactElement {
  let parsed: Record<string, unknown> | null = null;
  try {
    const v: unknown = JSON.parse(input);
    if (v && typeof v === "object") parsed = v as Record<string, unknown>;
  } catch {
    parsed = null;
  }
  const str = (k: string): string => (typeof parsed?.[k] === "string" ? (parsed[k] as string) : "");
  const cmd = str("command");
  if (name === "Bash" && cmd) return <div style={s.cmd}>$ {cmd}</div>;
  const path = str("file_path") || str("path");
  if (path)
    return (
      <div style={s.cmd}>
        {name} {path}
      </div>
    );
  const pattern = str("pattern") || str("query");
  if (pattern)
    return (
      <div style={s.cmd}>
        {name} {pattern}
      </div>
    );
  return <div style={{ ...s.cmd, whiteSpace: "pre-wrap" }}>{input}</div>;
}
