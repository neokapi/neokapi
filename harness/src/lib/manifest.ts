import fs from "node:fs";
import path from "node:path";
import YAML from "yaml";
import type { DemoLocaleOverlay, DemoManifest, NarrationSpec, ZoomRect } from "../types.ts";
import { DEMOS_DIR } from "./paths.ts";

/** A generated localized sidecar next to demo.yaml: demo.<bcp47>.yaml. */
const LOCALE_SIDECAR_RE = /^demo\.([a-zA-Z]{2,3}(?:-[a-zA-Z0-9]{2,8})*)\.yaml$/;

/**
 * Load the generated demo.<locale>.yaml sidecars for a demo into locale
 * overlays (see DemoManifest.locales). A sidecar is a full localized copy of
 * demo.yaml produced by the dogfood l10n pipeline (`make l10n`); only
 * the localized narration fields are lifted into the overlay:
 *
 *  - a scene whose text is still identical to the English master is a content memory
 *    miss (pending translation) and is skipped — EN fallback, not an error;
 *  - a scene id the master no longer has is source drift: warn and skip, so
 *    a stale sidecar never blocks loading (regenerate via `make l10n`);
 *  - title/subtitle ride along when the sidecar carries a translation.
 */
function loadLocaleSidecars(id: string, m: DemoManifest, dir: string): Record<string, DemoLocaleOverlay> | undefined {
  const overlays: Record<string, DemoLocaleOverlay> = {};
  for (const entry of fs.readdirSync(dir)) {
    const match = LOCALE_SIDECAR_RE.exec(entry);
    if (!match) continue;
    const locale = match[1];
    const sidecar = YAML.parse(fs.readFileSync(path.join(dir, entry), "utf8")) as Partial<DemoManifest>;
    const overlay: DemoLocaleOverlay = { narration: [] };
    if (sidecar.title?.trim() && sidecar.title !== m.title) overlay.title = sidecar.title;
    if (sidecar.subtitle?.trim() && sidecar.subtitle !== m.subtitle) overlay.subtitle = sidecar.subtitle;
    for (const s of sidecar.narration ?? []) {
      if (!s?.id || !s.text?.trim()) continue;
      const master = m.narration.find((n) => n.id === s.id);
      if (!master) {
        console.warn(`demo ${id}: ${entry} carries scene "${s.id}" which demo.yaml no longer has — regenerate via 'make l10n'`);
        continue;
      }
      if (s.text === master.text) continue; // untranslated (Memory miss) — EN fallback
      const caption = s.caption && s.caption !== master.caption ? s.caption : undefined;
      overlay.narration.push({ id: s.id, text: s.text, ...(caption ? { caption } : {}) });
    }
    if (overlay.narration.length > 0 || overlay.title || overlay.subtitle) {
      overlays[locale] = overlay;
    }
  }
  return Object.keys(overlays).length > 0 ? overlays : undefined;
}

/**
 * List demo ids (directories under demos/ that contain a demo.yaml), sorted.
 * `demos/_retired/` holds its manifests one level deeper and so is skipped.
 * The root is a parameter only so tests can point at a fixture tree.
 */
export function listDemoIds(demosDir: string = DEMOS_DIR): string[] {
  if (!fs.existsSync(demosDir)) return [];
  return fs
    .readdirSync(demosDir, { withFileTypes: true })
    .filter((d) => d.isDirectory() && fs.existsSync(path.join(demosDir, d.name, "demo.yaml")))
    .map((d) => d.name)
    .sort();
}

/** A normalized [0,1] box: four numbers, inside the unit square, with area. */
function isUnitRect(v: unknown): v is ZoomRect {
  if (typeof v !== "object" || v === null) return false;
  const r = v as Record<string, unknown>;
  const num = (k: string) => typeof r[k] === "number" && Number.isFinite(r[k] as number);
  if (!num("x") || !num("y") || !num("w") || !num("h")) return false;
  const { x, y, w, h } = r as unknown as ZoomRect;
  return x >= 0 && y >= 0 && w > 0 && h > 0 && x + w <= 1.0001 && y + h <= 1.0001;
}

function isSelectorList(v: unknown): v is string | string[] {
  if (typeof v === "string") return v.trim().length > 0;
  return Array.isArray(v) && v.length > 0 && v.every((s) => typeof s === "string" && s.trim().length > 0);
}

/**
 * The per-beat picture fields (hold, crop, zoom, highlight, through), checked
 * against the scene kind they apply to so a typo fails the load rather than
 * rendering a video that quietly ignores it. A bare-string highlight is
 * normalized to its object form here, once, for every consumer.
 */
function validateBeatFields(n: NarrationSpec, where: string, ctx: { isShell: boolean; scriptSteps: number }): void {
  if (n.hold !== undefined && !(typeof n.hold === "number" && Number.isFinite(n.hold) && n.hold >= 0)) {
    throw new Error(`${where}: hold must be a number of seconds, 0 or more`);
  }
  if (n.crop !== undefined) {
    if (n.kind !== "desktop") throw new Error(`${where}: crop applies to desktop scenes only`);
    const c = n.crop as Record<string, unknown> | null;
    if (typeof c !== "object" || c === null) throw new Error(`${where}: crop must be a map with a selector or an x/y/w/h box`);
    const hasSelector = c.selector !== undefined;
    const hasBox = ["x", "y", "w", "h"].some((k) => c[k] !== undefined);
    if (hasSelector && hasBox) throw new Error(`${where}: crop takes a selector or a box, never both`);
    if (hasSelector && !isSelectorList(c.selector)) throw new Error(`${where}: crop.selector must be a non-empty selector or a list of them`);
    if (hasBox && !isUnitRect(c)) throw new Error(`${where}: crop box needs x, y, w and h in [0,1], inside the window, with w and h above 0`);
    if (!hasSelector && !hasBox) throw new Error(`${where}: crop needs a selector or an x/y/w/h box`);
  }
  if (n.zoom !== undefined) {
    if (n.kind !== "desktop") throw new Error(`${where}: zoom applies to desktop scenes only`);
    if (!(typeof n.zoom === "number" && Number.isFinite(n.zoom) && n.zoom >= 1 && n.zoom <= 3)) {
      throw new Error(`${where}: zoom must be a number from 1 to 3`);
    }
  }
  if (n.highlight !== undefined) {
    if (typeof n.highlight === "string") {
      if (!n.highlight.trim()) throw new Error(`${where}: highlight must name the text to mark`);
      n.highlight = { text: n.highlight };
    }
    const h = n.highlight as Record<string, unknown> | null;
    if (typeof h !== "object" || h === null) throw new Error(`${where}: highlight must be a string or a map with text, selector or box`);
    const keys = ["text", "selector", "box"].filter((k) => h[k] !== undefined);
    if (keys.length !== 1) throw new Error(`${where}: highlight takes exactly one of text, selector or box`);
    const [k] = keys;
    if (k === "text") {
      if (n.kind !== "terminal") throw new Error(`${where}: highlight.text applies to terminal scenes only`);
      if (!isSelectorList(h.text)) throw new Error(`${where}: highlight.text must be a non-empty string or a list of them`);
    } else if (k === "selector") {
      if (n.kind !== "desktop") throw new Error(`${where}: highlight.selector applies to desktop scenes only`);
      if (typeof h.selector !== "string" || !h.selector.trim()) throw new Error(`${where}: highlight.selector must be a non-empty selector`);
    } else {
      if (n.kind !== "desktop" && n.kind !== "artifact") throw new Error(`${where}: highlight.box applies to desktop and artifact scenes only`);
      if (!isUnitRect(h.box)) throw new Error(`${where}: highlight.box needs x, y, w and h in [0,1], inside the frame, with w and h above 0`);
    }
  }
  if (n.through !== undefined) {
    if (n.kind !== "terminal") throw new Error(`${where}: through applies to terminal scenes only`);
    if (!ctx.isShell) throw new Error(`${where}: through counts script steps, so it applies to scripted shell demos only`);
    if (!Number.isInteger(n.through) || n.through < 1 || n.through > ctx.scriptSteps) {
      throw new Error(`${where}: through must be a step number from 1 to ${ctx.scriptSteps} (the script has ${ctx.scriptSteps} steps)`);
    }
  }
}

export function loadManifest(id: string, demosDir: string = DEMOS_DIR): DemoManifest {
  const dir = path.join(demosDir, id);
  const file = path.join(dir, "demo.yaml");
  const raw = fs.readFileSync(file, "utf8");
  const m = YAML.parse(raw) as DemoManifest;
  if (!m.id) m.id = id;
  if (m.id !== id) {
    throw new Error(`demo.yaml id "${m.id}" does not match directory "${id}"`);
  }
  // Light validation so authoring mistakes fail fast. A "shell" demo is scripted
  // (commands in `script`), a "desktop" demo replays a recorded screencast — both
  // need no Claude `prompt`; a Claude demo needs one.
  const isShell = m.terminal === "shell" || (Array.isArray(m.script) && m.script.length > 0);
  const isDesktop = m.terminal === "desktop";
  if (!m.title) throw new Error(`demo ${id}: title is required`);
  if (isShell) {
    if (!Array.isArray(m.script) || m.script.length === 0) {
      throw new Error(`demo ${id}: terminal "shell" requires a non-empty "script"`);
    }
    m.script.forEach((s, n) => {
      if (s.expectExit === undefined) return;
      const codes = Array.isArray(s.expectExit) ? s.expectExit : [s.expectExit];
      if (codes.length === 0 || codes.some((c) => !Number.isInteger(c))) {
        throw new Error(
          `demo ${id}: script step ${n + 1} declares expectExit ${JSON.stringify(s.expectExit)} — give one exit code, or a non-empty list of them`,
        );
      }
    });
  } else if (!isDesktop && !m.prompt) {
    throw new Error(`demo ${id}: prompt is required (or set terminal: shell/desktop)`);
  }
  m.artifacts ??= [];
  m.narration ??= [];
  m.aspects ??= [];
  if (m.outro !== undefined) {
    if (typeof m.outro !== "object" || m.outro === null || Array.isArray(m.outro)) {
      throw new Error(`demo ${id}: outro must be a map with "line" and/or "pointer"`);
    }
    for (const k of ["line", "pointer"] as const) {
      const v = m.outro[k];
      if (v !== undefined && typeof v !== "string") throw new Error(`demo ${id}: outro.${k} must be a string`);
    }
  }
  const scriptSteps = Array.isArray(m.script) ? m.script.length : 0;
  let lastThrough = 0;
  for (const n of m.narration) {
    const where = `demo ${id}: narration scene "${n.id}"`;
    if (n.kind === "artifact" && !n.artifact) {
      throw new Error(`${where} is kind=artifact but has no artifact id`);
    }
    if (n.kind === "artifact" && !m.artifacts.find((a) => a.id === n.artifact)) {
      throw new Error(`${where} references unknown artifact "${n.artifact}"`);
    }
    if (n.kind === "desktop" && !n.beat) {
      throw new Error(`${where} is kind=desktop but has no beat id`);
    }
    validateBeatFields(n, where, { isShell, scriptSteps });
    if (n.through !== undefined) {
      if (n.through < lastThrough) {
        throw new Error(`${where} declares through: ${n.through}, earlier than a previous scene's ${lastThrough}; the reveal never runs backwards`);
      }
      lastThrough = n.through;
    }
  }
  // Locale overlays are GENERATED, never authored: an inline `locales:` block
  // is hand-maintained target-language content inside a source file, which the
  // repo's l10n contract forbids (CLAUDE.md "Target-language drift"). The
  // overlays load from demo.<locale>.yaml sidecars instead.
  if (m.locales) {
    throw new Error(
      `demo ${id}: inline "locales:" blocks are no longer supported — localized narration lives in generated demo.<lang>.yaml sidecars (make l10n). Fold the translations into .kapi/memory/demo-narration-<lang>.memory.json and delete the block.`,
    );
  }
  m.locales = loadLocaleSidecars(id, m, dir);
  return m;
}
