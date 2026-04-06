import React from "react";
import { AbsoluteFill, OffthreadVideo, staticFile } from "remotion";
import type { Branding, RecordingSceneType } from "../schema";
import { BrandedFrame } from "./BrandedFrame";
import { TerminalFrame } from "./TerminalFrame";
import { TextOverlay } from "./TextOverlay";

interface RecordingSceneProps {
  scene: RecordingSceneType;
  branding: Branding;
}

export const RecordingScene: React.FC<RecordingSceneProps> = ({ scene, branding }) => {
  const src = staticFile(`raw/${scene.source}`);
  const startFrom = scene.trim?.start ? Math.round(scene.trim.start * 30) : undefined;
  const playbackRate = scene.playbackRate;

  const video = (
    <OffthreadVideo
      src={src}
      startFrom={startFrom}
      playbackRate={playbackRate}
      style={{
        width: "100%",
        height: "100%",
        objectFit: "contain",
      }}
    />
  );

  const frameType = scene.frame?.type ?? "none";

  let framedContent: React.ReactNode;
  if (frameType === "desktop") {
    framedContent = (
      <BrandedFrame title={scene.frame?.title} branding={branding}>
        {video}
      </BrandedFrame>
    );
  } else if (frameType === "terminal") {
    framedContent = (
      <TerminalFrame title={scene.frame?.title} branding={branding}>
        {video}
      </TerminalFrame>
    );
  } else {
    framedContent = video;
  }

  return (
    <AbsoluteFill>
      {framedContent}
      {scene.overlays?.map((overlay, i) => (
        <TextOverlay key={i} overlay={overlay} />
      ))}
    </AbsoluteFill>
  );
};
