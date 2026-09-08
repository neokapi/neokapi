import React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme, CLAUDE } from "./theme.ts";
import { PROMPT_FS, SAFE_X, SAFE_Y, sceneLayout } from "./layout.ts";

/**
 * Full-screen view of the user's actual request, the thing a developer types into
 * Claude Code. The point is that an ordinary prompt, plus the kapi skill, is
 * enough; so the prompt is shown verbatim and given room to read.
 */
export const PromptCard: React.FC<{ prompt: string }> = ({ prompt }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const intro = spring({ frame, fps, config: { damping: 200 } });
  const caret = frame % 30 < 16;

  return (
    <AbsoluteFill
      style={{
        background: theme.bgGrad,
        fontFamily: theme.fontSans,
        justifyContent: "center",
        alignItems: "center",
        // Centred in the room above the caption band, like the cards.
        padding: `${SAFE_Y}px ${SAFE_X}px ${1080 - sceneLayout(false).stackBottom}px ${SAFE_X}px`,
      }}
    >
      <div style={{ color: theme.dim, fontSize: 36, letterSpacing: 3, textTransform: "uppercase", marginBottom: 34, opacity: intro }}>
        What the developer types
      </div>
      <div
        style={{
          maxWidth: 1920 - 2 * SAFE_X,
          width: "100%",
          background: theme.panel,
          border: `1px solid ${theme.panelBorder}`,
          borderRadius: 18,
          padding: "44px 52px",
          display: "flex",
          gap: 26,
          boxShadow: "0 40px 100px rgba(0,0,0,0.5)",
          opacity: intro,
          translate: `0px ${interpolate(intro, [0, 1], [24, 0])}px`,
        }}
      >
        <span style={{ color: CLAUDE, fontFamily: theme.fontMono, fontSize: PROMPT_FS, lineHeight: 1.4, flex: "none" }}>&gt;</span>
        <div style={{ color: theme.text, fontSize: PROMPT_FS, lineHeight: 1.4, fontWeight: 450 }}>
          {prompt}
          <span style={{ display: "inline-block", width: 20, height: PROMPT_FS - 6, marginLeft: 8, translate: "0px 5px", background: theme.text, opacity: caret ? 0.85 : 0.1, borderRadius: 2 }} />
        </div>
      </div>
    </AbsoluteFill>
  );
};
