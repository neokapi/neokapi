import React, { useMemo } from "react";
import { Sequence, useCurrentFrame, useVideoConfig } from "remotion";
import { createTikTokStyleCaptions, type TikTokPage } from "@remotion/captions";
import type { CaptionsFile } from "../../types.ts";
import type { SceneTiming } from "../timeline.ts";
import { theme } from "./theme.ts";
import { CAPTION_FS, CAPTION_LH, CAPTION_PAD_X, CAPTION_PAD_Y, CAPTION_PAGE_MS, sceneLayout } from "./layout.ts";

/** The spoken word, in the always-dark pill; the word being said, in this. */
const CAPTION_TEXT = "#f4f7ff";
const CAPTION_ACTIVE = "#ffc46b";

/** How long the last page of a scene lingers after its last word, in ms. */
const PAGE_TAIL_MS = 600;

export interface CaptionPageSlot {
  sceneId: string;
  from: number;
  durationInFrames: number;
  page: TikTokPage;
}

/**
 * Lay a video's caption pages onto the composition timeline: each scene's
 * captions are paged on their own, from that scene's start frame, so a page
 * never straddles a cut. A page stays up until the next page of its scene;
 * the last page lingers a moment after its last word and never past the
 * scene's own narration end.
 */
export function layoutCaptionPages(scenes: SceneTiming[], captions: CaptionsFile | null, fps: number): CaptionPageSlot[] {
  if (!captions) return [];
  const byId = new Map(captions.scenes.map((s) => [s.id, s.captions] as const));
  const slots: CaptionPageSlot[] = [];
  for (const scene of scenes) {
    const list = byId.get(scene.id);
    if (!list || list.length === 0) continue;
    const { pages } = createTikTokStyleCaptions({ captions: list, combineTokensWithinMilliseconds: CAPTION_PAGE_MS });
    const sceneEnd = scene.from + scene.durationFrames - scene.transitionAfter;
    pages.forEach((page, i) => {
      const next = pages[i + 1];
      const start = scene.from + Math.round((page.startMs / 1000) * fps);
      const lastWordEnd = page.tokens.reduce((m, t) => Math.max(m, t.toMs), page.startMs + page.durationMs);
      const end = next
        ? scene.from + Math.round((next.startMs / 1000) * fps)
        : Math.min(sceneEnd, scene.from + Math.round(((lastWordEnd + PAGE_TAIL_MS) / 1000) * fps));
      const durationInFrames = end - start;
      if (durationInFrames <= 0) return;
      slots.push({ sceneId: scene.id, from: start, durationInFrames, page });
    });
  }
  return slots;
}

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
