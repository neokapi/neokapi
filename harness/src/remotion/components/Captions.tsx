import React, { useMemo } from "react";
import { Sequence, useCurrentFrame, useVideoConfig } from "remotion";
import type { TikTokPage } from "@remotion/captions";
import type { CaptionsFile } from "../../types.ts";
import type { SceneTiming } from "../timeline.ts";
import { layoutCaptionPages } from "../caption-pages.ts";
import { theme } from "./theme.ts";
import { CAPTION_FS, CAPTION_LH, CAPTION_PAD_X, CAPTION_PAD_Y, sceneLayout } from "./layout.ts";

/** The spoken word, in the always-dark pill; the word being said, in this. */
const CAPTION_TEXT = "#f4f7ff";
const CAPTION_ACTIVE = "#ffc46b";

const CaptionPage: React.FC<{ page: TikTokPage }> = ({ page }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const nowMs = page.startMs + (frame / fps) * 1000;
  const lay = sceneLayout(false);
  return (
    <div
      style={{
        position: "absolute",
        left: lay.left,
        width: lay.width,
        bottom: 1080 - lay.captionBottom,
        display: "flex",
        justifyContent: "center",
        alignItems: "flex-end",
        pointerEvents: "none",
      }}
    >
      <div
        style={{
          maxWidth: lay.width,
          padding: `${CAPTION_PAD_Y}px ${CAPTION_PAD_X}px`,
          background: "rgba(8,11,19,0.92)",
          border: "1px solid rgba(255,255,255,0.14)",
          borderRadius: 16,
          color: CAPTION_TEXT,
          fontFamily: theme.fontSans,
          fontSize: CAPTION_FS,
          lineHeight: CAPTION_LH,
          fontWeight: 600,
          textAlign: "center",
          whiteSpace: "pre-wrap",
        }}
      >
        {page.tokens.map((token, i) => {
          const active = token.fromMs <= nowMs && token.toMs > nowMs;
          const text = i === 0 ? token.text.replace(/^\s+/, "") : token.text;
          return (
            <span key={`${token.fromMs}-${i}`} style={{ color: active ? CAPTION_ACTIVE : CAPTION_TEXT }}>
              {text}
            </span>
          );
        })}
      </div>
    </div>
  );
};

/**
 * The lower-third captions of a whole video, one component over every scene.
 * Put the captioning logic here and nowhere else.
 */
export const Captions: React.FC<{ scenes: SceneTiming[]; captions: CaptionsFile | null }> = ({ scenes, captions }) => {
  const { fps } = useVideoConfig();
  const slots = useMemo(() => layoutCaptionPages(scenes, captions, fps), [scenes, captions, fps]);
  return (
    <>
      {slots.map((s, i) => (
        <Sequence key={`${s.sceneId}-${i}`} from={s.from} durationInFrames={s.durationInFrames} layout="none" name={`caption:${s.sceneId}`}>
          <CaptionPage page={s.page} />
        </Sequence>
      ))}
    </>
  );
};
