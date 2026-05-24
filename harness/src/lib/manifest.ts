import fs from "node:fs";
import path from "node:path";
import YAML from "yaml";
import type { DemoManifest } from "../types.ts";
import { DEMOS_DIR, demoSrcDir } from "./paths.ts";

/** List demo ids (directories under demos/ that contain a demo.yaml), sorted. */
export function listDemoIds(): string[] {
  if (!fs.existsSync(DEMOS_DIR)) return [];
  return fs
    .readdirSync(DEMOS_DIR, { withFileTypes: true })
    .filter((d) => d.isDirectory() && fs.existsSync(path.join(DEMOS_DIR, d.name, "demo.yaml")))
    .map((d) => d.name)
    .sort();
}

export function loadManifest(id: string): DemoManifest {
  const file = path.join(demoSrcDir(id), "demo.yaml");
  const raw = fs.readFileSync(file, "utf8");
  const m = YAML.parse(raw) as DemoManifest;
  if (!m.id) m.id = id;
  if (m.id !== id) {
    throw new Error(`demo.yaml id "${m.id}" does not match directory "${id}"`);
  }
  // Light validation so authoring mistakes fail fast.
  if (!m.title || !m.prompt) throw new Error(`demo ${id}: title and prompt are required`);
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
  }
  return m;
}
