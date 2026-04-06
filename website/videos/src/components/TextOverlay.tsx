import React from "react";
import { interpolate, useCurrentFrame, useVideoConfig } from "remotion";
import type { Overlay } from "../schema";

interface TextOverlayProps {
  overlay: Overlay;
}

const positionStyles: Record<string, React.CSSProperties> = {
  "top-left": { top: 40, left: 40 },
  "top-center": { top: 40, left: "50%", transform: "translateX(-50%)" },
  "top-right": { top: 40, right: 40 },
  "bottom-left": { bottom: 60, left: 40 },
  "bottom-center": { bottom: 60, left: "50%", transform: "translateX(-50%)" },
  "bottom-right": { bottom: 60, right: 40 },
};

const styleConfig: Record<
  string,
  { bg: string; color: string; fontSize: number; padding: string }
> = {
  caption: {
    bg: "rgba(0, 0, 0, 0.75)",
    color: "#ffffff",
    fontSize: 28,
    padding: "12px 28px",
  },
  highlight: {
    bg: "rgba(99, 102, 241, 0.9)",
    color: "#ffffff",
    fontSize: 28,
    padding: "12px 28px",
  },
  "step-number": {
    bg: "rgba(99, 102, 241, 0.95)",
    color: "#ffffff",
    fontSize: 24,
    padding: "8px 20px",
  },
};

export const TextOverlay: React.FC<TextOverlayProps> = ({ overlay }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const startFrame = Math.round(overlay.startAt * fps);
  const durationFrames = Math.round(overlay.duration * fps);
  const endFrame = startFrame + durationFrames;

  // Not visible yet or already done
  if (frame < startFrame || frame >= endFrame) {
    return null;
  }

  const localFrame = frame - startFrame;
  const fadeFrames = 8;

  // Fade in over first 8 frames
  const fadeIn = interpolate(localFrame, [0, fadeFrames], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  // Fade out over last 8 frames
  const fadeOut = interpolate(localFrame, [durationFrames - fadeFrames, durationFrames], [1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  const opacity = Math.min(fadeIn, fadeOut);

  const pos = positionStyles[overlay.position] ?? positionStyles["bottom-center"];
  const style = styleConfig[overlay.style] ?? styleConfig.caption;

  return (
    <div
      style={{
        position: "absolute",
        ...pos,
        opacity,
        backgroundColor: style.bg,
        color: style.color,
        fontSize: style.fontSize,
        padding: style.padding,
        borderRadius: 8,
        fontFamily: "Inter, system-ui, sans-serif",
        fontWeight: 500,
        maxWidth: "80%",
        textAlign: "center",
        whiteSpace: "nowrap",
        zIndex: 10,
      }}
    >
      {overlay.text}
    </div>
  );
};
