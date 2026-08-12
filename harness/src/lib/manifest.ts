import fs from "node:fs";
import path from "node:path";
import YAML from "yaml";
import type { DemoLocaleOverlay, DemoManifest } from "../types.ts";
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
  for (const n of m.narration) {
    if (n.kind === "artifact" && !n.artifact) {
      throw new Error(`demo ${id}: narration scene "${n.id}" is kind=artifact but has no artifact id`);
    }
    if (n.kind === "artifact" && !m.artifacts.find((a) => a.id === n.artifact)) {
      throw new Error(`demo ${id}: narration scene "${n.id}" references unknown artifact "${n.artifact}"`);
    }
    if (n.kind === "desktop" && !n.beat) {
      throw new Error(`demo ${id}: narration scene "${n.id}" is kind=desktop but has no beat id`);
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
