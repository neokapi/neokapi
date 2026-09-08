import React from "react";
import { AbsoluteFill, Img, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import type { CapturedArtifact, ZoomRect } from "../../types.ts";
import { theme } from "./theme.ts";
import type { SceneLayout } from "./layout.ts";
import { BoxMark } from "./Marks.tsx";

/** Where an image of the artifact's aspect lands inside the scene's picture box. */
export function fitArtifact(artifact: { width: number; height: number }, layout: SceneLayout): { x: number; y: number; w: number; h: number } {
  const boxW = layout.width;
  const boxH = layout.stackBottom - layout.picTop;
  const aspect = artifact.width > 0 && artifact.height > 0 ? artifact.width / artifact.height : 16 / 9;
  let w = boxW;
  let h = w / aspect;
  if (h > boxH) {
    h = boxH;
    w = h * aspect;
  }
  return { x: layout.left + (boxW - w) / 2, y: layout.picTop + (boxH - h) / 2, w, h };
}

/**
 * A captured artifact (rendered app, report card, document) in the picture
 * box, with a slow push-in, and a hand-drawn box around the region the scene
 * names.
 */
export const ArtifactView: React.FC<{ demoId: string; artifact: CapturedArtifact; layout: SceneLayout; highlight?: ZoomRect }> = ({
  demoId,
  artifact,
  layout,
  highlight,
}) => {
  const frame = useCurrentFrame();
  const { fps, durationInFrames } = useVideoConfig();
  const intro = spring({ frame, fps, config: { damping: 200 } });
  const rect = fitArtifact(artifact, layout);

  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans }}>
      <div
        style={{
          position: "absolute",
          left: rect.x,
          top: rect.y,
          width: rect.w,
          height: rect.h,
          borderRadius: 18,
          overflow: "hidden",
          boxShadow: "0 50px 120px rgba(0,0,0,0.6), 0 0 0 1px rgba(255,255,255,0.08)",
          opacity: intro,
          translate: `0px ${interpolate(intro, [0, 1], [30, 0])}px`,
        }}
      >
        <div
          style={{
            position: "absolute",
            inset: 0,
            scale: String(interpolate(frame, [0, durationInFrames], [1, 1.03], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })),
          }}
        >
          <Img src={staticFile(`${demoId}/${artifact.image}`)} style={{ display: "block", width: rect.w, height: rect.h }} />
          {highlight ? (
            <BoxMark left={highlight.x * rect.w} top={highlight.y * rect.h} width={highlight.w * rect.w} height={highlight.h * rect.h} />
          ) : null}
        </div>
      </div>
    </AbsoluteFill>
  );
};
