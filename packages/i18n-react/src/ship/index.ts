/**
 * @neokapi/i18n-react/ship — ship-aware language picker helpers.
 *
 * `kapi status --ship --emit ship.json` writes a minimal manifest keyed by
 * locale, each entry `{ shippable, verified }` — the two-gate model. `shippable`
 * means the locale cleared its ship gate (safe to offer); `verified` means it
 * also cleared its verified gate (a person reviewed or signed off). A locale
 * that ships but is not verified is AI-only work.
 *
 * These helpers are framework-light and dependency-free: a loader that tolerates
 * a missing manifest, and a pure transform that turns the manifest plus the
 * app's locale list into a render model. The ONLY badge this layer emits is
 * `'ai'` (shippable but unverified); a verified locale carries no badge.
 *
 * Callers pass locale CODES; display labels are derived automatically from each
 * code as its endonym — the language named in its own language (fr → "Français",
 * ja → "日本語") — via `Intl.DisplayNames`, so no per-locale label table is
 * needed. An explicit `label` on a `LocaleInput` still wins, and an `labels`
 * override map names locales `Intl` cannot (e.g. a pseudo-locale `qps`). A React
 * hook wrapping the loader lives in `./react`.
 */

/** One locale's standing in the manifest. */
export interface ShipEntry {
  /** Cleared its ship gate — safe to offer in the picker. */
  shippable: boolean;
  /** Cleared its verified gate — human-reviewed, so no AI badge. */
  verified: boolean;
}

/** The ship.json manifest: locale → standing. */
export type ShipStatus = Record<string, ShipEntry>;

/**
 * An input locale: a bare code, or a code with an explicit display label. Omit
 * `label` to have it derived from the code via `Intl.DisplayNames`; supply it to
 * override the derived endonym.
 */
export interface LocaleInput {
  locale: string;
  label?: string;
}

/** Options for {@link languagePickerModel}. */
export interface LanguagePickerOptions {
  /**
   * Display-label overrides keyed by locale code, for locales `Intl.DisplayNames`
   * cannot name — non-BCP-47 or special codes such as a pseudo-locale
   * (`{ qps: "Pseudo English" }`). An override wins over the derived endonym; an
   * explicit `label` on a {@link LocaleInput} still wins over the override.
   */
  labels?: Record<string, string>;
}

/** The only badge this layer emits, or none. */
export type LocaleBadge = "ai" | null;

/** One entry in the picker render model. */
export interface PickerLocale {
  locale: string;
  /**
   * Display label, resolved in this order: an explicit `LocaleInput.label`, an
   * `options.labels` override, the `Intl.DisplayNames` endonym for the code, and
   * finally the raw code.
   */
  label: string;
  /** Always true — non-shippable locales are filtered out of the model. */
  shippable: boolean;
  /** `'ai'` when shippable but unverified; `null` when verified (no badge). */
  badge: LocaleBadge;
}

/**
 * Load the ship manifest, defaulting to `/ship.json`. Tolerant of a missing or
 * malformed file: any failure (no `fetch`, network error, non-2xx, unparseable
 * body, wrong shape) resolves to `{}` so the picker falls back to showing every
 * locale unbadged rather than breaking the page.
 */
export async function loadShipStatus(url = "/ship.json"): Promise<ShipStatus> {
  if (typeof fetch !== "function") return {};
  try {
    const res = await fetch(url);
    if (!res.ok) return {};
    const data: unknown = await res.json();
    return normalizeShipStatus(data);
  } catch {
    return {};
  }
}

/** Coerce arbitrary JSON into a ShipStatus, dropping anything malformed. */
function normalizeShipStatus(data: unknown): ShipStatus {
  if (data === null || typeof data !== "object") return {};
  const out: ShipStatus = {};
  for (const [locale, raw] of Object.entries(data as Record<string, unknown>)) {
    if (raw === null || typeof raw !== "object") continue;
    const e = raw as Record<string, unknown>;
    out[locale] = { shippable: e.shippable === true, verified: e.verified === true };
  }
  return out;
}

/**
 * Uppercase the first character with the locale's own casing rules (so a Turkish
 * `i` becomes `İ`, not `I`). Codepoint-aware, so it is safe on astral-plane
 * scripts, and a no-op for scripts without case (e.g. Japanese, Arabic).
 */
function capitalizeFirst(text: string, locale: string): string {
  const chars = Array.from(text);
  if (chars.length === 0) return text;
  chars[0] = chars[0].toLocaleUpperCase(locale);
  return chars.join("");
}

/**
 * Derive a locale's endonym — its name in its own language (fr → "Français",
 * ja → "日本語") — via `Intl.DisplayNames`, with the first letter capitalized (some
 * endonyms are lowercase in their own orthography, e.g. "français"; a picker
 * reads better with menu-style capitalization). Returns `undefined` — never
 * throws — when the runtime lacks `Intl.DisplayNames`, the code is structurally
 * invalid, or the API has no name for it and echoes the code back (e.g. a
 * pseudo-locale like `qps`), so the caller can fall through to the next label
 * source.
 */
function deriveEndonym(locale: string): string | undefined {
  try {
    const name = new Intl.DisplayNames([locale], { type: "language" }).of(locale);
    // `Intl.DisplayNames` echoes the code back when it has no name for it.
    if (name === undefined || name === locale) return undefined;
    return capitalizeFirst(name, locale);
  } catch {
    return undefined;
  }
}

/**
 * Resolve a locale's display label. Precedence, highest first: an explicit
 * `LocaleInput.label`, an `options.labels` override, the `Intl.DisplayNames`
 * endonym, then the raw code.
 */
function resolveLabel(
  locale: string,
  explicit: string | undefined,
  overrides: Record<string, string> | undefined,
): string {
  if (explicit !== undefined) return explicit;
  const override = overrides?.[locale];
  if (override !== undefined) return override;
  return deriveEndonym(locale) ?? locale;
}

/**
 * Build the picker render model from the manifest and the app's locale list —
 * the headless core, safe to call on the server or client (no React).
 *
 * Pass locale CODES; each display label is derived automatically (see
 * {@link resolveLabel}) — an explicit `LocaleInput.label`, else an
 * `options.labels` override, else the `Intl.DisplayNames` endonym, else the code.
 *
 * Rules:
 *   - Empty manifest (missing/malformed ship.json) → every locale is shown
 *     unbadged; the picker cannot judge what it cannot see.
 *   - A locale with an entry that is not shippable is dropped from the model.
 *   - A shippable locale is badged `'ai'` when unverified, `null` when verified.
 *   - A locale with NO entry in a non-empty manifest (e.g. the source language,
 *     which ship.json — targets only — never lists) is shown unbadged.
 */
export function languagePickerModel(
  status: ShipStatus,
  locales: ReadonlyArray<string | LocaleInput>,
  options: LanguagePickerOptions = {},
): PickerLocale[] {
  const overrides = options.labels;
  const empty = Object.keys(status).length === 0;
  const out: PickerLocale[] = [];
  for (const item of locales) {
    const locale = typeof item === "string" ? item : item.locale;
    const explicit = typeof item === "string" ? undefined : item.label;
    const label = resolveLabel(locale, explicit, overrides);
    const entry = empty ? undefined : status[locale];
    if (entry) {
      if (!entry.shippable) continue; // gated out of the picker
      out.push({ locale, label, shippable: true, badge: entry.verified ? null : "ai" });
    } else {
      // No manifest, or no entry for this locale — show it unbadged.
      out.push({ locale, label, shippable: true, badge: null });
    }
  }
  return out;
}
