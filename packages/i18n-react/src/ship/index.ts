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
 * `'ai'` (shippable but unverified); a verified locale carries no badge. Dev-only
 * concerns (qps/pseudo locales) are sample-side and deliberately not modeled
 * here. A React hook wrapping the loader lives in `./react`.
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

/** An input locale: a bare code, or a code with a display label. */
export interface LocaleInput {
  locale: string;
  label?: string;
}

/** The only badge this layer emits, or none. */
export type LocaleBadge = "ai" | null;

/** One entry in the picker render model. */
export interface PickerLocale {
  locale: string;
  /** Display label (the caller's label, else the locale code). */
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
 * Build the picker render model from the manifest and the app's locale list —
 * the headless core, safe to call on the server or client (no React).
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
): PickerLocale[] {
  const empty = Object.keys(status).length === 0;
  const out: PickerLocale[] = [];
  for (const item of locales) {
    const locale = typeof item === "string" ? item : item.locale;
    const label = typeof item === "string" ? item : (item.label ?? item.locale);
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
