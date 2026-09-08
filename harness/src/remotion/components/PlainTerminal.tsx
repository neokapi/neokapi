import React from "react";
import { useCurrentFrame, useVideoConfig } from "remotion";
import type { TimelineEvent } from "../../types.ts";
import { theme } from "./theme.ts";
import { TERM_FS, TERM_LH } from "./layout.ts";
import { LineMark, lineMatches } from "./Marks.tsx";

interface Props {
  events: TimelineEvent[];
  /** Events revealed at the scene's first and last frame (see SceneTiming). */
  revealStart: number;
  revealEnd: number;
  /** Output lines containing one of these are marked as they appear. */
  highlight?: string[];
}

const mono = (extra: React.CSSProperties = {}): React.CSSProperties => ({
  fontFamily: theme.fontMono,
  fontSize: TERM_FS,
  lineHeight: TERM_LH,
  ...extra,
});

const Prompt: React.FC = () => <span style={mono({ color: theme.termGreen, flex: "none", fontWeight: 600 })}>$</span>;

const Cursor: React.FC<{ on: boolean }> = ({ on }) => (
  <span style={{ display: "inline-block", width: 14, height: TERM_FS, marginLeft: 4, translate: "0px 5px", background: theme.termText, opacity: on ? 0.85 : 0.12, borderRadius: 2 }} />
);

/** An output block, one element per line so a single line can carry a mark. */
const OutputLines: React.FC<{ text: string; isError: boolean; highlight?: string[] }> = ({ text, isError, highlight }) => (
  <div style={mono({ color: isError ? theme.termRed : theme.termText, opacity: isError ? 1 : 0.92, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>
    {text.split("\n").map((line, i) => (
      <div key={i}>
        <LineMark on={lineMatches(line, highlight)}>{line || " "}</LineMark>
      </div>
    ))}
  </div>
);

/** A completed (already-revealed) event: `$ command`, a `# comment`, or output. */
function renderBlock(ev: TimelineEvent, highlight?: string[]): React.ReactNode {
  switch (ev.kind) {
    case "comment":
      return <div style={mono({ color: theme.termDim, whiteSpace: "pre-wrap", wordBreak: "break-word" })}># {ev.text}</div>;
    case "command":
      return (
        <div style={{ display: "flex", gap: 14 }}>
          <Prompt />
          <div style={mono({ color: theme.termText, flex: 1, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>{ev.text}</div>
        </div>
      );
    case "output":
      if (!ev.text) return null;
      return <OutputLines text={ev.text} isError={ev.isError} highlight={highlight} />;
    default:
      return null;
  }
}

/**
 * A plain shell session replay, without any Claude chrome. Events reveal
 * progressively between the scene's two anchors (revealStart to revealEnd):
 * the line being typed (a command or comment) carries the blinking cursor,
 * and a bare `$ ▌` waiting prompt is shown only between commands.
 */
export const PlainTerminal: React.FC<Props> = ({ events, revealStart, revealEnd, highlight }) => {
  const localFrame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const blinkOn = localFrame % 30 < 16;
  const N = events.length;
  const p = durationInFrames > 0 ? Math.min(1, localFrame / durationInFrames) : 1;
  const exact = Math.min(N, revealStart + (revealEnd - revealStart) * p);
  const fullCount = Math.floor(exact);
  const frac = exact - fullCount;

  const shown = events.slice(0, Math.min(fullCount, N));
  const inProgress = fullCount < N ? events[fullCount] : undefined;
  // Narrow to the typeable variants so `.text` is available (tool_use has none).
  const typing = inProgress && (inProgress.kind === "command" || inProgress.kind === "comment") ? inProgress : null;
  const typed = typing ? typing.text.slice(0, Math.max(1, Math.ceil(frac * typing.text.length))) : "";

  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        display: "flex",
        flexDirection: "column",
        justifyContent: "flex-end",
        overflow: "hidden",
        padding: "30px 34px 22px",
        gap: 12,
        background: theme.termBg,
      }}
    >
      {shown.map((ev) => (
        <div key={ev.i}>{renderBlock(ev, highlight)}</div>
      ))}

      {/* Output reveals fully as it comes into range; commands/comments type in below. */}
      {inProgress && !typing ? <div key={`ip-${inProgress.i}`}>{renderBlock(inProgress, highlight)}</div> : null}

      {typing?.kind === "comment" ? (
        <div style={mono({ color: theme.termDim, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>
          # {typed}
          <Cursor on={blinkOn} />
        </div>
      ) : typing?.kind === "command" ? (
        <div style={{ display: "flex", gap: 14 }}>
          <Prompt />
          <div style={mono({ color: theme.termText, flex: 1, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>
            {typed}
            <Cursor on={blinkOn} />
          </div>
        </div>
      ) : (
        // Between commands (or finished): a single waiting prompt with the cursor.
        <div style={{ display: "flex", gap: 14 }}>
          <Prompt />
          <Cursor on={blinkOn} />
        </div>
      )}
    </div>
  );
};
