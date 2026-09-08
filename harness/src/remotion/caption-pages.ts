/**
 * Where a video's caption pages sit on the composition timeline, as
 * arithmetic the Captions component and the tests share.
 */
import { createTikTokStyleCaptions, type TikTokPage } from "@remotion/captions";
import type { CaptionsFile } from "../types.ts";
import type { SceneTiming } from "./timeline.ts";

/** Milliseconds a caption page combines tokens over. */
export const CAPTION_PAGE_MS = 1200;

/** How long the last page of a scene lingers after its last word, in ms. */
const PAGE_TAIL_MS = 600;

export interface CaptionPageSlot {
  sceneId: string;
  from: number;
  durationInFrames: number;
  page: TikTokPage;
}

/**
 * Each scene's captions are paged on their own, from that scene's start
 * frame, so a page never straddles a cut. A page stays up until the next page
 * of its scene; the last page lingers a moment after its last word and never
 * past the scene's own narration end.
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
