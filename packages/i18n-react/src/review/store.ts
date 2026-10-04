/**
 * In-context review store + HTTP handler (Node side).
 *
 * Serves review payloads out of a local KBF tree (the output of
 * `neokapi-i18n extract`, translated in place by kapi) and writes
 * target edits back into the `.kbf.json` files — git-diffable review.
 * Stand-off annotation files (`*.overlays.jsonl`, e.g. from `kapi run
 * term-check` / `qa`) found under the same tree are passed through
 * per block hash so the overlay can paint term/check highlights.
 *
 * Endpoints (mounted at `/__kapi/review` by the Vite plugin):
 *
 *   GET  {base}/annotations     → { [hash]: Annotation[] }
 *   GET  {base}/events          → SSE stream of translation updates
 *   GET  {base}/{hash}?locale=  → review payload for one block
 *   PUT  {base}/{hash}          → { locale, text } target write-back
 *
 * The handler is plain Node http — usable behind Vite's connect
 * middleware today and any dev server later.
 */

import { readFileSync, readdirSync, statSync, writeFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import type { IncomingMessage, ServerResponse } from "node:http";

import type { Block, BlockV1, Edition, File as KBFFile, Run } from "@neokapi/kapi-format";
import {
  Kind,
  SchemaVersion,
  flattenRuns,
  isAnnotationPath,
  isKbfPath,
  marshalFile,
  sourceRuns,
  targetKeys,
  upgradeBlock,
} from "@neokapi/kapi-format";

interface BlockLocation {
  path: string;
  docIndex: number;
  blockIndex: number;
}

interface AnnotationRecord {
  id: string;
  annotationType: string;
  /** The block the annotation is about. */
  block: string;
  /** Where inside that block — the model's Anchor, kind and all. */
  anchor: { kind: string } & Record<string, unknown>;
  data: unknown;
}

export interface ReviewPayload {
  hash: string;
  sourceText: string;
  source: Run[];
  targets: Record<string, { text: string; runs: Run[] }>;
  properties: Block["properties"];
  placeholders: Block["placeholders"];
  annotations: AnnotationRecord[];
}

export interface ReviewUpdate {
  hash: string;
  locale: string;
  text: string;
}

export class ReviewStore {
  private readonly kbfDir: string;
  private index = new Map<string, BlockLocation>();
  private annotations = new Map<string, AnnotationRecord[]>();
  /** mtimeMs per indexed file — index rebuilds when anything drifts. */
  private mtimes = new Map<string, number>();
  private readonly subscribers = new Set<(update: ReviewUpdate) => void>();

  constructor(kbfDir: string) {
    this.kbfDir = kbfDir;
  }

  subscribe(fn: (update: ReviewUpdate) => void): () => void {
    this.subscribers.add(fn);
    return () => this.subscribers.delete(fn);
  }

  private broadcast(update: ReviewUpdate): void {
    for (const fn of this.subscribers) fn(update);
  }

  /** Rebuild the hash index when any .kbf.json/.overlays.jsonl file changed. */
  private refresh(): void {
    if (!existsSync(this.kbfDir)) {
      this.index.clear();
      this.annotations.clear();
      return;
    }
    const files: string[] = [];
    walkFiles(this.kbfDir, files);
    let dirty = files.length !== this.mtimes.size;
    if (!dirty) {
      for (const f of files) {
        const m = statSync(f).mtimeMs;
        if (this.mtimes.get(f) !== m) {
          dirty = true;
          break;
        }
      }
    }
    if (!dirty) return;

    this.index.clear();
    this.annotations.clear();
    this.mtimes.clear();
    for (const path of files) {
      this.mtimes.set(path, statSync(path).mtimeMs);
      if (isKbfPath(path)) this.indexKBF(path);
      else this.indexAnnotations(path);
    }
  }

  private indexKBF(path: string): void {
    try {
      const file = readKBF(path);
      (file.documents ?? []).forEach((doc, docIndex) => {
        (doc.blocks ?? []).forEach((block, blockIndex) => {
          if (block.hash && !this.index.has(block.hash)) {
            this.index.set(block.hash, { path, docIndex, blockIndex });
          }
        });
      });
    } catch {
      // Unparseable file — skip; extract will rewrite it.
    }
  }

  private indexAnnotations(path: string): void {
    try {
      const lines = readFileSync(path, "utf-8").split("\n");
      let annotationType = "unknown";
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed.startsWith("{")) continue;
        const rec = JSON.parse(trimmed) as Record<string, unknown>;
        if (rec.type === "header") {
          annotationType = typeof rec.annotationType === "string" ? rec.annotationType : "unknown";
          continue;
        }
        if (rec.type !== "annotation") continue;
        const anchor = rec.anchor as AnnotationRecord["anchor"] | undefined;
        if (!anchor || typeof rec.block !== "string" || rec.block === "") continue;
        const list = this.annotations.get(rec.block) ?? [];
        list.push({
          id: typeof rec.id === "string" ? rec.id : "",
          annotationType,
          block: rec.block,
          anchor,
          data: rec.data,
        });
        this.annotations.set(rec.block, list);
      }
    } catch {
      // Malformed annotation file — annotations are derivable; skip.
    }
  }

  get(hash: string): ReviewPayload | null {
    this.refresh();
    const loc = this.index.get(hash);
    if (!loc) return null;
    const file = readKBF(loc.path);
    const block = file.documents?.[loc.docIndex]?.blocks?.[loc.blockIndex];
    if (!block) return null;
    const targets: ReviewPayload["targets"] = {};
    for (const key of targetKeys(block)) {
      const runs = block.editions[key]?.runs ?? [];
      targets[key] = { text: flattenRuns(runs), runs };
    }
    const source = sourceRuns(block);
    return {
      hash,
      sourceText: flattenRuns(source),
      source,
      targets,
      properties: block.properties,
      placeholders: block.placeholders,
      annotations: this.annotations.get(hash) ?? [],
    };
  }

  annotationsByHash(): Record<string, AnnotationRecord[]> {
    this.refresh();
    return Object.fromEntries(this.annotations);
  }

  /**
   * Write a target edit back into the block's `.kbf.json` file. The
   * edited text is stored as a single text run, the same shape a
   * translator editing the file by hand produces; kapi's validators
   * and content memory treat it as any other unstructured target. The
   * edition records the edit as a person's (reviewedEdition).
   */
  put(hash: string, locale: string, text: string): ReviewPayload | null {
    this.refresh();
    const loc = this.index.get(hash);
    if (!loc) return null;
    const file = readKBF(loc.path);
    const block = file.documents?.[loc.docIndex]?.blocks?.[loc.blockIndex];
    if (!block) return null;
    block.editions = {
      ...block.editions,
      [locale]: reviewedEdition(block.editions[locale], text),
    };
    // Written back under the current root kind and schema, the way kapi's own
    // writer stamps a catalog it rewrites. A catalog an older release of this
    // package or of kapi wrote becomes a current one the first time a target is
    // saved into it. A minor of the current schema keeps its version.
    file.kind = Kind;
    if (file.schemaVersion?.split(".")[0] !== SchemaVersion.split(".")[0]) {
      file.schemaVersion = SchemaVersion;
    }
    let serialized: Uint8Array | string;
    try {
      serialized = marshalFile(file);
    } catch {
      // Hand-made or partial KBF (missing generator/project envelope)
      // — keep it valid JSON rather than refusing the edit.
      serialized = JSON.stringify(file, null, 2) + "\n";
    }
    writeFileSync(loc.path, serialized);
    this.mtimes.set(loc.path, statSync(loc.path).mtimeMs);
    this.broadcast({ hash, locale, text });
    return this.get(hash);
  }
}

/**
 * The edition a reviewer's edit leaves: the edited text as one run, with what
 * the engine records when a person changes a translation (Go
 * `core/change.Consequences`). It is translated, with a human origin stamped
 * now, and keeps the producer's score. The producer's origin, status and
 * derivation described the wording the reviewer replaced, so none of them is
 * kept.
 */
export function reviewedEdition(previous: Edition | undefined, text: string): Edition {
  const edition: Edition = {
    runs: [{ text }] as Run[],
    status: "translated",
    origin: { kind: "human", timestamp: new Date().toISOString().replace(/\.\d+Z$/, "Z") },
  };
  if (previous?.score) edition.score = previous.score;
  return edition;
}

/**
 * Read a catalog for review, in either schema. A hand-made catalog may lack the
 * envelope a strict read wants (kind, version), and review still serves and
 * edits its blocks, so this reads the blocks leniently: each in the shape it was
 * written, upgraded to editions (upgradeBlock).
 */
function readKBF(path: string): KBFFile {
  const file = JSON.parse(readFileSync(path, "utf-8")) as KBFFile;
  for (const doc of file.documents ?? []) {
    doc.blocks = (doc.blocks ?? []).map((block) => upgradeBlock(block as Block | BlockV1));
  }
  return file;
}

function walkFiles(dir: string, out: string[]): void {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) walkFiles(path, out);
    else if (entry.isFile() && (isKbfPath(path) || isAnnotationPath(path))) out.push(path);
  }
}

// ─── HTTP handler ────────────────────────────────────────────

/**
 * Handle one request under the review base path. `url` is the path
 * AFTER the base (leading slash, no query). Returns false when the
 * route doesn't match (caller falls through to the next middleware).
 */
export function handleReviewRequest(
  store: ReviewStore,
  req: IncomingMessage,
  res: ServerResponse,
  url: string,
): boolean {
  const [path] = url.split("?");
  const seg = path.replace(/^\/+/, "");

  if (req.method === "GET" && seg === "annotations") {
    json(res, 200, store.annotationsByHash());
    return true;
  }

  if (req.method === "GET" && seg === "events") {
    res.writeHead(200, {
      "content-type": "text/event-stream",
      "cache-control": "no-cache",
      connection: "keep-alive",
    });
    res.write(":ok\n\n");
    const unsubscribe = store.subscribe((update) => {
      res.write(`data: ${JSON.stringify(update)}\n\n`);
    });
    req.on("close", unsubscribe);
    return true;
  }

  if (seg.length > 0 && !seg.includes("/")) {
    if (req.method === "GET") {
      const payload = store.get(seg);
      if (!payload) {
        json(res, 404, { error: `no block with hash ${seg}` });
        return true;
      }
      json(res, 200, payload);
      return true;
    }
    if (req.method === "PUT") {
      readBody(req)
        .then((body) => {
          const { locale, text } = JSON.parse(body) as { locale?: string; text?: string };
          if (!locale || typeof text !== "string") {
            json(res, 400, { error: "body must be { locale, text }" });
            return;
          }
          const updated = store.put(seg, locale, text);
          if (!updated) {
            json(res, 404, { error: `no block with hash ${seg}` });
            return;
          }
          json(res, 200, updated);
        })
        .catch((e) => json(res, 400, { error: String(e) }));
      return true;
    }
  }

  return false;
}

function json(res: ServerResponse, status: number, body: unknown): void {
  res.writeHead(status, { "content-type": "application/json" });
  res.end(JSON.stringify(body));
}

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    req.on("data", (c: Buffer) => chunks.push(c));
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    req.on("error", reject);
  });
}
