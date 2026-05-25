import React from "react";
import { useCurrentFrame } from "remotion";
import type { TimelineEvent } from "../../types.ts";
import { theme } from "./theme.ts";

interface Props {
  events: TimelineEvent[];
  /** Frames of terminal time elapsed in earlier terminal scenes. */
  globalTermFrom: number;
  /** Total frames across all terminal scenes (reveal is spread across this). */
  totalTermFrames: number;
}

const FS = 21;
const LH = 1.5;

const mono = (extra: React.CSSProperties = {}): React.CSSProperties => ({
  fontFamily: theme.fontMono,
  fontSize: FS,
  lineHeight: LH,
  ...extra,
});

const Prompt: React.FC = () => <span style={mono({ color: theme.termGreen, flex: "none", fontWeight: 600 })}>$</span>;

const Cursor: React.FC<{ on: boolean }> = ({ on }) => (
  <span style={{ display: "inline-block", width: 10, height: 21, marginLeft: 3, transform: "translateY(4px)", background: theme.termText, opacity: on ? 0.85 : 0.12, borderRadius: 2 }} />
);

/** A completed (already-revealed) event: `$ command`, a `# comment`, or output. */
function renderBlock(ev: TimelineEvent): React.ReactNode {
  switch (ev.kind) {
    case "comment":
      return <div style={mono({ color: theme.termDim, whiteSpace: "pre-wrap", wordBreak: "break-word" })}># {ev.text}</div>;
    case "command":
      return (
        <div style={{ display: "flex", gap: 11 }}>
          <Prompt />
          <div style={mono({ color: theme.termText, flex: 1, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>{ev.text}</div>
        </div>
      );
    case "output":
      if (!ev.text) return null;
      return (
        <div style={mono({ color: ev.isError ? theme.termRed : theme.termText, opacity: ev.isError ? 1 : 0.92, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>
          {ev.text}
        </div>
      );
    default:
      return null;
  }
}

/**
 * A plain shell session replay — the toolbox demo without any Claude chrome.
 * Reveals events progressively across all terminal scenes (same reveal math as
 * the Claude terminal). The line being typed (a command or comment) carries the
 * blinking cursor; a bare `$ ▌` waiting prompt is shown only between commands —
 * never stacked under the line currently being typed.
 */
export const PlainTerminal: React.FC<Props> = ({ events, globalTermFrom, totalTermFrames }) => {
  const localFrame = useCurrentFrame();
  const blinkOn = localFrame % 30 < 16;
  const N = events.length;
  const global = globalTermFrom + localFrame;
  const progress = totalTermFrames > 0 ? Math.min(1, global / totalTermFrames) : 1;
  const exact = Math.min(N, progress * (N + 0.5));
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
        padding: "24px 34px 18px",
        gap: 10,
        background: theme.termBg,
      }}
    >
      {shown.map((ev) => (
        <div key={ev.i}>{renderBlock(ev)}</div>
      ))}

      {/* Output reveals fully as it comes into range; commands/comments type in below. */}
      {inProgress && !typing ? <div key={`ip-${inProgress.i}`}>{renderBlock(inProgress)}</div> : null}

      {typing?.kind === "comment" ? (
        <div style={mono({ color: theme.termDim, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>
          # {typed}
          <Cursor on={blinkOn} />
        </div>
      ) : typing?.kind === "command" ? (
        <div style={{ display: "flex", gap: 11 }}>
          <Prompt />
          <div style={mono({ color: theme.termText, flex: 1, whiteSpace: "pre-wrap", wordBreak: "break-word" })}>
            {typed}
            <Cursor on={blinkOn} />
          </div>
        </div>
      ) : (
        // Between commands (or finished): a single waiting prompt with the cursor.
        <div style={{ display: "flex", gap: 11 }}>
          <Prompt />
          <Cursor on={blinkOn} />
        </div>
      )}
    </div>
  );
};
