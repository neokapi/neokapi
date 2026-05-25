import React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme, CLAUDE, KAPI } from "./theme.ts";

const Light: React.FC<{ c: string }> = ({ c }) => (
  <span style={{ width: 14, height: 14, borderRadius: 14, background: c, display: "inline-block" }} />
);

/** The Claude Code input box (persistent at the bottom of the window). Shell demos
 * carry their own `$` prompt + cursor in the transcript, so this is Claude-only. */
const InputBox: React.FC = () => {
  const frame = useCurrentFrame();
  const blink = frame % 30 < 16;
  return (
    <div style={{ padding: "0 18px 16px" }}>
      <div
        style={{
          border: `1px solid ${theme.termFaint}`,
          borderRadius: 10,
          padding: "13px 16px",
          display: "flex",
          alignItems: "center",
          gap: 10,
          fontFamily: theme.fontMono,
          fontSize: 20,
        }}
      >
        <span style={{ color: CLAUDE }}>&gt;</span>
        <span style={{ width: 11, height: 22, background: theme.termText, opacity: blink ? 0.9 : 0.12, borderRadius: 2 }} />
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", padding: "7px 6px 0", fontFamily: theme.fontMono, fontSize: 15, color: theme.termFaint }}>
        <span>⏵⏵ accept edits on</span>
        <span>? for shortcuts</span>
      </div>
    </div>
  );
};

/** A macOS terminal window (Claude Code, or a plain shell) with a caption lower-third below it. */
export const TerminalWindow: React.FC<{
  model?: string;
  caption: string;
  /** Plain shell mode: drops the Claude chrome (banner is omitted by the child, this swaps the title bar + input box). */
  shell?: boolean;
  /** Working-directory label for the title bar (shell mode). */
  cwd?: string;
  children: React.ReactNode;
}> = ({ model, caption, shell, cwd = "~/project", children }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const capOpacity = spring({ frame: frame - 4, fps, config: { damping: 200 } });

  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans, padding: "48px 60px 0", flexDirection: "column" }}>
      <div
        style={{
          flex: 1,
          background: theme.termBg,
          border: `1px solid ${theme.panelBorder}`,
          borderRadius: 16,
          overflow: "hidden",
          boxShadow: "0 40px 100px rgba(0,0,0,0.6)",
          display: "flex",
          flexDirection: "column",
        }}
      >
        {/* macOS title bar */}
        <div style={{ display: "flex", alignItems: "center", gap: 13, padding: "14px 20px", background: "#26282e", borderBottom: `1px solid rgba(0,0,0,0.3)` }}>
          <Light c="#ff5f57" />
          <Light c="#febc2e" />
          <Light c="#28c840" />
          <div style={{ flex: 1, textAlign: "center", color: theme.termDim, fontSize: 18, fontFamily: theme.fontMono }}>
            {shell ? (
              <span>{cwd} — zsh</span>
            ) : (
              <>
                <span style={{ color: CLAUDE }}>✻</span> claude — ~/project
              </>
            )}
          </div>
          <div style={{ color: shell ? KAPI : theme.termFaint, fontSize: shell ? 16 : 15, fontWeight: shell ? 700 : 400, fontFamily: theme.fontMono }}>
            {shell ? "kapi" : model}
          </div>
        </div>
        {/* transcript (scrolls, pinned to bottom) */}
        <div style={{ flex: 1, position: "relative" }}>{children}</div>
        {/* Claude's persistent input box; shell demos render their own prompt inline. */}
        {shell ? null : <InputBox />}
      </div>

      {/* caption lower-third (below the window, never covers the terminal) */}
      <div style={{ height: 150, display: "flex", alignItems: "center", justifyContent: "center" }}>
        {caption ? (
          <div
            style={{
              maxWidth: 1500,
              padding: "16px 32px",
              background: "rgba(8,11,19,0.9)",
              border: "1px solid rgba(255,255,255,0.14)",
              borderRadius: 14,
              // Always-dark lower-third → always-light text (theme.text flips dark in light mode).
              color: "#f4f7ff",
              fontSize: 29,
              lineHeight: 1.3,
              fontWeight: 500,
              textAlign: "center",
              fontFamily: theme.fontSans,
              opacity: capOpacity,
              transform: `translateY(${interpolate(capOpacity, [0, 1], [10, 0])}px)`,
            }}
          >
            {caption}
          </div>
        ) : null}
      </div>
    </AbsoluteFill>
  );
};
