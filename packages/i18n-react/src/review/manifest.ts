/**
 * Static review-manifest builder (Node side).
 *
 * Reads a KBF tree — the source catalog plus its `i18n-<locale>` siblings, and
 * any `.overlays.jsonl` stand-off annotation files — and produces one JSON manifest keyed
 * by block hash: source text, per-locale targets, translator properties, and
 * term/check annotations. It is the read-only, deploy-time counterpart to the dev
 * review middleware (`review/store.ts`): the hosted overlay (`review/hosted.ts`)
 * fetches this manifest from the deployed site, so a reviewer can open the live
 * page in context with no dev server. Emitted by `neokapi-i18n compile --review`.
 */

import { readFileSync, readdirSync, statSync, existsSync } from "node:fs";
import { join } from "node:path";

import type { File as KBFFile, Run } from "@neokapi/kapi-format";
import { flattenRuns, isAnnotationPath, isKbfPath } from "@neokapi/kapi-format";

/** One block's review data, flattened to display-ready strings. */
export interface ReviewManifestEntry {
  /** Flattened source text. */
  source: string;
  /** locale → flattened target text (only locales that have a target). */
  targets: Record<string, string>;
  /** Translator-facing context. */
  properties: {
    file?: string;
    line?: number;
    component?: string;
    element?: string;
    locNote?: string;
  };
  /** Term / check annotations, summarized for display. */
  annotations: Array<{ type: string; summary: string }>;
}

/** Block hash → review data. Serialized to `review.json`. */
export type ReviewManifest = Record<string, ReviewManifestEntry>;

/**
 * Build a review manifest from one or more `.kbf.json`/`.overlays.jsonl` files or directories.
 * Blocks are merged by hash across inputs, so passing the source catalog and
 * every `i18n-<locale>` directory yields a single entry per block carrying all
 * of its locales' targets.
 */
export function buildReviewManifest(paths: string[]): ReviewManifest {
  const manifest: ReviewManifest = {};
  const files: string[] = [];
  for (const p of paths) {
    if (!existsSync(p)) continue;
    if (statSync(p).isDirectory()) walkFiles(p, files);
    else files.push(p);
  }
  // Blocks first so annotation records (which reference a block hash) land on
  // an existing entry.
  for (const path of files) {
    if (isKbfPath(path)) indexBlocks(path, manifest);
  }
  for (const path of files) {
    if (isAnnotationPath(path)) indexAnnotations(path, manifest);
  }
  return manifest;
}

function entryFor(manifest: ReviewManifest, hash: string): ReviewManifestEntry {
  return (manifest[hash] ??= { source: "", targets: {}, properties: {}, annotations: [] });
}

function indexBlocks(path: string, manifest: ReviewManifest): void {
  let file: KBFFile;
  try {
    file = JSON.parse(readFileSync(path, "utf-8")) as KBFFile;
  } catch {
    return; // unparseable — extract will rewrite it
  }
  for (const doc of file.documents ?? []) {
    for (const block of doc.blocks ?? []) {
      if (!block.hash) continue;
      const e = entryFor(manifest, block.hash);
      if (!e.source && block.source) e.source = flattenRuns(block.source);
      const p = block.properties;
      if (p && !e.properties.file) {
        e.properties = {
          file: p.file,
          line: p.line,
          component: p.component,
          element: p.element,
          locNote: p.locNote,
        };
      }
      for (const [locale, runs] of Object.entries(block.targets ?? {})) {
        const r = runs as Run[] | undefined;
        if (r && r.length > 0) e.targets[locale] = flattenRuns(r);
      }
    }
  }
}

function indexAnnotations(path: string, manifest: ReviewManifest): void {
  let lines: string[];
  try {
    lines = readFileSync(path, "utf-8").split("\n");
  } catch {
    return;
  }
  let annotationType = "unknown";
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("{")) continue;
    let rec: Record<string, unknown>;
    try {
      rec = JSON.parse(trimmed) as Record<string, unknown>;
    } catch {
      continue;
    }
    if (rec.type === "header") {
      annotationType = typeof rec.annotationType === "string" ? rec.annotationType : "unknown";
      continue;
    }
    if (rec.type !== "annotation") continue;
    if (typeof rec.block !== "string" || rec.block === "") continue;
    const e = manifest[rec.block];
    if (!e) continue; // annotation for a block not in the source tree
    e.annotations.push({
      type: shortType(annotationType),
      summary: annotationSummary(rec.data) ?? annotationType,
    });
  }
}

function shortType(t: string): string {
  return t.split("/").pop() ?? t;
}

function annotationSummary(data: unknown): string | null {
  if (typeof data === "string") return data;
  if (data === null || typeof data !== "object") return null;
  const d = data as Record<string, unknown>;
  for (const key of ["message", "term", "text", "match", "note", "summary"]) {
    if (typeof d[key] === "string") return d[key] as string;
  }
  return null;
}

function walkFiles(dir: string, out: string[]): void {
  for (const ent of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, ent.name);
    if (ent.isDirectory()) walkFiles(path, out);
    else if (ent.isFile() && (isKbfPath(path) || isAnnotationPath(path))) out.push(path);
  }
}
