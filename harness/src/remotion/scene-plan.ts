import { fade } from "@remotion/transitions/fade";
import { slide } from "@remotion/transitions/slide";
import type { TransitionPresentation } from "@remotion/transitions";
import type { BeatHighlight, BeatsFile, CardsSpec, DemoCapture, NarrationManifest, NarrationScene, ZoomRect } from "../types.ts";
import type { TimedScene } from "./timeline.ts";
import { TRANSITION_FRAMES } from "./components/layout.ts";
import { cardsFor } from "../lib/cards.ts";

/** A scene as the composition sees it: the narration's timing and the manifest's picture. */
export type SceneSpec = TimedScene & {
  caption: string;
  artifact?: string;
  beat?: string;
  crop?: ZoomRect;
  zoom?: number;
  highlight?: BeatHighlight;
  through?: number;
  hold?: number;
};

/**
 * Join the two halves of a demo: narration.json (what the audio knows: order
 * of spoken scenes, durations, audio paths) and beats.json (what demo.yaml
 * says about the picture). beats.json is written from the manifest on every
 * run, so it leads; a scene it lists that the narration has not synthesized
 * shows silent at its floor length. Without a beats.json the narration's
 * own copy of the picture fields is used.
 */
export function mergeScenes(narration: NarrationManifest, beats: BeatsFile | null): SceneSpec[] {
  if (!beats) return narration.scenes.map((s) => ({ ...s, caption: s.caption ?? "" }));
  const byId = new Map(narration.scenes.map((s) => [s.id, s] as const));
  return beats.scenes.map((p) => {
    const n: NarrationScene = byId.get(p.id) ?? { id: p.id, kind: p.kind, text: "", caption: "", durationSec: 0, holdSec: 0 };
    return {
      ...n,
      kind: p.kind,
      caption: p.caption ?? "",
      artifact: p.artifact,
      beat: p.beat,
      crop: p.crop,
      zoom: p.zoom,
      highlight: p.highlight,
      through: p.through,
      hold: p.hold,
    };
  });
}

/** The cards' text: from beats.json, else from the capture's copy of the manifest. */
export function cardsOf(beats: BeatsFile | null, capture: DemoCapture): CardsSpec {
  return beats?.cards ?? cardsFor({ title: capture.title, subtitle: capture.subtitle, brand: capture.brand });
}

/**
 * Frames two adjacent scenes overlap. A terminal following a terminal, or a
 * desktop beat following a desktop beat, is one continuous take and cuts;
 * every other boundary is a short transition.
 */
export function transitionFrames(a: TimedScene, b: TimedScene): number {
  if (a.kind === b.kind && (a.kind === "terminal" || a.kind === "desktop")) return 0;
  return TRANSITION_FRAMES;
}

export type Presentation = TransitionPresentation<Record<string, unknown>>;

/** The presentation on a boundary: a slide between artifact spotlights, a fade otherwise. */
export function presentationBetween(a: TimedScene, b: TimedScene): Presentation | undefined {
  if (transitionFrames(a, b) === 0) return undefined;
  if (a.kind === "artifact" && b.kind === "artifact") return slide({ direction: "from-right" }) as Presentation;
  return fade() as Presentation;
}

/** The strings a terminal scene marks, from its highlight. */
export function highlightText(h: BeatHighlight | undefined): string[] | undefined {
  if (!h?.text) return undefined;
  return Array.isArray(h.text) ? h.text : [h.text];
}
