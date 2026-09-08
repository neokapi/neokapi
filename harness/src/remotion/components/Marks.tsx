import React from "react";
import { Easing, interpolate, useCurrentFrame, useVideoConfig } from "remotion";
import { Box, Highlight } from "@remotion/rough-notation";
import { theme } from "./theme.ts";

/**
 * How far a hand-drawn mark has been drawn at this frame of its scene: it
 * starts a third of a second in, once the eye has landed, and takes two
 * thirds of a second to draw.
 */
export function markProgress(frame: number, fps: number): number {
  return interpolate(frame, [Math.round(0.3 * fps), Math.round(1.0 * fps)], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.bezier(0.16, 1, 0.3, 1),
  });
}

/** Whether a terminal line is one the scene asked to mark. */
export function lineMatches(line: string, needles: string[] | undefined): boolean {
  if (!needles || needles.length === 0 || !line) return false;
  return needles.some((n) => n.length > 0 && line.includes(n));
}

/**
 * A terminal line with a marker highlight drawn behind it when `on`. The
 * annotation's own wrapper never wraps text, so the line is given its width
 * back explicitly and wraps inside the mark like its neighbours.
 */
export const LineMark: React.FC<{ on: boolean; width: number; children: React.ReactNode }> = ({ on, width, children }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  if (!on) return <>{children}</>;
  return (
    <Highlight
      progress={markProgress(frame, fps)}
      color={theme.markFill}
      padding={{ left: 8, right: 8, top: 2, bottom: 2 }}
      iterations={1}
      roughness={1.2}
    >
      <div style={{ width, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>{children}</div>
    </Highlight>
  );
};

/** A hand-drawn box around a region of the element it sits in, in pixels. */
export const BoxMark: React.FC<{ left: number; top: number; width: number; height: number }> = ({ left, top, width, height }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  return (
    <div style={{ position: "absolute", left, top, width, height, pointerEvents: "none" }}>
      <Box
        progress={markProgress(frame, fps)}
        color={theme.markStroke}
        strokeWidth={6}
        iterations={2}
        roughness={1.2}
        padding={{ left: 10, right: 10, top: 10, bottom: 10 }}
      >
        <div style={{ width, height }} />
      </Box>
    </div>
  );
};
