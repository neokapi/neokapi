import React from "react";
import { AbsoluteFill, Img, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import type { CapturedArtifact } from "../../types.ts";
import { theme } from "./theme.ts";

/** Full-screen spotlight of a captured artifact (rendered app, report card, …) with a gentle Ken-Burns. */
export const ArtifactView: React.FC<{ demoId: string; artifact: CapturedArtifact; caption: string }> = ({
  demoId,
  artifact,
  caption,
}) => {
  const frame = useCurrentFrame();
  const { fps, durationInFrames } = useVideoConfig();
  const intro = spring({ frame, fps, config: { damping: 200 } });
  const zoom = interpolate(frame, [0, durationInFrames], [1.02, 1.08]);
  const drift = interpolate(frame, [0, durationInFrames], [0, -16]);

  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans, justifyContent: "center", alignItems: "center", padding: "70px 90px 140px" }}>
      <div
        style={{
          position: "relative",
          borderRadius: 18,
          overflow: "hidden",
          boxShadow: "0 50px 120px rgba(0,0,0,0.6), 0 0 0 1px rgba(255,255,255,0.08)",
          opacity: intro,
          transform: `translateY(${interpolate(intro, [0, 1], [30, 0])}px) scale(${interpolate(intro, [0, 1], [0.96, 1])})`,
          maxWidth: "82%",
          maxHeight: "78%",
        }}
      >
        <Img
          src={staticFile(`${demoId}/${artifact.image}`)}
          style={{ display: "block", width: "100%", height: "100%", objectFit: "contain", transform: `scale(${zoom}) translateY(${drift}px)` }}
        />
      </div>
      <div
        style={{
          position: "absolute",
          bottom: 70,
          padding: "16px 30px",
          background: "rgba(10,14,28,0.9)",
          border: "1px solid rgba(255,255,255,0.14)",
          borderRadius: 14,
          // The lower-third box is always dark, so the text is always light —
          // theme.text flips to dark in light mode and vanished into the box.
          color: "#f4f7ff",
          fontSize: 28,
          fontWeight: 500,
          opacity: spring({ frame: frame - 10, fps, config: { damping: 200 } }),
          backdropFilter: "blur(8px)",
          maxWidth: "80%",
          textAlign: "center",
        }}
      >
        {caption}
      </div>
    </AbsoluteFill>
  );
};
