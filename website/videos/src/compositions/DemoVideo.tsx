import React from "react";
import {
  AbsoluteFill,
  interpolate,
  Sequence,
  useCurrentFrame,
} from "remotion";
import type { ResolvedScript } from "../schema";
import { TitleCard } from "../components/TitleCard";
import { RecordingScene } from "../components/RecordingScene";

export interface DemoVideoProps {
  script: ResolvedScript;
}

/**
 * FadeBlack renders a full-screen black overlay with opacity animation.
 */
const FadeBlack: React.FC<{ durationInFrames: number }> = ({
  durationInFrames,
}) => {
  const frame = useCurrentFrame();
  const mid = durationInFrames / 2;
  const opacity = interpolate(
    frame,
    [0, mid, durationInFrames],
    [0, 1, 0],
    { extrapolateLeft: "clamp", extrapolateRight: "clamp" }
  );

  return (
    <AbsoluteFill style={{ backgroundColor: "black", opacity }} />
  );
};

/**
 * Crossfade: renders the previous and next scenes with blending.
 * Since we don't have access to prev/next in Series flow,
 * we render a simple fade-to-black-and-back.
 */
const Crossfade: React.FC<{ durationInFrames: number }> = ({
  durationInFrames,
}) => {
  return <FadeBlack durationInFrames={durationInFrames} />;
};

export const DemoVideo: React.FC<DemoVideoProps> = ({ script }) => {
  let frameOffset = 0;

  return (
    <AbsoluteFill style={{ backgroundColor: script.branding.backgroundColor }}>
      {script.scenes.map((resolved, i) => {
        const { scene, durationInFrames } = resolved;
        const from = frameOffset;
        frameOffset += durationInFrames;

        return (
          <Sequence
            key={i}
            from={from}
            durationInFrames={durationInFrames}
          >
            {scene.type === "title-card" && (
              <TitleCard
                scene={scene}
                branding={script.branding}
                durationInFrames={durationInFrames}
              />
            )}
            {scene.type === "recording" && (
              <RecordingScene scene={scene} branding={script.branding} />
            )}
            {scene.type === "transition" && scene.effect === "fade-black" && (
              <FadeBlack durationInFrames={durationInFrames} />
            )}
            {scene.type === "transition" && scene.effect === "crossfade" && (
              <Crossfade durationInFrames={durationInFrames} />
            )}
            {scene.type === "transition" && scene.effect === "wipe-left" && (
              <FadeBlack durationInFrames={durationInFrames} />
            )}
          </Sequence>
        );
      })}
    </AbsoluteFill>
  );
};
