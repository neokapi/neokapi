import { useEffect, useMemo, useState } from "react";
import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";
import data from "./_skilleval.json";

// The skill-eval dashboard: does the shipped Agent Skill fire on the tasks it
// should, stay quiet on the ones it should not, and finish the job.
//
// Built to be drilled rather than read. The summary is four numbers; everything
// below is the evidence, one row per scenario, expanding into the exact prompt,
// the workspace the agent saw, the kapi commands it ran, and a diff of what it
// changed. A verdict nobody can inspect is an opinion.
//
// Regenerate with `make skill-eval` (triggering) or `make skill-eval-completion`.

interface FixtureFile {
  as: string;
  from?: string;
  note?: string;
  bytes?: number;
}
interface FileChange {
  path: string;
  kind: "added" | "edited" | "removed";
  before?: string;
  after?: string;
  binary?: boolean;
  bytesAfter?: number;
  // Where the file itself was published, for the changes this page cannot
  // show. A .docx diff is meaningless as text, so the alternative to a link is
  // a byte count.
  artifact?: string;
  // Why it was not published, when it was not.
  skipped?: string;
}
interface GateResult {
  command: string;
  exitCode: number;
  output?: string;
}
interface Run {
  triggered: boolean;
  tools?: string[];
  kapiCommands?: string[];
  mcpTools?: string[];
  finalText?: string;
  messages: number;
  durationMs: number;
  gate?: GateResult;
  changed?: FileChange[];
  error?: string;
}
// One step of a recorded session: an assistant message, or a tool call with
// what it returned.
interface SessionEvent {
  kind: "text" | "tool";
  name?: string;
  text?: string;
  input?: string;
  output?: string;
  failed?: boolean;
}
interface Session {
  events: SessionEvent[];
}
// The sidecar, fetched when a reader opens a scenario. Sessions are large and
// the dataset above is imported into the bundle, so they are not in it.
interface SessionFile {
  key: string;
  scenario: string;
  prompt: string;
  runs: Session[];
  unaided?: Session[];
}
interface Scenario {
  id: string;
  kind: "positive" | "negative";
  surface?: "skill" | "mcp";
  expectTool?: string;
  prompt: string;
  path: string;
  why: string;
  fixture: FixtureFile[];
  turns: number;
  completionGate?: string;
  knownLimit?: string;
}
interface Result {
  scenario: Scenario;
  runs: Run[];
  fired: number;
  verdict: "pass" | "fail" | "flaky" | "no gate" | "not run";
  gatePassed?: number;
  foundTool?: number;
  wrongTool?: string[];
  // The control arm: the same prompt and workspace with no skill, no MCP
  // server, and no kapi anywhere on PATH.
  unaided?: Run[];
  unaidedGatePassed?: number;
  contribution?: "enabled" | "eased" | "hindered" | "neither" | "unknown";
  // The file holding this scenario's sessions. Absent on a dataset generated
  // before they were kept, so everything below treats it as optional.
  transcript?: string;
}
interface Runner {
  claudeVersion: string;
  model: string;
  skillCommit: string;
  skillModified: string;
  host: string;
  settings: string;
  kapi?: string;
  kapiVersion?: string;
}
interface Report {
  generated: string;
  mode: string;
  surface?: string;
  repeat: number;
  runner: Runner;
  summary: {
    scenarios: number;
    pass: number;
    flaky: number;
    fail: number;
    positives: number;
    negatives: number;
    falseTriggers: number;
    gatesRun?: number;
    gatesPassed?: number;
    wrongToolPicks?: number;
    ungated?: number;
    contributions?: Record<string, number>;
  };
  results: Result[];
}

const reports = data as Record<string, Report>;

// ControlArm reports what kapi added, measured rather than asserted.
//
// Every scenario runs a second time with no skill, no MCP server, and no kapi
// anywhere on PATH. The comparison exists because a scenario note claimed of a
// .pptx that "the agent has no other way to read it", and an unaided agent
// answered correctly in three calls with unzip. A .pptx is a zip of XML. The
// suite was full of assertions like that, and only the run settles them.
function ControlArm({
  sum,
  results,
}: {
  sum: Report["summary"];
  results: Result[];
}): ReactElement | null {
  const n = sum.contributions;
  if (!n) return null;
  const total = CONTRIBUTIONS.reduce((a, c) => a + (n[c.key] ?? 0), 0);
  if (total === 0) return null;
  return (
    <section
      style={{
        border: "1px solid var(--ifm-color-emphasis-300)",
        borderRadius: 8,
        padding: "1.1rem 1.3rem",
        margin: "1.4rem 0",
        background: "var(--ifm-background-surface-color)",
      }}
    >
      <h2 style={{ marginTop: 0, fontSize: "1.2rem" }}>What kapi added</h2>
      <p style={{ maxWidth: "72ch", fontSize: ".92rem", color: "var(--ifm-color-emphasis-800)" }}>
        Each scenario ran a second time with no skill, no MCP server, and no kapi on PATH. The
        comparison is deliberately conservative: kapi failing is never counted as a win, the message
        counts are medians, and a scenario with no gate is unknown rather than assumed.
      </p>
      <MessageCost results={results} />
      <div style={{ display: "flex", flexWrap: "wrap", gap: "1.4rem", marginTop: ".8rem" }}>
        {CONTRIBUTIONS.map((c) => (
          <div key={c.key} style={{ minWidth: "13rem" }}>
            <div
              style={{
                fontFamily: "var(--ifm-font-family-monospace)",
                fontSize: "1.7rem",
                fontWeight: 700,
                fontVariantNumeric: "tabular-nums",
                color: tone[c.t].fg,
                lineHeight: 1.1,
              }}
            >
              {n[c.key] ?? 0}
            </div>
            <div style={{ fontWeight: 600, fontSize: ".9rem" }}>
              {contributionLabel[c.key] ?? c.label}
            </div>
            <div style={{ fontSize: ".8rem", color: "var(--ifm-color-emphasis-700)" }}>
              {c.means}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

// What the control arm answers, in the order a reader should take it.
//
// "neither" first would be defensive and "enabled" first would be a pitch, so
// the order is the strength of the claim, and the neither count is given the
// same weight as the rest rather than tucked at the end. It is the number that
// says how much of this suite is not evidence for kapi.
const CONTRIBUTIONS: { key: string; label: string; means: string; t: keyof typeof tone }[] = [
  {
    key: "enabled",
    label: "enabled",
    means: "the unaided agent could not finish and the one with kapi did",
    t: "pass",
  },
  {
    key: "eased",
    label: "eased",
    means: "both finished, and kapi took materially fewer messages",
    t: "pass",
  },
  {
    key: "hindered",
    label: "hindered",
    means: "the unaided agent finished and the one with kapi did not",
    t: "fail",
  },
  {
    key: "neither",
    label: "neither",
    means: "both finished, and kapi saved nothing measurable",
    t: "flat",
  },
  {
    key: "unknown",
    label: "unknown",
    means: "no gate, so there is no outcome to compare",
    t: "flat",
  },
];

// The dataset holds one report per surface and mode. The keys are exact and
// unlovely, so the tabs get names a reader can act on.
const reportLabel: Record<string, string> = {
  "skill:trigger": "Skill — does it fire",
  "skill:completion": "Skill — does it finish",
  "mcp:trigger": "MCP — right tool",
};

// The two surfaces ask different questions of the same product, and the summary
// number differs with them. A skill can fail to be noticed; an MCP server is
// already in context and fails by being picked wrongly.
const surfaceNote: Record<string, string> = {
  skill:
    "The agent has to notice kapi is relevant. The only lever is the skill's description, and a false trigger costs more than a miss: missing a positive wastes one kapi user's prompt, firing on a code task wastes everyone's.",
  mcp: "The agent already holds all nineteen tools, so reaching kapi is nearly free and says almost nothing. What is scored is whether it reached the tool the task called for. A wrong pick names two descriptions that are not telling each other apart.",
};
const mono = "var(--ifm-font-family-monospace)";

// Infima's `-contrast-foreground` tokens invert with the theme; the `-darker`
// ones do not, and using them puts mid-red on dark red for half the readers.
const tone = {
  pass: {
    fg: "var(--ifm-color-success-contrast-foreground)",
    bg: "var(--ifm-color-success-contrast-background)",
  },
  flaky: {
    fg: "var(--ifm-color-warning-contrast-foreground)",
    bg: "var(--ifm-color-warning-contrast-background)",
  },
  fail: {
    fg: "var(--ifm-color-danger-contrast-foreground)",
    bg: "var(--ifm-color-danger-contrast-background)",
  },
  flat: { fg: "var(--ifm-color-emphasis-800)", bg: "var(--ifm-color-emphasis-200)" },
} as const;

type Tone = keyof typeof tone;

const s: Record<string, CSSProperties> = {
  strip: {
    display: "flex",
    flexWrap: "wrap",
    gap: "1.6rem",
    padding: "1.1rem 1.3rem",
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    background: "var(--ifm-background-surface-color)",
    margin: "1.4rem 0",
  },
  stat: { display: "flex", flexDirection: "column", gap: ".15rem" },
  statN: { fontSize: "1.6rem", fontWeight: 650, lineHeight: 1, fontVariantNumeric: "tabular-nums" },
  statL: {
    fontSize: ".7rem",
    letterSpacing: ".1em",
    textTransform: "uppercase",
    color: "var(--ifm-color-emphasis-700)",
  },
  meta: {
    display: "grid",
    gridTemplateColumns: "auto 1fr",
    gap: ".3rem 1rem",
    fontSize: ".84rem",
    fontFamily: mono,
    padding: "0 .2rem 1.2rem",
  },
  metaK: { color: "var(--ifm-color-emphasis-700)" },
  row: {
    display: "grid",
    // The first column holds a verdict and, on a gated sweep, a contribution
    // beside it. A fixed 5.5rem fitted one pill and overlapped the prompt with
    // the second. max-content sizes to whatever is actually there, and the
    // minmax(0,1fr) lets a long prompt wrap instead of widening the grid.
    gridTemplateColumns: "max-content minmax(0, 1fr) max-content",
    gap: ".8rem",
    alignItems: "center",
    width: "100%",
    textAlign: "left",
    padding: ".6rem .8rem",
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 6,
    background: "var(--ifm-background-surface-color)",
    cursor: "pointer",
    font: "inherit",
    color: "inherit",
  },
  prompt: { fontSize: ".92rem", lineHeight: 1.35 },
  sub: { fontSize: ".75rem", color: "var(--ifm-color-emphasis-700)", fontFamily: mono },
  pill: {
    fontFamily: mono,
    fontSize: ".67rem",
    letterSpacing: ".07em",
    textTransform: "uppercase",
    padding: ".18rem .5rem",
    borderRadius: 4,
    whiteSpace: "nowrap",
  },
  detail: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderTop: "none",
    borderRadius: "0 0 6px 6px",
    padding: "1rem 1.1rem",
    background: "var(--ifm-background-color)",
    display: "grid",
    gap: "1.1rem",
  },
  h: {
    fontSize: ".7rem",
    letterSpacing: ".1em",
    textTransform: "uppercase",
    color: "var(--ifm-color-emphasis-700)",
    marginBottom: ".4rem",
  },
  pre: {
    fontFamily: mono,
    fontSize: ".8rem",
    lineHeight: 1.5,
    margin: 0,
    padding: ".6rem .75rem",
    background: "var(--ifm-color-emphasis-100)",
    borderRadius: 5,
    overflowX: "auto",
    whiteSpace: "pre-wrap",
    wordBreak: "break-word",
  },
  cmd: {
    fontFamily: mono,
    fontSize: ".8rem",
    padding: ".35rem .6rem",
    background: "var(--ifm-color-emphasis-100)",
    borderRadius: 4,
    borderLeft: "3px solid var(--ifm-color-primary)",
    overflowX: "auto",
    whiteSpace: "pre",
  },
  table: { width: "100%", fontSize: ".84rem", borderCollapse: "collapse" },
};

function Pill({ text, t, title }: { text: string; t: Tone; title?: string }): ReactElement {
  return (
    <span title={title} style={{ ...s.pill, color: tone[t].fg, background: tone[t].bg }}>
      {text}
    </span>
  );
}

// What a contribution verdict means, on the verdict itself.
//
// The words are the runner's and two of them do not survive being read alone:
// "neither" on a row says nothing without the panel at the top of the page that
// defines it, and a reader meets the row first. The label is what it means; the
// key stays what the dataset says.
const contributionLabel: Record<string, string> = {
  enabled: "kapi enabled it",
  eased: "kapi eased it",
  hindered: "kapi hindered it",
  neither: "no difference",
  unknown: "not comparable",
};

function contributionMeans(key: string): string {
  return CONTRIBUTIONS.find((c) => c.key === key)?.means ?? "";
}

function contributionTone(key: string): Tone {
  if (key === "enabled" || key === "eased") return "pass";
  if (key === "hindered") return "fail";
  return "flat";
}

// Every verdict the runner can emit needs an entry. A Record over the union
// makes that a compile error rather than a crash: "no gate" was added to the
// Go side and this map was not, so the completion tab threw on its first
// ungated row and the whole page fell back to the error boundary.
const verdictTone: Record<Result["verdict"], Tone> = {
  pass: "pass",
  flaky: "flaky",
  fail: "fail",
  // Not a pass and not a failure. Nothing about the scenario was verified,
  // which is its own thing and reads as neutral.
  "no gate": "flat",
  "not run": "flat",
};

/** A minimal line diff, enough to see what an agent changed and not more. */
// A change is capped at 24KB by the harness, which is still around 500 lines.
// Rendering all of them inline pushes everything below a single diff off the
// screen, so the block scrolls in place.
const DIFF_MAX_HEIGHT = "22rem";

function Diff({ before, after }: { before: string; after: string }): ReactElement {
  const a = before.split("\n");
  const b = after.split("\n");
  const removed = new Set(a.filter((l) => !b.includes(l)));
  const added = new Set(b.filter((l) => !a.includes(l)));

  const lines: { text: string; mark: "+" | "-" | " " }[] = [];
  for (const l of a) if (removed.has(l)) lines.push({ text: l, mark: "-" });
  for (const l of b) lines.push({ text: l, mark: added.has(l) ? "+" : " " });

  const changed = lines.filter((l) => l.mark !== " ").length;

  return (
    <pre style={{ ...s.pre, maxHeight: DIFF_MAX_HEIGHT, overflowY: "auto" }}>
      {changed === 0 && (
        <div style={{ color: "var(--ifm-color-emphasis-700)" }}>
          (no line changed; the file was rewritten with the same content)
        </div>
      )}
      {lines.map((l, i) => (
        <div
          key={i}
          style={{
            color: l.mark === "+" ? tone.pass.fg : l.mark === "-" ? tone.fail.fg : undefined,
            background: l.mark === "+" ? tone.pass.bg : l.mark === "-" ? tone.fail.bg : undefined,
            padding: "0 .25rem",
          }}
        >
          {l.mark} {l.text}
        </div>
      ))}
    </pre>
  );
}

function Detail({ r }: { r: Result }): ReactElement {
  const sc = r.scenario;
  const runs = r.runs ?? [];
  const sessions = useSessions(r);
  const [open, setOpen] = useState<OpenSession | null>(null);
  const show = (arm: "runs" | "unaided", index: number): void => {
    sessions.load();
    setOpen({ arm, index });
  };
  return (
    <div style={s.detail}>
      <div>
        <div style={s.h}>Prompt, verbatim</div>
        <pre style={s.pre}>{sc.prompt}</pre>
      </div>

      {sc.expectTool && (
        <div>
          <div style={s.h}>The right answer</div>
          <div style={{ display: "flex", gap: ".5rem", alignItems: "center" }}>
            <Pill text={sc.expectTool} t="pass" />
            <span style={s.sub}>one of nineteen tools the server advertises</span>
          </div>
        </div>
      )}

      <div>
        <div style={s.h}>Why this scenario exists</div>
        <p style={{ margin: 0, fontSize: ".88rem", lineHeight: 1.55 }}>{sc.why}</p>
      </div>

      {sc.fixture.length > 0 && (
        <div>
          <div style={s.h}>Workspace the agent saw</div>
          <table style={s.table}>
            <tbody>
              {sc.fixture.map((f) => (
                <tr key={f.as} style={{ borderTop: "1px solid var(--ifm-color-emphasis-200)" }}>
                  <td
                    style={{
                      fontFamily: mono,
                      padding: ".3rem .6rem .3rem 0",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {f.as}
                  </td>
                  <td
                    style={{
                      padding: ".3rem .6rem",
                      textAlign: "right",
                      fontVariantNumeric: "tabular-nums",
                      color: "var(--ifm-color-emphasis-700)",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {f.bytes ? `${f.bytes.toLocaleString()} B` : ""}
                  </td>
                  <td style={{ padding: ".3rem 0", color: "var(--ifm-color-emphasis-700)" }}>
                    {f.note}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {sc.knownLimit && (
        <div>
          <div style={s.h}>Known limit</div>
          <p style={{ margin: 0, fontSize: ".88rem", color: tone.flaky.fg }}>{sc.knownLimit}</p>
        </div>
      )}

      <div>
        <div style={s.h}>
          {runs.length} pass{runs.length === 1 ? "" : "es"}
        </div>
        <div style={{ display: "grid", gap: ".9rem" }}>
          {runs.map((run, i) => (
            <div
              key={i}
              style={{
                borderLeft: `3px solid ${run.triggered ? tone.pass.fg : tone.flat.fg}`,
                paddingLeft: ".8rem",
                display: "grid",
                gap: ".5rem",
              }}
            >
              {/* One line per pass. The commands it ran and the message it
                  closed on used to sit here, three passes deep, which made a
                  scenario a wall of prose before a reader reached the gate.
                  They are turns of the session, and that is where they are. */}
              <div
                style={{ display: "flex", gap: ".5rem", alignItems: "center", flexWrap: "wrap" }}
              >
                <span style={{ ...s.sub, fontWeight: 600 }}>pass {i + 1}</span>
                <Pill
                  text={run.triggered ? "fired" : "silent"}
                  t={run.triggered === (sc.kind === "positive") ? "pass" : "fail"}
                />
                <span style={s.sub}>
                  {run.messages} msg · {(run.durationMs / 1000).toFixed(1)}s
                  {(run.tools ?? []).length > 0 && ` · ${run.tools!.join(", ")}`}
                </span>
              </div>

              {run.error && <pre style={{ ...s.pre, color: tone.fail.fg }}>{run.error}</pre>}

              {(run.mcpTools ?? []).length > 0 && (
                <div>
                  <div style={s.h}>Tools it reached for</div>
                  <div style={{ display: "flex", gap: ".35rem", flexWrap: "wrap" }}>
                    {run.mcpTools!.map((t) => (
                      <Pill
                        key={t}
                        text={t}
                        t={sc.expectTool ? (t === sc.expectTool ? "pass" : "fail") : "fail"}
                      />
                    ))}
                  </div>
                </div>
              )}

              {run.gate && (
                <div>
                  <div style={s.h}>Completion gate</div>
                  <div
                    style={{
                      display: "flex",
                      gap: ".6rem",
                      alignItems: "center",
                      marginBottom: ".3rem",
                    }}
                  >
                    <Pill
                      text={run.gate.exitCode === 0 ? "green" : "red"}
                      t={run.gate.exitCode === 0 ? "pass" : "fail"}
                    />
                    <code style={{ fontFamily: mono, fontSize: ".8rem" }}>{run.gate.command}</code>
                  </div>
                  {run.gate.output && <pre style={s.pre}>{run.gate.output}</pre>}
                </div>
              )}

              {(run.changed ?? []).length > 0 && (
                <div>
                  <div style={s.h}>What it changed</div>
                  <div style={{ display: "grid", gap: ".6rem" }}>
                    {run.changed!.map((c) => (
                      <div key={c.path}>
                        <div
                          style={{
                            display: "flex",
                            gap: ".5rem",
                            alignItems: "center",
                            marginBottom: ".2rem",
                          }}
                        >
                          <Pill text={c.kind} t={c.kind === "removed" ? "fail" : "pass"} />
                          <code style={{ fontFamily: mono, fontSize: ".8rem" }}>{c.path}</code>
                          {c.binary && <ArtifactLink c={c} />}
                        </div>
                        {!c.binary && c.before !== undefined && c.after !== undefined && (
                          <Diff before={c.before} after={c.after} />
                        )}
                        {!c.binary && c.before === undefined && c.after !== undefined && (
                          <Diff before="" after={c.after} />
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <SessionButton sessions={sessions} onOpen={() => show("runs", i)} />
            </div>
          ))}
        </div>
      </div>

      <UnaidedRuns r={r} sessions={sessions} onOpen={(i) => show("unaided", i)} />

      {open && <SessionModal r={r} sessions={sessions} open={open} onClose={() => setOpen(null)} />}
    </div>
  );
}

// The recorded session belongs to the pass it came from.
//
// It was one block at the bottom of the scenario listing every pass in turn,
// which put pass 3's conversation four screens below pass 3's tool list and its
// diff. The transcript is evidence about one run, so it sits with that run, and
// the control arm's passes get the same.
//
// One fetch per scenario, shared: the sidecar holds every pass, so loading it
// from any one of them fills in the rest.

interface Sessions {
  file: SessionFile | null;
  state: "idle" | "loading" | "failed";
  available: boolean;
  load: () => void;
}

function useSessions(r: Result): Sessions {
  // Same place the artefacts go. The transcripts were committed while they were
  // capped; uncapped they are far too large for git, so they follow the videos
  // to the CDN and resolve same-origin when none is configured.
  const { siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const local = useBaseUrl("/skill-eval/transcripts/");
  const dir = cdnEnabled(cdn) ? cdnHref(cdn, "/skill-eval/transcripts/") : local;
  const [file, setFile] = useState<SessionFile | null>(null);
  const [state, setState] = useState<"idle" | "loading" | "failed">("idle");

  const load = (): void => {
    if (!r.transcript || state === "loading" || file) return;
    setState("loading");
    fetch(dir + r.transcript)
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error(String(res.status)))))
      .then((body: SessionFile) => {
        setFile(body);
        setState("idle");
      })
      .catch(() => setState("failed"));
  };
  return { file, state, available: Boolean(r.transcript), load };
}

// OpenSession names the pass whose transcript is on screen.
interface OpenSession {
  arm: "runs" | "unaided";
  index: number;
}

// SessionButton opens the transcript for the pass it sits in.
//
// A button rather than the conversation itself: a completion run is up to a
// hundred events, and drawing that inline pushed the pass's own tool list, diff
// and gate result off the screen. The row stays scannable and the transcript
// gets a surface the size it needs.
function SessionButton({
  sessions,
  onOpen,
}: {
  sessions: Sessions;
  onOpen: () => void;
}): ReactElement | null {
  if (!sessions.available) return null;
  return (
    <button
      type="button"
      onClick={onOpen}
      style={{
        font: "inherit",
        fontSize: ".8rem",
        padding: ".25rem .7rem",
        borderRadius: 6,
        border: "1px solid var(--ifm-color-emphasis-300)",
        background: "var(--ifm-background-surface-color)",
        color: "var(--ifm-font-color-base)",
        cursor: "pointer",
        justifySelf: "start",
      }}
    >
      Full session
    </button>
  );
}

// SessionModal draws one pass's conversation over the page.
function SessionModal({
  r,
  sessions,
  open,
  onClose,
}: {
  r: Result;
  sessions: Sessions;
  open: OpenSession;
  onClose: () => void;
}): ReactElement {
  // Escape closes, and the page behind does not scroll while this is up.
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

  const sess = sessions.file?.[open.arm]?.[open.index];
  const arm = open.arm === "runs" ? "with kapi" : "control, no kapi";
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
        aria-label={`Session transcript: ${r.scenario.id}, ${arm}, pass ${open.index + 1}`}
        onClick={(e) => e.stopPropagation()}
        style={{
          // --ifm-background-color is `transparent` in Infima: it reads as the
          // page's own ground because the page is what is behind it. A panel
          // over a dimmed backdrop needs a real surface colour, or the page
          // shows through the transcript.
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
              {r.scenario.id} · {arm} · pass {open.index + 1}
            </div>
            <div style={{ ...s.sub, marginTop: ".15rem" }}>{r.scenario.prompt}</div>
          </div>
          <button
            type="button"
            onClick={onClose}
            autoFocus
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
          {sessions.state === "loading" && <p style={s.sub}>Loading the transcript…</p>}
          {sessions.state === "failed" && (
            <p style={{ ...s.sub, margin: 0 }}>
              The transcript could not be loaded. It is written by the run that produced this row,
              so a dataset copied without <code>static/skill-eval/transcripts/</code> will not have
              it.
            </p>
          )}
          {sess && <SessionView sess={sess} prompt={sessions.file?.prompt} />}
          {!sess && sessions.state === "idle" && sessions.file && (
            <p style={{ ...s.sub, margin: 0 }}>
              This pass has no recorded session. The transcript file covers the passes the run
              produced, and this row predates it.
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

// ArtifactLink offers the file itself.
//
// A .docx cannot be diffed as text, so this page used to say `binary, 41,205 B`
// and stop there. That is the weakest form of the claim: a byte count says a
// file exists, the gate says a check passed, and the document says what kapi
// actually wrote — the only one a reader can hold against their own idea of
// what faithful write-back means. The control arm's version of the same file is
// one row down.
//
// The files are too large for git (about 50MB a sweep, rewritten every run), so
// they go where the walkthrough videos go. With no CDN configured they resolve
// same-origin from web/static, which is where a local sweep stages them.
function ArtifactLink({ c }: { c: FileChange }): ReactElement {
  const { siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const local = useBaseUrl("/skill-eval/artifacts/");
  const size = c.bytesAfter ? `${(c.bytesAfter / 1024).toFixed(0)} KB` : "binary";

  if (!c.artifact) {
    return (
      <span style={s.sub}>
        {size}
        {c.skipped ? ` · not published: ${c.skipped}` : ""}
      </span>
    );
  }
  const href = cdnEnabled(cdn)
    ? cdnHref(cdn, `/skill-eval/artifacts/${c.artifact}`)
    : local + c.artifact;
  return (
    <>
      <span style={s.sub}>{size}</span>
      <a
        href={href}
        download
        style={{ fontSize: ".8rem", fontWeight: 600 }}
        title={`Download ${c.path} exactly as the run produced it`}
      >
        Download
      </a>
    </>
  );
}

// SessionView draws one conversation the way it happened.
//
// A session is a conversation, so it is laid out as one: the prompt as the turn
// that opened it, then each assistant message as prose, then each tool call
// with what it returned, in order. The earlier version was a flat list of
// bordered blocks that read as a log rather than as a session, and it truncated
// every tool result at 1,200 characters — which cut the part a reader opens a
// transcript for.
function SessionView({ sess, prompt }: { sess: Session; prompt?: string }): ReactElement {
  return (
    <div style={{ minWidth: 0 }}>
      {prompt && <Turn role="user">{prompt}</Turn>}
      <div style={{ display: "grid", gap: ".1rem", minWidth: 0 }}>
        {sess.events.map((e, i) => (
          <EventView key={i} e={e} />
        ))}
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

  if (e.kind === "text") {
    return <Turn role="assistant">{e.text ?? ""}</Turn>;
  }

  const out = e.output ?? "";
  const lines = out ? out.split("\n") : [];
  // A tool result is shown short and opened in full. Uncapped transcripts carry
  // whole files, and a reader scanning a forty-turn session wants the shape of
  // it before they want the bytes.
  const long = lines.length > 6 || out.length > 500;
  const preview = long && !open ? lines.slice(0, 6).join("\n") : out;

  return (
    <div
      style={{
        margin: "0 0 .9rem",
        paddingLeft: ".8rem",
        borderLeft: `2px solid ${e.failed ? tone.fail.fg : "var(--ifm-color-emphasis-300)"}`,
        minWidth: 0,
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: ".5rem",
          marginBottom: ".25rem",
          flexWrap: "wrap",
        }}
      >
        <span
          style={{
            fontFamily: mono,
            fontSize: ".74rem",
            fontWeight: 700,
            letterSpacing: ".04em",
            color: e.failed ? tone.fail.fg : "var(--ifm-color-emphasis-700)",
          }}
        >
          {e.name ?? "tool"}
        </span>
        {e.failed && <span style={{ ...s.sub, color: tone.fail.fg }}>returned an error</span>}
      </div>

      {e.input && <ToolInput name={e.name ?? ""} input={e.input} />}

      {out && (
        <pre
          style={{
            ...s.pre,
            marginTop: ".3rem",
            fontSize: ".76rem",
            background: "var(--ifm-color-emphasis-100)",
            maxHeight: open ? "none" : "11rem",
            overflow: "auto",
          }}
        >
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

// ToolInput renders a call the way the tool itself would read.
//
// The arguments arrive as the JSON the agent sent. For Bash that is a command,
// and a command shown as `{"command":"kapi status","description":"…"}` is
// harder to read than the command. The common shapes get their own line; the
// rest keep the JSON, which is at least exact.
function ToolInput({ name, input }: { name: string; input: string }): ReactElement {
  let parsed: Record<string, unknown> | null = null;
  try {
    const v: unknown = JSON.parse(input);
    if (v && typeof v === "object") parsed = v as Record<string, unknown>;
  } catch {
    parsed = null;
  }
  const str = (k: string): string => (typeof parsed?.[k] === "string" ? (parsed[k] as string) : "");

  const command = str("command");
  if (name === "Bash" && command) {
    return <div style={s.cmd}>$ {command}</div>;
  }
  const skill = str("skill");
  if (skill) {
    const args = str("args");
    return (
      <div style={s.cmd}>
        /{skill}
        {args && ` ${args}`}
      </div>
    );
  }
  const path = str("file_path") || str("path");
  if (path) {
    return (
      <div style={s.cmd}>
        {name} {path}
      </div>
    );
  }
  const pattern = str("pattern") || str("query");
  if (pattern) {
    return (
      <div style={s.cmd}>
        {name} {pattern}
      </div>
    );
  }
  return <div style={{ ...s.cmd, whiteSpace: "pre-wrap" }}>{input}</div>;
}

function UnaidedRuns({
  r,
  sessions,
  onOpen,
}: {
  r: Result;
  sessions: Sessions;
  onOpen: (index: number) => void;
}): ReactElement | null {
  const runs = r.unaided ?? [];
  if (runs.length === 0) return null;
  const withKapi = median((r.runs ?? []).map((x) => x.messages));
  const unaided = median(runs.map((x) => x.messages));
  return (
    <div>
      <div style={s.h}>
        Without kapi{" "}
        {r.contribution && (
          <Pill
            text={contributionLabel[r.contribution] ?? r.contribution}
            t={contributionTone(r.contribution)}
            title={contributionMeans(r.contribution)}
          />
        )}
      </div>
      <p style={{ ...s.sub, marginTop: 0 }}>
        The same prompt and the same workspace, with no skill, no MCP server and no kapi on PATH.
        Median messages: {withKapi} with kapi, {unaided} without.
      </p>
      <div style={{ display: "grid", gap: ".9rem" }}>
        {runs.map((run, i) => (
          <div
            key={i}
            style={{
              borderLeft: `3px solid ${tone.flat.fg}`,
              paddingLeft: ".8rem",
              display: "grid",
              gap: ".4rem",
            }}
          >
            <div style={{ display: "flex", gap: ".5rem", alignItems: "center", flexWrap: "wrap" }}>
              <span style={{ ...s.sub, fontWeight: 600 }}>pass {i + 1}</span>
              <span style={s.sub}>
                {run.messages} msg · {(run.durationMs / 1000).toFixed(1)}s
              </span>
              {run.gate && (
                <Pill
                  text={run.gate.exitCode === 0 ? "gate green" : "gate red"}
                  t={run.gate.exitCode === 0 ? "pass" : "fail"}
                />
              )}
            </div>
            {(run.changed ?? []).length > 0 && (
              <div style={{ display: "flex", gap: ".4rem", flexWrap: "wrap" }}>
                {run.changed!.map((c) => (
                  <code key={c.path} style={{ fontFamily: mono, fontSize: ".78rem" }}>
                    {c.path}
                  </code>
                ))}
              </div>
            )}
            <SessionButton sessions={sessions} onOpen={() => onOpen(i)} />
          </div>
        ))}
      </div>
    </div>
  );
}

// MessageCost is the other half of the control, and the counts say it plainly:
// on this suite the kapi route is not cheaper. "eased 0" on its own reads like
// an absence of evidence; it is not. The unaided arm took fewer messages on
// most scenarios, often several times fewer, and a page reporting what kapi
// added has to report that too.
function MessageCost({ results }: { results: Result[] }): ReactElement | null {
  const pairs = results
    .filter((r) => (r.unaided?.length ?? 0) > 0 && (r.runs?.length ?? 0) > 0)
    .map((r) => ({
      id: r.scenario.id,
      withKapi: median((r.runs ?? []).map((x) => x.messages)),
      unaided: median((r.unaided ?? []).map((x) => x.messages)),
    }))
    .filter((p) => p.withKapi > 0 && p.unaided > 0);
  if (pairs.length === 0) return null;

  const cheaper = pairs.filter((p) => p.unaided < p.withKapi).length;
  const totalWith = pairs.reduce((n, p) => n + p.withKapi, 0);
  const totalWithout = pairs.reduce((n, p) => n + p.unaided, 0);
  return (
    <p style={{ maxWidth: "72ch", fontSize: ".92rem", color: "var(--ifm-color-emphasis-800)" }}>
      <strong>What it cost.</strong> Across {pairs.length} scenarios the agent sent{" "}
      <strong>{totalWith}</strong> messages with kapi and <strong>{totalWithout}</strong> without,
      and the unaided arm was shorter on {cheaper} of them. That is the honest counterweight to the
      counts below: on this suite kapi reaches answers the unaided agent cannot, and it is not the
      cheaper route to the ones it can.
    </p>
  );
}

function median(xs: number[]): number {
  if (xs.length === 0) return 0;
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.floor(s.length / 2)];
}

function ScenarioRow({ r }: { r: Result }): ReactElement {
  const [open, setOpen] = useState(false);
  const sc = r.scenario;
  const n = r.runs?.length ?? 0;
  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        style={{ ...s.row, borderRadius: open ? "6px 6px 0 0" : 6 }}
      >
        <span style={{ display: "flex", gap: ".35rem", alignItems: "center" }}>
          <span aria-hidden style={{ fontFamily: mono, opacity: 0.5 }}>
            {open ? "−" : "+"}
          </span>
          <Pill text={r.verdict} t={verdictTone[r.verdict]} />
          {/* The control arm's answer belongs on the row rather than two clicks
              in: it is the difference between "this passed" and "this passed
              because of kapi". */}
          {r.contribution && r.contribution !== "unknown" && (
            <Pill
              text={contributionLabel[r.contribution] ?? r.contribution}
              t={contributionTone(r.contribution)}
              title={contributionMeans(r.contribution)}
            />
          )}
        </span>
        <span style={{ minWidth: 0 }}>
          <div style={s.prompt}>{sc.prompt}</div>
          <div style={s.sub}>
            {sc.id} · {sc.path}
          </div>
        </span>
        <span style={{ ...s.sub, whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}>
          {sc.surface === "mcp" && sc.expectTool
            ? `right tool ${r.foundTool ?? 0}/${n}`
            : `fired ${r.fired}/${n}`}
          {(r.wrongTool ?? []).length > 0 && (
            <span style={{ color: tone.fail.fg }}> · picked {r.wrongTool!.join(", ")}</span>
          )}
        </span>
      </button>
      {open && <Detail r={r} />}
    </div>
  );
}

export default function SkillEval(): ReactElement {
  // Reading order rather than alphabetical: does it fire, then does it finish,
  // then the other door. Sorting put MCP first, which is the least likely
  // question a reader arrives with.
  const modeOrder = ["skill:trigger", "skill:completion", "mcp:trigger"];
  const modes = Object.keys(reports).sort(
    (a, b) => (modeOrder.indexOf(a) + 1 || 99) - (modeOrder.indexOf(b) + 1 || 99),
  );
  const [mode, setMode] = useState(modes.includes("skill:trigger") ? "skill:trigger" : modes[0]);
  const report = reports[mode];
  const surface = report?.surface || "skill";

  const [positives, negatives] = useMemo(() => {
    const rs = report?.results ?? [];
    return [
      rs.filter((r) => r.scenario.kind === "positive"),
      rs.filter((r) => r.scenario.kind === "negative"),
    ];
  }, [report]);

  if (!report) {
    return (
      <Layout title="Skill eval">
        <main className="container margin-vert--lg">
          <h1>Skill eval</h1>
          <p>No run recorded yet. Run `make skill-eval`.</p>
        </main>
      </Layout>
    );
  }

  const sum = report.summary;
  const age = Math.round((Date.now() - Date.parse(report.generated)) / 86_400_000);

  return (
    <Layout
      title="Skill eval"
      description="Whether the shipped kapi Agent Skill fires on the right tasks, stays quiet on the wrong ones, and finishes the job."
    >
      <main className="container margin-vert--lg">
        <h1>Skill eval</h1>
        <p style={{ maxWidth: "68ch", fontSize: "1.05rem", lineHeight: 1.55 }}>
          An agent is handed a prompt and a workspace. Every row expands into the exact prompt, the
          files the agent saw, the tools and commands it reached for, and a diff of what it changed.
        </p>

        {modes.length > 1 && (
          <div style={{ display: "flex", gap: ".5rem", margin: "1rem 0" }}>
            {modes.map((m) => (
              <button
                key={m}
                type="button"
                onClick={() => setMode(m)}
                style={{
                  ...s.pill,
                  cursor: "pointer",
                  border: "1px solid var(--ifm-color-emphasis-300)",
                  background: m === mode ? "var(--ifm-color-primary)" : "transparent",
                  color: m === mode ? "var(--ifm-color-primary-contrast-background)" : "inherit",
                }}
              >
                {reportLabel[m] ?? m}
              </button>
            ))}
          </div>
        )}

        <div style={s.strip}>
          <div style={s.stat}>
            <span style={{ ...s.statN, color: tone.pass.fg }}>{sum.pass}</span>
            <span style={s.statL}>pass</span>
          </div>
          <div style={s.stat}>
            <span style={{ ...s.statN, color: tone.flaky.fg }}>{sum.flaky}</span>
            <span style={s.statL}>flaky</span>
          </div>
          <div style={s.stat}>
            <span style={{ ...s.statN, color: tone.fail.fg }}>{sum.fail}</span>
            <span style={s.statL}>fail</span>
          </div>
          {surface === "mcp" ? (
            <div style={s.stat}>
              <span style={{ ...s.statN, color: sum.wrongToolPicks ? tone.fail.fg : undefined }}>
                {sum.wrongToolPicks ?? 0}
              </span>
              <span style={s.statL}>wrong tool picked</span>
            </div>
          ) : (
            <div style={s.stat}>
              <span style={{ ...s.statN, color: sum.falseTriggers ? tone.fail.fg : undefined }}>
                {sum.falseTriggers}
              </span>
              <span style={s.statL}>false triggers</span>
            </div>
          )}
          {sum.gatesRun ? (
            <div style={s.stat}>
              <span style={s.statN}>
                {sum.gatesPassed}/{sum.gatesRun}
              </span>
              <span style={s.statL}>gates green</span>
            </div>
          ) : null}
          <div style={{ ...s.stat, marginLeft: "auto" }}>
            <span style={{ ...s.statN, color: age > 30 ? tone.flaky.fg : undefined }}>{age}d</span>
            <span style={s.statL}>since this run</span>
          </div>
        </div>

        <div style={s.meta}>
          <span style={s.metaK}>run</span>
          <span>
            {report.generated} · {report.repeat} passes per scenario
          </span>
          <span style={s.metaK}>agent</span>
          <span>
            {report.runner.claudeVersion} · {report.runner.model}
          </span>
          <span style={s.metaK}>skill</span>
          <span>
            {report.runner.skillCommit} · last edited {report.runner.skillModified}
          </span>
          {report.runner.kapi && (
            <>
              <span style={s.metaK}>kapi</span>
              <span>
                {report.runner.kapi} · {report.runner.kapiVersion}
              </span>
            </>
          )}
          <span style={s.metaK}>host</span>
          <span>{report.runner.host}</span>
          <span style={s.metaK}>settings</span>
          <span>{report.runner.settings}</span>
        </div>

        <p style={{ maxWidth: "72ch", fontSize: ".92rem", color: "var(--ifm-color-emphasis-800)" }}>
          {surfaceNote[surface]}
        </p>

        <ControlArm sum={sum} results={report.results} />

        <h2>
          {surface === "mcp" ? "Must pick the right tool" : "Must fire"} ({positives.length})
        </h2>
        <div style={{ display: "grid", gap: ".5rem" }}>
          {positives.map((r) => (
            <ScenarioRow key={r.scenario.id} r={r} />
          ))}
        </div>

        {negatives.length > 0 && (
          <>
            <h2 style={{ marginTop: "2.2rem" }}>Must stay quiet ({negatives.length})</h2>
            <div style={{ display: "grid", gap: ".5rem" }}>
              {negatives.map((r) => (
                <ScenarioRow key={r.scenario.id} r={r} />
              ))}
            </div>
          </>
        )}
      </main>
    </Layout>
  );
}
