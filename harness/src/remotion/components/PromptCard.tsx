import React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme, CLAUDE } from "./theme.ts";

/**
 * Full-screen view of the user's actual request — the thing a developer types into
 * Claude Code. The whole point of the harness is that an ordinary prompt, plus the kapi
 * skill, is enough; so the prompt is shown verbatim and given room to read.
 */
export const PromptCard: React.FC<{ prompt: string }> = ({ prompt }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const intro = spring({ frame, fps, config: { damping: 200 } });
  const caret = frame % 30 < 16;

  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans, justifyContent: "center", alignItems: "center", padding: "120px 140px" }}>
      <div style={{ color: theme.dim, fontSize: 24, letterSpacing: 3, textTransform: "uppercase", marginBottom: 30, opacity: intro }}>
        What the developer types
      </div>
      <div
        style={{
          maxWidth: 1300,
          width: "100%",
          background: theme.panel,
          border: `1px solid ${theme.panelBorder}`,
          borderRadius: 18,
          padding: "40px 46px",
          display: "flex",
          gap: 22,
          boxShadow: "0 40px 100px rgba(0,0,0,0.5)",
          opacity: intro,
          transform: `translateY(${interpolate(intro, [0, 1], [24, 0])}px)`,
        }}
      >
        <span style={{ color: CLAUDE, fontFamily: theme.fontMono, fontSize: 40, lineHeight: "52px", flex: "none" }}>&gt;</span>
        <div style={{ color: theme.text, fontSize: 38, lineHeight: 1.45, fontWeight: 450 }}>
          {prompt}
          <span style={{ display: "inline-block", width: 16, height: 34, marginLeft: 6, transform: "translateY(4px)", background: theme.text, opacity: caret ? 0.85 : 0.1, borderRadius: 2 }} />
        </div>
      </div>
    </AbsoluteFill>
  );
};
