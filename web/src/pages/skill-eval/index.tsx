import { useMemo, useState } from "react";
import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
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
function ControlArm({ sum }: { sum: Report["summary"] }): ReactElement | null {
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
            <div style={{ fontWeight: 600, fontSize: ".9rem" }}>{c.label}</div>
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
    gridTemplateColumns: "5.5rem 1fr auto",
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

function Pill({ text, t }: { text: string; t: Tone }): ReactElement {
  return <span style={{ ...s.pill, color: tone[t].fg, background: tone[t].bg }}>{text}</span>;
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
              <div
                style={{ display: "flex", gap: ".6rem", alignItems: "center", flexWrap: "wrap" }}
              >
                <Pill
                  text={run.triggered ? "fired" : "silent"}
                  t={run.triggered === (sc.kind === "positive") ? "pass" : "fail"}
                />
                <span style={s.sub}>
                  {run.messages} msg · {(run.durationMs / 1000).toFixed(1)}s
                </span>
                {(run.tools ?? []).map((t) => (
                  <span
                    key={t}
                    style={{
                      ...s.pill,
                      ...tone.flat,
                      background: tone.flat.bg,
                      color: tone.flat.fg,
                    }}
                  >
                    {t}
                  </span>
                ))}
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

              {(run.kapiCommands ?? []).length > 0 && (
                <div style={{ display: "grid", gap: ".25rem" }}>
                  {run.kapiCommands!.map((c, j) => (
                    <div key={j} style={s.cmd}>
                      $ {c}
                    </div>
                  ))}
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
                          {c.binary && (
                            <span style={s.sub}>binary, {c.bytesAfter?.toLocaleString()} B</span>
                          )}
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

              {run.finalText && (
                <div>
                  <div style={s.h}>Closing message</div>
                  <pre style={s.pre}>{run.finalText}</pre>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
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
        </span>
        <span>
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

        <ControlArm sum={sum} />

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
