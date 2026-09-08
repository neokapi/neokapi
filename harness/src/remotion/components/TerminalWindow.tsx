import React from "react";
import { AbsoluteFill, useCurrentFrame } from "remotion";
import { theme, CLAUDE, KAPI } from "./theme.ts";
import type { SceneLayout } from "./layout.ts";

/** Height of the macOS title bar. */
export const TITLE_BAR = 48;

const Light: React.FC<{ c: string }> = ({ c }) => (
  <span style={{ width: 14, height: 14, borderRadius: 14, background: c, display: "inline-block" }} />
);

/** The Claude Code input box (persistent at the bottom of the window). Shell demos
 * carry their own `$` prompt + cursor in the transcript, so this is Claude-only. */
const InputBox: React.FC = () => {
  const frame = useCurrentFrame();
  const blink = frame % 30 < 16;
  return (
    <div style={{ padding: "0 22px 18px" }}>
      <div
        style={{
          border: `1px solid ${theme.termFaint}`,
          borderRadius: 10,
          padding: "14px 18px",
          display: "flex",
          alignItems: "center",
          gap: 12,
          fontFamily: theme.fontMono,
          fontSize: 26,
        }}
      >
        <span style={{ color: CLAUDE }}>&gt;</span>
        <span style={{ width: 13, height: 28, background: theme.termText, opacity: blink ? 0.9 : 0.12, borderRadius: 2 }} />
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", padding: "8px 6px 0", fontFamily: theme.fontMono, fontSize: 19, color: theme.termFaint }}>
        <span>⏵⏵ accept edits on</span>
        <span>? for shortcuts</span>
      </div>
    </div>
  );
};

/**
 * A macOS terminal window (Claude Code, or a plain shell), placed on the
 * scene's grid: its text starts inside the safe area and its bottom edge
 * stays above the caption band, so nothing spoken ever covers a line.
 */
export const TerminalWindow: React.FC<{
  model?: string;
  /** Plain shell mode: drops the Claude chrome (banner is omitted by the child, this swaps the title bar + input box). */
  shell?: boolean;
  /** Working-directory label for the title bar (shell mode). */
  cwd?: string;
  layout: SceneLayout;
  children: React.ReactNode;
}> = ({ model, shell, cwd = "~/project", layout, children }) => {
  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans }}>
      <div
        style={{
          position: "absolute",
          left: layout.left,
          top: layout.picTop,
          width: layout.width,
          height: layout.stackBottom - layout.picTop,
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
        <div style={{ display: "flex", alignItems: "center", gap: 14, height: TITLE_BAR, padding: "0 20px", background: "#26282e", borderBottom: `1px solid rgba(0,0,0,0.3)` }}>
          <Light c="#ff5f57" />
          <Light c="#febc2e" />
          <Light c="#28c840" />
          <div style={{ flex: 1, textAlign: "center", color: theme.termDim, fontSize: 22, fontFamily: theme.fontMono }}>
            {shell ? (
              <span>{cwd} — zsh</span>
            ) : (
              <>
                <span style={{ color: CLAUDE }}>✻</span> claude — ~/project
              </>
            )}
          </div>
          <div style={{ color: shell ? KAPI : theme.termFaint, fontSize: 20, fontWeight: shell ? 700 : 400, fontFamily: theme.fontMono }}>
            {shell ? "kapi" : model}
          </div>
        </div>
        {/* transcript (scrolls, pinned to bottom) */}
        <div style={{ flex: 1, position: "relative" }}>{children}</div>
        {/* Claude's persistent input box; shell demos render their own prompt inline. */}
        {shell ? null : <InputBox />}
      </div>
    </AbsoluteFill>
  );
};
