/**
 * The presentation half of a demo, written from demo.yaml into
 * public/<id>/beats[-<locale>].json before every render (and for Studio), so
 * a change to a beat's caption, crop, zoom, highlight or hold reaches the
 * composition without a new narration. See BeatsFile in types.ts.
 */
import fs from "node:fs";
import path from "node:path";
import type { BeatHighlight, BeatsFile, DemoManifest, ScenePresentation, ZoomRect } from "../types.ts";
import { cardsFor } from "../lib/cards.ts";
import { isDefaultLocale, localeSuffix, resolveLocale } from "../lib/locale.ts";
import { ensureDir, publicDemoDir } from "../lib/paths.ts";

/** A crop's box form, or nothing for a selector crop (the recorder resolves those). */
function cropBox(crop: DemoManifest["narration"][number]["crop"]): ZoomRect | undefined {
  if (!crop || crop.x === undefined || crop.y === undefined || crop.w === undefined || crop.h === undefined) return undefined;
  return { x: crop.x, y: crop.y, w: crop.w, h: crop.h };
}

function highlightOf(h: DemoManifest["narration"][number]["highlight"]): BeatHighlight | undefined {
  if (!h) return undefined;
  return typeof h === "string" ? { text: h } : h;
}

/** The beats file for a demo in a locale, with the locale's sidecar text where it has one. */
export function beatsFor(m: DemoManifest, locale: string = "en"): BeatsFile {
  const overlay = isDefaultLocale(locale) ? undefined : m.locales?.[locale];
  const cards = cardsFor({ title: overlay?.title ?? m.title, subtitle: overlay?.subtitle ?? m.subtitle, brand: m.brand, outro: m.outro });
  const captionOverride = new Map((overlay?.narration ?? []).map((n) => [n.id, n.caption] as const));
  const scenes: ScenePresentation[] = m.narration.map((n) => {
    const caption = (captionOverride.get(n.id) ?? n.caption)?.trim();
    const out: ScenePresentation = { id: n.id, kind: n.kind };
    if (caption) out.caption = caption;
    if (n.artifact) out.artifact = n.artifact;
    if (n.beat) out.beat = n.beat;
    if (n.hold !== undefined) out.hold = n.hold;
    const crop = cropBox(n.crop);
    if (crop) out.crop = crop;
    if (n.zoom !== undefined) out.zoom = n.zoom;
    const highlight = highlightOf(n.highlight);
    if (highlight) out.highlight = highlight;
    if (n.through !== undefined) out.through = n.through;
    return out;
  });
  const file: BeatsFile = { id: m.id, cards, scenes };
  if (!isDefaultLocale(locale)) file.locale = locale;
  return file;
}

/** Write public/<id>/beats[-<locale>].json; returns its path. */
export function writeBeats(m: DemoManifest, locale?: string): string {
  const loc = resolveLocale(locale);
  const file = path.join(ensureDir(publicDemoDir(m.id)), `beats${localeSuffix(loc)}.json`);
  fs.writeFileSync(file, JSON.stringify(beatsFor(m, loc), null, 2));
  return file;
}
