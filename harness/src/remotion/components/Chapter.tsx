import React from "react";
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { theme } from "./theme.ts";
import { CHAPTER_FS, CHAPTER_LH, type SceneLayout } from "./layout.ts";

/**
 * The one authored line of a scene, above its window: the consequence of the
 * beat. The spoken words run as captions in the lower third; this line says
 * what they add up to.
 */
export const ChapterLine: React.FC<{ text: string; layout: SceneLayout }> = ({ text, layout }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const on = spring({ frame: frame - 2, fps, config: { damping: 200 } });
  return (
    <div
      style={{
        position: "absolute",
        left: layout.left,
        top: layout.chapterTop,
        width: layout.width,
        fontFamily: theme.fontSans,
        fontSize: CHAPTER_FS,
        lineHeight: CHAPTER_LH,
        fontWeight: 600,
        color: theme.text,
        whiteSpace: "nowrap",
        overflow: "hidden",
        textOverflow: "ellipsis",
        opacity: on,
        translate: `0px ${interpolate(on, [0, 1], [8, 0])}px`,
      }}
    >
      {text}
    </div>
  );
};
