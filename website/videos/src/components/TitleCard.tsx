import React from "react";
import {
  AbsoluteFill,
  Img,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import type { Branding, TitleCardSceneType } from "../schema";

interface TitleCardProps {
  scene: TitleCardSceneType;
  branding: Branding;
  durationInFrames: number;
}

const styleColors: Record<string, { bg: string; text: string }> = {
  branded: { bg: "", text: "#ffffff" },
  minimal: { bg: "#ffffff", text: "#111111" },
  dark: { bg: "#0a0a0a", text: "#ffffff" },
  light: { bg: "#fafafa", text: "#111111" },
};

export const TitleCard: React.FC<TitleCardProps> = ({
  scene,
  branding,
  durationInFrames,
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const colors = styleColors[scene.style] ?? styleColors.branded;
  const bg = colors.bg || branding.backgroundColor;

  // Enter animation: scale + opacity via spring
  const enterProgress = spring({
    frame,
    fps,
    config: { damping: 15, stiffness: 80, mass: 0.8 },
  });

  // Exit: fade out over last 10 frames
  const exitStart = durationInFrames - 10;
  const exitOpacity =
    frame >= exitStart ? 1 - (frame - exitStart) / 10 : 1;

  const scale = 0.85 + enterProgress * 0.15;
  const opacity = enterProgress * Math.max(0, exitOpacity);

  return (
    <AbsoluteFill
      style={{
        backgroundColor: bg,
        justifyContent: "center",
        alignItems: "center",
        fontFamily: branding.fontFamily,
      }}
    >
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          gap: 24,
          transform: `scale(${scale})`,
          opacity,
        }}
      >
        {branding.logo && (
          <Img
            src={staticFile(branding.logo)}
            style={{ width: 80, height: 80, marginBottom: 16 }}
          />
        )}
        <h1
          style={{
            fontSize: 72,
            fontWeight: 700,
            color: colors.text,
            margin: 0,
            textAlign: "center",
            lineHeight: 1.1,
          }}
        >
          {scene.heading}
        </h1>
        {scene.subheading && (
          <p
            style={{
              fontSize: 32,
              fontWeight: 400,
              color: colors.text,
              opacity: 0.7,
              margin: 0,
              textAlign: "center",
            }}
          >
            {scene.subheading}
          </p>
        )}
      </div>
    </AbsoluteFill>
  );
};
