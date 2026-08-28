import { useEffect, useState } from "react";
import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import CodeBlock from "@theme/CodeBlock";
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
  // Every way this run reached for kapi: a shell invocation verbatim, or
  // Skill(kapi) where it loaded the guidance without running anything. Only the
  // pulled arm can have any, and empty there is the arm's result.
  kapiCommands?: string[];
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
  pulled?: AgentRun;
  bareFile: string;
  governedFile: string;
  pulledFile?: string;
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

// Through `unknown`, which is what TypeScript asks for here. It infers a literal
// type per document, and with three arms the union is wide enough that it stops
// comparing and reports the two types as non-overlapping (TS2352). The shape is
// checked where it is generated: scripts/authoringlab writes this file from Go
// structs whose json tags are these field names.
const report = data as unknown as Report;
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
  // The panel is chrome: what the run cost, what it read, which arm this is.
  // Dimmer than the document deliberately — the two used to sit on the same
  // ground with only a rule between them, and read as one block.
  panel: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    background: "var(--ifm-color-emphasis-100)",
    padding: "1rem 1.1rem",
    minWidth: 0,
  },
  doc: {
    fontSize: ".88rem",
    lineHeight: 1.6,
    maxHeight: "40rem",
    overflow: "auto",
    minWidth: 0,
    // The model's writing, on its own warm ground.
    //
    // Everything else on this page is chrome the harness produced: the task, the
    // guide, the file counts, the cost. This is the one block a model wrote, and
    // a reader scrolling three columns should be able to find it without reading
    // a heading. Infima's warning pair, because it is the one tinted surface
    // that ships with a matching foreground for both themes — cream on dark ink
    // in light, deep amber under cream in dark — so the tint never costs
    // legibility. The colour carries no severity here; it is a highlighter.
    background: "var(--ifm-color-warning-contrast-background)",
    color: "var(--ifm-color-warning-contrast-foreground)",
    border: "1px solid var(--ifm-color-emphasis-200)",
    // Warm to match, where the browser can mix it; the grey above is the
    // fallback where it cannot.
    borderColor: "color-mix(in srgb, var(--ifm-color-warning) 35%, transparent)",
    borderRadius: 6,
    padding: ".9rem 1.1rem",
    // Rendered Markdown, so the document reads as a document. Headings inside
    // it are content, not page structure, so they are scaled down to sit under
    // the panel's own heading.
    ["--ifm-h1-font-size" as string]: "1.25rem",
    ["--ifm-h2-font-size" as string]: "1.1rem",
    ["--ifm-h3-font-size" as string]: "1rem",
    // Headings and links inside the document take the document's colour rather
    // than the page's, or they land as the one unreadable element on the tint.
    ["--ifm-heading-color" as string]: "var(--ifm-color-warning-contrast-foreground)",
    // Code spans tinted from the document's own ink instead of the page's grey,
    // which on the amber ground of the dark theme reads as a smudge.
    ["--ifm-code-background" as string]:
      "color-mix(in srgb, var(--ifm-color-warning-contrast-foreground) 12%, transparent)",
  },
  guide: {
    fontSize: ".85rem",
    lineHeight: 1.55,
    maxHeight: "22rem",
    overflow: "auto",
    minWidth: 0,
    background: "var(--ifm-background-surface-color)",
    border: "1px solid var(--ifm-color-emphasis-200)",
    borderRadius: 6,
    padding: ".8rem 1rem",
    ["--ifm-h1-font-size" as string]: "1.1rem",
    ["--ifm-h2-font-size" as string]: "1rem",
    ["--ifm-h3-font-size" as string]: ".95rem",
    ["--ifm-paragraph-margin-bottom" as string]: ".5rem",
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
    background: "var(--ifm-background-surface-color)",
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

// pullTally counts the arm's result across the whole sweep.
//
// Computed here rather than stored in the dataset: it is a count of a field the
// page already has, and a stored copy is a second answer that can disagree with
// the documents beside it.
function pullTally(): { runs: number; asked: number; commands: string[] } {
  const commands = new Set<string>();
  let runs = 0;
  let asked = 0;
  for (const d of report.docs) {
    if (!d.pulled || (!d.pulled.text && !d.pulled.error)) continue;
    runs += 1;
    const calls = d.pulled.kapiCommands ?? [];
    if (calls.length > 0) asked += 1;
    for (const c of calls) commands.add(c);
  }
  return { runs, asked, commands: [...commands].sort() };
}

// PullResult states the third arm's result once, at the top.
//
// A reader should not have to open eight cells to find out whether the loop
// closed. Stated as a count with the commands beside it, so "it asked" cannot be
// read as "it asked the right thing".
function PullResult(): ReactElement | null {
  const { runs, asked, commands } = pullTally();
  if (runs === 0) return null;
  const none = asked === 0;
  return (
    <div
      style={{
        ...s.panel,
        margin: "1rem 0 0",
        // Emphasis rather than the warning ramp: the tint is spoken for. A
        // document on this page is yellow because a model wrote it, and a second
        // yellow meaning "look here" would make the first one ambiguous.
        borderColor: none ? "var(--ifm-color-emphasis-500)" : "var(--ifm-color-emphasis-300)",
      }}
    >
      <div style={s.h}>The pulled arm</div>
      {none ? (
        <p style={{ margin: 0, fontSize: ".9rem", lineHeight: 1.55 }}>
          <strong>None of the {runs} runs asked for the context they were standing in.</strong>{" "}
          Every one had the kapi skill installed and a project at the root binding a voice profile,
          and every one read the source and wrote the document without loading either. The pulled
          documents are therefore the bare arm&apos;s, with an unopened workspace beside them. This
          arm asks whether an assistant reaches for the guidance on its own, given a task that says
          only what to write. On these runs, it did not.
        </p>
      ) : (
        <p style={{ margin: 0, fontSize: ".9rem", lineHeight: 1.55 }}>
          <strong>
            {asked} of {runs} runs fetched their own context.
          </strong>{" "}
          What they ran is below, per cell. A run that reached for kapi and one that reached for the
          right part of it are different results, so the commands are shown rather than counted.
        </p>
      )}
      {commands.length > 0 && (
        <ul
          style={{
            margin: ".6rem 0 0",
            paddingLeft: "1.1rem",
            fontFamily: mono,
            fontSize: ".78rem",
          }}
        >
          {commands.map((c) => (
            <li key={c}>{c}</li>
          ))}
        </ul>
      )}
      {none && (
        <p style={{ ...s.sub, margin: ".7rem 0 0", lineHeight: 1.55 }}>
          A follow-up says what was missing. Adding a three-sentence <code>CLAUDE.md</code> — the
          project&apos;s voice is held by kapi, retrieve it before writing — flipped both models it
          was tried on. Claude Sonnet 5 then ran the whole loop unprompted: loaded the skill, asked{" "}
          <code>kapi context GUIDE.md</code> what applied, fetched the guide, wrote, and checked its
          own draft with <code>kapi voice check</code>. Two runs is enough to say the signpost was
          missing and not enough to say how often, so it is{" "}
          <a href="https://github.com/neokapi/neokapi/issues/2250">an issue</a> rather than a fourth
          column.
        </p>
      )}
    </div>
  );
}

// Asked reports whether the arm that had to fetch its own context fetched it.
//
// The result of this arm is not the prose alone. A document no better than the
// bare one means one of two things — the guide did not help, or the agent never
// got it — and they have opposite fixes. This says which happened before a
// reader starts comparing paragraphs.
function Asked({ run }: { run: AgentRun }): ReactElement {
  const calls = run.kapiCommands ?? [];
  if (calls.length === 0) {
    return (
      <p style={{ ...s.sub, margin: ".5rem 0 0" }}>
        {/* Uncoloured on purpose. The warning ramp's darker end is a fixed
            value, so it would be a legible amber on the light theme and a
            near-black smudge on the dark one — and the tint is already spoken
            for by the document below. */}
        <strong>It never asked.</strong> The skill was installed and the project bound a voice; the
        agent wrote the document without loading either, so its prose is the bare arm&apos;s with a
        workspace it did not use.
      </p>
    );
  }
  return (
    <div style={{ marginTop: ".5rem" }}>
      <div style={s.h}>What it asked kapi</div>
      <ul style={{ margin: 0, paddingLeft: "1.1rem", fontFamily: mono, fontSize: ".78rem" }}>
        {calls.map((c) => (
          <li key={c}>{c}</li>
        ))}
      </ul>
    </div>
  );
}

function Arm({
  label,
  how,
  mustAsk,
  run,
  file,
  onOpenSession,
}: {
  label: string;
  // How the governance reached this run. The arms differ in nothing else, so a
  // reader who cannot see this cannot read the comparison.
  how: string;
  // mustAsk marks the arm that was given the skill and a project instead of a
  // prompt. Only there is "it ran no kapi command" a result rather than the
  // ordinary state of every other arm.
  mustAsk?: boolean;
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
          marginBottom: ".25rem",
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
      <p style={{ ...s.sub, margin: "0 0 .7rem" }}>{how}</p>

      {/* What went in. Boxed and labelled, because it used to run straight into
          the document below it and a reader could not tell where the inputs
          ended and the model's writing began. */}
      <div style={{ marginBottom: ".9rem" }}>
        <div style={s.h}>What it read</div>
        <Reading run={run} />
        {mustAsk && <Asked run={run} />}
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
          document one part of it. It reads whatever it needs. The same task runs three ways, and
          the three differ only in how the governance arrives: not at all, pushed into the system
          prompt, or waiting in the workspace for an agent that has to ask for it. Two coordinates,
          four models.
        </p>
        <p style={s.lede}>
          The third arm is there because the first two cannot answer the question the product asks.
          Pushing the guide in measures whether that context changes the writing. It says nothing
          about whether an assistant plugged into kapi ends up holding the context at all, and a
          document that comes back no better than bare has two explanations with opposite fixes. The
          pulled arm gets the kapi skill and a project that binds the voice, and each run records
          what it asked kapi for — or that it never did.
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

        <PullResult />

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
          <div style={s.h}>The task, given to every arm</div>
          {/* The task names four flags in backticks, so it is Markdown too, and
              showing it as plain text put the backticks on the page. */}
          <div style={{ margin: "0 0 .9rem", fontSize: ".88rem", lineHeight: 1.55 }}>
            <Markdown remarkPlugins={[remarkGfm]}>{report.tasks[audience]}</Markdown>
          </div>
          <div style={s.h}>The guide the two governed arms are held to</div>
          {/* The guide is Markdown that `kapi voice guide` renders, so showing
              it as monospace source shows the reader something the model never
              saw as source either. */}
          <div style={s.guide}>
            <Markdown remarkPlugins={[remarkGfm]}>{report.guides[audience]}</Markdown>
          </div>
          <p style={{ ...s.sub, margin: ".5rem 0 0" }}>
            Rendered by <code>kapi voice guide</code> from ripgrep&apos;s own voice profile,
            resolved at this coordinate. The pushed arm is handed this text; the pulled arm&apos;s
            workspace answers with it, checked byte for byte before the sweep runs so the two arms
            differ in delivery and not in what they were governed by. The profile itself is at the
            bottom of the page.
          </p>
        </div>

        {!doc && <p>No document for this cell.</p>}
        {doc && (
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fit, minmax(24rem, 1fr))",
              gap: "1rem",
            }}
          >
            <Arm
              label="Bare"
              how="The repository and the task. No governance of any kind."
              run={doc.bare}
              file={doc.bareFile}
              onOpenSession={() => setSession(doc.bare.transcript ?? null)}
            />
            <Arm
              label="Pushed"
              how="The same, plus the guide appended to its system prompt before it started."
              run={doc.governed}
              file={doc.governedFile}
              onOpenSession={() => setSession(doc.governed.transcript ?? null)}
            />
            {doc.pulled && (
              <Arm
                label="Pulled"
                how="The same, plus the kapi skill and a project that binds the voice. Nothing in its prompt: it has to go and get the guide itself."
                mustAsk
                run={doc.pulled}
                file={doc.pulledFile ?? ""}
                onOpenSession={() => setSession(doc.pulled?.transcript ?? null)}
              />
            )}
          </div>
        )}

        {session && <SessionModal file={session} onClose={() => setSession(null)} />}

        <details style={{ marginTop: "1.6rem" }}>
          <summary style={{ cursor: "pointer", fontWeight: 600 }}>
            ripgrep&apos;s voice profile, inferred by kapi from its own docs and corrected by hand
          </summary>
          <div style={{ marginTop: ".6rem", maxHeight: "34rem", overflow: "auto" }}>
            <CodeBlock language="yaml">{report.profile}</CodeBlock>
          </div>
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
function Turn({ role, children }: { role: "user" | "assistant"; children: string }): ReactElement {
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
      {/* Claude writes Markdown, so a turn that shows it raw shows `**bold**`
          and `- item` as literal characters. The prompt is rendered as written
          because it IS the plain text that was sent. */}
      <div
        style={{
          fontSize: ".9rem",
          lineHeight: 1.6,
          whiteSpace: isUser ? "pre-wrap" : "normal",
          wordBreak: "break-word",
          padding: isUser ? ".65rem .8rem" : 0,
          background: isUser ? "var(--ifm-color-emphasis-100)" : "transparent",
          borderRadius: isUser ? 6 : 0,
          minWidth: 0,
          ["--ifm-h1-font-size" as string]: "1.15rem",
          ["--ifm-h2-font-size" as string]: "1.05rem",
          ["--ifm-h3-font-size" as string]: "1rem",
          ["--ifm-paragraph-margin-bottom" as string]: ".6rem",
        }}
      >
        {isUser ? children : <Markdown remarkPlugins={[remarkGfm]}>{children}</Markdown>}
      </div>
    </div>
  );
}

// EventView is one step: a message Claude wrote, or a tool it called.
function EventView({ e }: { e: SessionEvent }): ReactElement {
  const [open, setOpen] = useState(false);
  if (e.kind === "text") return <Turn role="assistant">{e.text ?? ""}</Turn>;

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
