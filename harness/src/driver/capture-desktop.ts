/**
 * capture-desktop.ts — "capture" stage for terminal:"desktop" demos.
 *
 * There is no transcript to capture; instead we (1) record the Kapi Desktop
 * walkthrough screencast (record-desktop.ts) and (2) write a minimal
 * capture.json so the rest of the pipeline (registry, calcMeta, title/outro
 * cards) works unchanged. The title/subtitle/aspects come from the manifest;
 * `events` are empty (the desktop scenes replay the screencast, not a terminal).
 */
import fs from "node:fs";
import path from "node:path";
import type { DemoManifest, DemoCapture } from "../types.ts";
import { ensureDir, publicDemoDir } from "../lib/paths.ts";
import { recordDesktop } from "./record-desktop.ts";

export interface CaptureDesktopOptions {
  force?: boolean;
  /** UI language for the recorded app (default "en"). Passed through to
   *  record-desktop so a localized recording pass can run the desktop UI in
   *  that language. NOTE: the screencast outputs are NOT locale-suffixed —
   *  a localized pass overwrites public/<id>/screencast.* (record per locale,
   *  then render/publish that locale before re-recording another). */
  uiLocale?: string;
}

export async function captureDesktopDemo(m: DemoManifest, opts: CaptureDesktopOptions = {}): Promise<void> {
  const pub = ensureDir(publicDemoDir(m.id));

  // 1. Record the screencast (light + dark webms + screencast.json).
  await recordDesktop(m.id, {
    force: opts.force,
    web: m.target === "web",
    bowrainDesktop: m.target === "bowrain-desktop",
    uiLocale: opts.uiLocale,
  });

  // 2. Minimal capture.json so the rest of the pipeline is unchanged.
  const capture: DemoCapture = {
    id: m.id,
    title: m.title,
    subtitle: m.subtitle,
    tagline: m.tagline,
    aspects: m.aspects,
    prompt: "",
    terminal: "desktop",
    brand: m.brand ?? "desktop",
    events: [],
    meta: {
      model: "Kapi Desktop",
      durationMs: 0,
      numTurns: 0,
      capturedAt: new Date().toISOString(),
      errors: [],
    },
  };
  fs.writeFileSync(path.join(pub, "capture.json"), JSON.stringify(capture, null, 2));
  console.log(`  ✓ desktop capture written for ${m.id}`);
}
