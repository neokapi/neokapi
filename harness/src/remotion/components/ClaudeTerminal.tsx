import React from "react";
import { useCurrentFrame, useVideoConfig } from "remotion";
import type { TimelineEvent } from "../../types.ts";
import { theme, CLAUDE } from "./theme.ts";
import { TERM_FS, TERM_LH, TERM_PAD_X, TERM_TEXT_WIDTH } from "./layout.ts";
import { LineMark, lineMatches } from "./Marks.tsx";

interface Props {
  events: TimelineEvent[];
  model: string;
  /** Events revealed at the scene's first and last frame (see SceneTiming). */
  revealStart: number;
  revealEnd: number;
  /** Result lines containing one of these are marked as they appear. */
  highlight?: string[];
  showThinking?: boolean;
}

// Claude Code's default (non-verbose / non-ctrl+o) view collapses tool output to a few
// lines with a "+N lines" indicator, rather than printing every command's full output.
const RESULT_MAX_LINES = 3;
const FS = TERM_FS; // base monospace font size
const LH = TERM_LH;

function clampLines(text: string, n: number): { body: string; more: number } {
  const lines = text.split("\n");
  if (lines.length <= n) return { body: text, more: 0 };
  return { body: lines.slice(0, n).join("\n"), more: lines.length - n };
}

const mono = (extra: React.CSSProperties = {}): React.CSSProperties => ({
  fontFamily: theme.fontMono,
  fontSize: FS,
  lineHeight: LH,
  ...extra,
});

/** ⏺ bullet + hanging-indented content, the core Claude Code line shape. */
const BulletLine: React.FC<{ color: string; children: React.ReactNode }> = ({ color, children }) => (
  <div style={{ display: "flex", gap: 12 }}>
    <span style={mono({ color, flex: "none" })}>⏺</span>
    <div style={mono({ color: theme.termText, flex: 1, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>{children}</div>
  </div>
);

// Claude Code's bootstrap banner: the block-art logo (Claude coral/orange) beside
// the version, model, and cwd — the real first thing you see when the CLI starts.
const CLAUDE_LOGO = [" ▐▛███▜▌", "▝▜█████▛▘", "  ▘▘ ▝▝  "];
const CLAUDE_CODE_VERSION = "v2.1.148";

/** "claude-sonnet-4-5-20250929" → "Sonnet 4.5"; "sonnet" → "Sonnet"; else passthrough. */
function formatModel(model: string): string {
  const m = model.match(/(opus|sonnet|haiku)(?:[-\s]?(\d+))?(?:[-.](\d+))?/i);
  if (!m) return model;
  const fam = m[1][0].toUpperCase() + m[1].slice(1).toLowerCase();
  const ver = [m[2], m[3]].filter(Boolean).join(".");
  return ver ? `${fam} ${ver}` : fam;
}

const BootstrapBanner: React.FC<{ model: string; cwd?: string }> = ({ model, cwd = "~/project" }) => (
  <div style={{ display: "flex", gap: 18, alignItems: "center" }}>
    <pre style={mono({ color: CLAUDE, fontWeight: 700, lineHeight: 1.15, margin: 0, whiteSpace: "pre" })}>{CLAUDE_LOGO.join("\n")}</pre>
    <div style={{ display: "flex", flexDirection: "column", justifyContent: "center", lineHeight: 1.45 }}>
      <div style={mono({ color: theme.termText })}>
        <span style={{ fontWeight: 700 }}>Claude Code</span> <span style={{ color: theme.termDim }}>{CLAUDE_CODE_VERSION}</span>
      </div>
      <div style={mono({ color: theme.termDim, fontSize: FS - 3 })}>{formatModel(model)}</div>
      <div style={mono({ color: theme.termDim, fontSize: FS - 3 })}>{cwd}</div>
    </div>
  </div>
);

const UserPrompt: React.FC<{ text: string }> = ({ text }) => (
  <div style={{ display: "flex", gap: 12 }}>
    <span style={mono({ color: theme.termDim, flex: "none" })}>&gt;</span>
    <div style={mono({ color: theme.termText, opacity: 0.92, flex: 1, whiteSpace: "pre-wrap" })}>{text}</div>
  </div>
);

const Thinking: React.FC<{ text: string }> = ({ text }) => (
  <div style={mono({ color: theme.termDim, fontStyle: "italic", fontSize: FS - 2, whiteSpace: "pre-wrap" })}>
    <span style={{ color: CLAUDE, fontStyle: "normal" }}>✶ </span>
    {text.length > 200 ? text.slice(0, 197) + "…" : text}
  </div>
);

function toolCall(ev: Extract<TimelineEvent, { kind: "tool_use" }>, typed?: string): React.ReactNode {
  // Claude Code renders actions as:  ⏺ Tool(arg)
  let label: React.ReactNode;
  if (ev.tool === "Bash") {
    label = (
      <>
        <span style={{ fontWeight: 700 }}>Bash</span>
        <span style={{ color: theme.termDim }}>(</span>
        <span style={{ color: theme.termGreen }}>{typed ?? ev.command}</span>
        <span style={{ color: theme.termDim }}>)</span>
      </>
    );
  } else {
    // Read / Write / Glob / mcp tool / etc.  ev.title already prettifies mcp names.
    const name = ev.tool.startsWith("mcp__") ? ev.title : ev.tool;
    label = (
      <>
        <span style={{ fontWeight: 700 }}>{name}</span>
        {ev.detail ? (
          <>
            <span style={{ color: theme.termDim }}>(</span>
            <span style={{ color: theme.termDim }}>{ev.detail}</span>
            <span style={{ color: theme.termDim }}>)</span>
          </>
        ) : null}
      </>
    );
  }
  return <BulletLine color={CLAUDE}>{label}</BulletLine>;
}

const ToolResult: React.FC<{ ev: Extract<TimelineEvent, { kind: "tool_result" }>; highlight?: string[] }> = ({ ev, highlight }) => {
  const { body, more } = clampLines(ev.output || "", RESULT_MAX_LINES);
  const color = ev.isError ? theme.termRed : theme.termDim;
  return (
    <div style={{ display: "flex", gap: 10, paddingLeft: 12 }}>
      <span style={mono({ color, flex: "none" })}>⎿</span>
      <div style={mono({ color, flex: 1, fontSize: FS - 2, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>
        {body
          ? body.split("\n").map((line, i) => (
              <div key={i}>
                <LineMark on={lineMatches(line, highlight)} width={TERM_TEXT_WIDTH - 42}>
                  {line || "\u00a0"}
                </LineMark>
              </div>
            ))
          : "(no output)"}
        {more > 0 ? <div style={{ color: theme.termFaint }}>… +{more} lines</div> : null}
      </div>
    </div>
  );
};

// The kapi check Stop hook fired and refused to let Claude finish. Render it as
// a distinct gate card so it reads as "kapi stopped me", not just another result.
const HookBlock: React.FC<{ ev: Extract<TimelineEvent, { kind: "hook_block" }> }> = ({ ev }) => (
  <div
    style={{
      border: `1px solid ${theme.termRed}`,
      borderRadius: 8,
      padding: "10px 16px",
      background: "rgba(244,113,103,0.08)",
    }}
  >
    <div style={mono({ color: theme.termRed, fontWeight: 700 })}>✗ kapi check --ship — blocked (Stop hook)</div>
    {ev.findings.slice(0, 4).map((f, i) => (
      <div key={i} style={mono({ color: theme.termText, fontSize: FS - 3, whiteSpace: "pre-wrap", marginTop: 3 })}>
        {f}
      </div>
    ))}
    {ev.findings.length > 4 ? (
      <div style={mono({ color: theme.termFaint, fontSize: FS - 3 })}>… +{ev.findings.length - 4} more</div>
    ) : null}
  </div>
);

const HookPass: React.FC = () => (
  <div
    style={{
      border: `1px solid ${theme.termGreen}`,
      borderRadius: 8,
      padding: "10px 16px",
      background: "rgba(126,231,135,0.08)",
    }}
  >
    <div style={mono({ color: theme.termGreen, fontWeight: 700 })}>✓ kapi check --ship — passed</div>
  </div>
);

function renderBlock(ev: TimelineEvent, typed?: string, highlight?: string[]): React.ReactNode {
  switch (ev.kind) {
    case "prompt":
      return <UserPrompt text={typed ?? ev.text} />;
    case "text":
    case "result":
      return <BulletLine color={CLAUDE}>{typed ?? ev.text}</BulletLine>;
    case "thinking":
      return <Thinking text={typed ?? ev.text} />;
    case "skill":
      return (
        <BulletLine color={CLAUDE}>
          <span style={{ fontWeight: 700 }}>Skill</span>
          <span style={{ color: theme.termDim }}>({ev.name})</span>
        </BulletLine>
      );
    case "tool_use":
      return toolCall(ev, typed);
    case "tool_result":
      return <ToolResult ev={ev} highlight={highlight} />;
    case "hook_block":
      return <HookBlock ev={ev} />;
    case "hook_pass":
      return <HookPass />;
  }
}

export const ClaudeTerminal: React.FC<Props> = ({ events, model, revealStart, revealEnd, highlight, showThinking = true }) => {
  const localFrame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const filtered = showThinking ? events : events.filter((e) => e.kind !== "thinking");
  const N = filtered.length;
  // The reveal anchors count every event; thinking lines hidden here shift the
  // count by the same fraction on both ends.
  const scale = events.length > 0 ? N / events.length : 1;
  const p = durationInFrames > 0 ? Math.min(1, localFrame / durationInFrames) : 1;
  const exact = Math.min(N, (revealStart + (revealEnd - revealStart) * p) * scale);
  const fullCount = Math.floor(exact);
  const frac = exact - fullCount;

  const shown = filtered.slice(0, Math.min(fullCount, N));
  const inProgress = fullCount < N ? filtered[fullCount] : undefined;

  let typed: string | undefined;
  if (inProgress) {
    const src = inProgress.kind === "tool_use" ? inProgress.command : (inProgress as any).text;
    if (typeof src === "string") typed = src.slice(0, Math.max(1, Math.ceil(frac * src.length)));
  }

  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        display: "flex",
        flexDirection: "column",
        justifyContent: "flex-end",
        overflow: "hidden",
        padding: `30px ${TERM_PAD_X}px 18px`,
        gap: 16,
        background: theme.termBg,
      }}
    >
      <BootstrapBanner model={model} />
      {shown.map((ev) => (
        <div key={ev.i}>{renderBlock(ev, undefined, highlight)}</div>
      ))}
      {inProgress ? <div key={`ip-${inProgress.i}`}>{renderBlock(inProgress, typed, highlight)}</div> : null}
    </div>
  );
};
