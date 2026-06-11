/**
 * Locale dimension of the harness — see DemoManifest.locales in src/types.ts.
 *
 * The English narration in demo.yaml is the master; a `locales:` overlay
 * re-voices scenes per locale. The default locale ("en") is fully backwards
 * compatible: no suffixes anywhere, byte-identical behavior to the
 * pre-locale harness. Non-default locales suffix every derived artifact:
 *
 *   narration-<locale>.json + audio-<locale>/   (narrate)
 *   out/<id>-<locale>[-light].mp4               (render)
 *   <publishAs>-<locale>-{light,dark}.webm/jpg  (publish)
 */
import type { DemoManifest } from "../types.ts";

/** The authoring locale of demo.yaml narration. Unsuffixed everywhere. */
export const DEFAULT_LOCALE = "en";

/** Resolve the active locale: explicit arg > HARNESS_LOCALE env > "en". */
export function resolveLocale(explicit?: string): string {
  const l = (explicit || process.env.HARNESS_LOCALE || DEFAULT_LOCALE).trim();
  if (!/^[a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})*$/.test(l)) {
    throw new Error(`invalid locale "${l}" (expected a BCP-47 tag like "nb" or "pt-BR")`);
  }
  return l;
}

export function isDefaultLocale(locale: string): boolean {
  return locale.toLowerCase() === DEFAULT_LOCALE;
}

/** Filename suffix for a locale: "" for en, "-nb" for nb, … */
export function localeSuffix(locale: string): string {
  return isDefaultLocale(locale) ? "" : `-${locale}`;
}

/** English display name of the locale's language, for TTS style prompts. */
export function languageNameFor(locale: string): string {
  // Pin the names the TTS prompts depend on; Intl covers the long tail.
  const pinned: Record<string, string> = {
    en: "British English",
    nb: "Norwegian Bokmål",
    nn: "Norwegian Nynorsk",
  };
  const base = locale.toLowerCase();
  if (pinned[base]) return pinned[base];
  try {
    return new Intl.DisplayNames(["en"], { type: "language" }).of(locale) ?? locale;
  } catch {
    return locale;
  }
}

/**
 * Apply a locale overlay to a manifest: returns a copy whose narration
 * carries the locale's text/caption for every overridden scene. For the
 * default locale the manifest is returned unchanged (no copy).
 *
 * Validation:
 *  - every override must reference an existing narration scene id;
 *  - published demos (publishAs set) must cover EVERY spoken scene, so a
 *    shipped video never mixes languages mid-narration.
 */
export function localizeManifest(m: DemoManifest, locale: string): DemoManifest {
  if (isDefaultLocale(locale)) return m;
  const overlay = m.locales?.[locale];
  if (!overlay) {
    throw new Error(`demo ${m.id}: no "locales.${locale}" narration overlay in demo.yaml`);
  }
  const byId = new Map(overlay.narration.map((o) => [o.id, o] as const));
  for (const o of overlay.narration) {
    if (!m.narration.find((s) => s.id === o.id)) {
      throw new Error(`demo ${m.id}: locales.${locale} overrides unknown scene "${o.id}"`);
    }
    if (!o.text?.trim()) {
      throw new Error(`demo ${m.id}: locales.${locale} scene "${o.id}" has empty text`);
    }
  }
  if (m.publishAs) {
    const missing = m.narration.filter((s) => s.text?.trim() && !byId.has(s.id)).map((s) => s.id);
    if (missing.length > 0) {
      throw new Error(
        `demo ${m.id}: published demos require full ${locale} narration coverage — missing scene(s): ${missing.join(", ")}`,
      );
    }
  }
  return {
    ...m,
    narration: m.narration.map((s) => {
      const o = byId.get(s.id);
      return o ? { ...s, text: o.text, caption: o.caption ?? undefined } : s;
    }),
  };
}
