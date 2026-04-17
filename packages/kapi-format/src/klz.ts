/**
 * @neokapi/kapi-format — .klz archive writer (AD-045).
 *
 * Mirrors `core/klz.Writer` in Go: bundles one or more .klf documents
 * plus skeletons, target overlays, vocabulary, annotation sidecars,
 * and assets into a ZIP with a content-addressed manifest. Each part
 * gets a SHA-256 stamped into the manifest; the manifest itself is
 * the first entry in the ZIP so lazy readers can verify before
 * inflating.
 *
 * Path safety mirrors the Go side: no ZIP slip, no absolute paths,
 * no backslashes, no duplicate entries, no non-NFC UTF-8.
 */

import { createHash } from 'node:crypto';
import { unzipSync, zipSync } from 'fflate';

import type { Block, File } from './block.ts';
import { marshalFile } from './klf.ts';

/** The well-known name of the archive manifest. */
export const MANIFEST_PATH = 'manifest.json';

/**
 * ZIP-epoch timestamp (1980-01-01 UTC). Used as the per-entry mtime
 * so archives with identical inputs produce identical bytes — the
 * manifest SHA-256 is computed over the archive-level payload, so
 * the ZIP header dates must not drift.
 */
const ZIP_EPOCH = new Date(Date.UTC(1980, 0, 1));

/**
 * Archive-level envelope version (RFC-0001 §Versioning). Independent
 * of the .klf SchemaVersion — this is the ZIP envelope's version, not
 * the per-document schema's.
 */
export const ManifestVersion = '1.0' as const;

export type PartRole =
  | 'document'
  | 'target'
  | 'skeleton'
  | 'vocabulary'
  | 'asset'
  | 'signature'
  | 'meta'
  | 'annotation';

export interface ManifestGenerator {
  id: string;
  version: string;
}

export interface ManifestProject {
  id: string;
  sourceLocale: string;
  targetLocales?: string[];
}

export interface ManifestPartInfo {
  path: string;
  sha256: string;
  size: number;
  role: PartRole;
  attributes?: Record<string, unknown>;
}

export interface Manifest {
  kapiLocalizationFormat: typeof ManifestVersion;
  created?: string;
  generator: ManifestGenerator;
  project: ManifestProject;
  parts: ManifestPartInfo[];
}

export interface WriterOptions {
  generator: ManifestGenerator;
  project: ManifestProject;
  /** RFC3339 timestamp. Omitted from the manifest when absent. */
  created?: string;
}

/**
 * Build a .klz archive in a single pass. Parts are added via
 * addDocument / addTarget / addSkeleton / addAnnotationFile /
 * addAsset / addVocabulary / addMeta; `build()` finalizes the
 * manifest (with per-part SHA-256) and returns the ZIP bytes.
 */
export class KlzWriter {
  private readonly opts: WriterOptions;
  private readonly parts: PendingPart[] = [];
  private readonly seen = new Set<string>();

  constructor(opts: WriterOptions) {
    this.opts = opts;
  }

  /** Add a .klf document. Convention: `documents/<name>.klf`. */
  addDocument(path: string, file: File, attrs?: Record<string, unknown>): void {
    this.addPart(path, marshalFile(file), 'document', attrs);
  }

  /** Add a pre-marshaled .klf document payload. */
  addDocumentBytes(path: string, data: Uint8Array, attrs?: Record<string, unknown>): void {
    this.addPart(path, data, 'document', attrs);
  }

  /** Add a sparse per-locale overlay. Convention: `targets/<locale>/<name>.klf`. */
  addTarget(path: string, file: File, attrs?: Record<string, unknown>): void {
    this.addPart(path, marshalFile(file), 'target', attrs);
  }

  /** Add an opaque skeleton blob. Consumer-owned bytes; the writer just stamps a hash. */
  addSkeleton(path: string, data: Uint8Array, attrs?: Record<string, unknown>): void {
    this.addPart(path, data, 'skeleton', attrs);
  }

  /** Add a binary asset (screenshot, audio, video). */
  addAsset(path: string, data: Uint8Array, attrs?: Record<string, unknown>): void {
    this.addPart(path, data, 'asset', attrs);
  }

  /** Add a vocabulary JSON override. Convention: `vocabulary/<name>.json`. */
  addVocabulary(path: string, data: Uint8Array, attrs?: Record<string, unknown>): void {
    this.addPart(path, data, 'vocabulary', attrs);
  }

  /** Add the optional meta.json part at the archive root. */
  addMeta(data: Uint8Array): void {
    this.addPart('meta.json', data, 'meta');
  }

  /**
   * Add an annotation sidecar (`.klfl` JSON-Lines file). The caller
   * supplies the serialized bytes; formatting rules live in
   * `annotation.ts`.
   */
  addAnnotationFile(path: string, data: Uint8Array, attrs?: Record<string, unknown>): void {
    this.addPart(path, data, 'annotation', attrs);
  }

  /** Emit the manifest currently assembled, without finalizing the ZIP. */
  buildManifest(): Manifest {
    const parts: ManifestPartInfo[] = this.parts.map((p) => ({
      path: p.path,
      sha256: sha256Hex(p.data),
      size: p.data.length,
      role: p.role,
      ...(p.attributes ? { attributes: p.attributes } : {}),
    }));
    return {
      kapiLocalizationFormat: ManifestVersion,
      ...(this.opts.created != null ? { created: this.opts.created } : {}),
      generator: this.opts.generator,
      project: this.opts.project,
      parts,
    };
  }

  /**
   * Finalize the archive: compute per-part SHA-256, marshal the
   * manifest, and emit a DEFLATE-compressed ZIP with the manifest as
   * the first entry.
   */
  build(): Uint8Array {
    const manifest = this.buildManifest();
    const manifestBytes = marshalManifest(manifest);

    const entries: Record<string, Uint8Array> = Object.create(null);
    entries[MANIFEST_PATH] = manifestBytes;
    for (const p of this.parts) {
      entries[p.path] = p.data;
    }
    // ZIP epoch (1980-01-01 UTC) for deterministic headers — mirrors
    // the Go side which zeroes Modified so identical inputs produce
    // identical .klz bytes.
    return zipSync(entries, { level: 6, mtime: ZIP_EPOCH });
  }

  /** Added part paths in insertion order. Mirrors Go's SortedPartPaths use. */
  partPaths(): string[] {
    return this.parts.map((p) => p.path);
  }

  // ─── Internals ──────────────────────────────────────────────

  private addPart(
    rawPath: string,
    data: Uint8Array,
    role: PartRole,
    attributes?: Record<string, unknown>,
  ): void {
    const safe = validatePartPath(rawPath);
    if (safe === MANIFEST_PATH) {
      throw new Error(`klz: part path ${JSON.stringify(rawPath)} is reserved for the manifest`);
    }
    if (this.seen.has(safe)) {
      throw new Error(`klz: duplicate part path ${JSON.stringify(safe)}`);
    }
    this.seen.add(safe);
    this.parts.push({ path: safe, data, role, attributes });
  }
}

interface PendingPart {
  path: string;
  data: Uint8Array;
  role: PartRole;
  attributes?: Record<string, unknown>;
}

/**
 * Serialize a Manifest deterministically. Same rules as `klf.marshalFile`:
 * 2-space indent, no HTML escaping, trailing newline. The raw bytes this
 * returns are what the runtime cache keys on, so drift is a cache-key
 * change by design.
 */
export function marshalManifest(m: Manifest): Uint8Array {
  const canon = {
    kapiLocalizationFormat: m.kapiLocalizationFormat ?? ManifestVersion,
    ...(m.created != null ? { created: m.created } : {}),
    generator: { id: m.generator.id, version: m.generator.version },
    project: canonicalProject(m.project),
    parts: m.parts.map(canonicalPart),
  };
  const text = `${JSON.stringify(canon, null, 2)}\n`;
  return new TextEncoder().encode(text);
}

function canonicalProject(p: ManifestProject): unknown {
  const out: Record<string, unknown> = {
    id: p.id,
    sourceLocale: p.sourceLocale,
  };
  if (p.targetLocales && p.targetLocales.length > 0) {
    out.targetLocales = p.targetLocales;
  }
  return out;
}

function canonicalPart(p: ManifestPartInfo): unknown {
  const out: Record<string, unknown> = {
    path: p.path,
    sha256: p.sha256,
    size: p.size,
    role: p.role,
  };
  if (p.attributes && Object.keys(p.attributes).length > 0) {
    out.attributes = p.attributes;
  }
  return out;
}

// ─── Path safety ─────────────────────────────────────────────────

/**
 * Rejects ZIP-slip and other unsafe path shapes matching
 * `core/klz.validatePartPath`:
 *   - empty paths
 *   - leading slash
 *   - backslash separators
 *   - non-canonical paths (`a/./b`, `a/../b`)
 *   - parent references
 *   - empty components
 *   - non-NFC UTF-8 (normalized on write)
 *
 * Returns the NFC-normalized POSIX path.
 */
export function validatePartPath(raw: string): string {
  if (raw === '') throw new Error('klz: empty part path');
  if (raw.startsWith('/')) {
    throw new Error(`klz: part path ${JSON.stringify(raw)} has leading slash`);
  }
  if (raw.includes('\\')) {
    throw new Error(
      `klz: part path ${JSON.stringify(raw)} contains backslash; must use POSIX separators`,
    );
  }
  const cleaned = posixClean(raw);
  if (cleaned !== raw) {
    throw new Error(
      `klz: part path ${JSON.stringify(raw)} is not in canonical form (cleans to ${JSON.stringify(
        cleaned,
      )})`,
    );
  }
  if (cleaned === '.' || cleaned.startsWith('..')) {
    throw new Error(`klz: part path ${JSON.stringify(raw)} escapes the archive root`);
  }
  for (const comp of cleaned.split('/')) {
    if (comp === '') {
      throw new Error(`klz: part path ${JSON.stringify(raw)} has empty component`);
    }
    if (comp === '..') {
      throw new Error(`klz: part path ${JSON.stringify(raw)} contains parent reference`);
    }
  }
  return cleaned.normalize('NFC');
}

/**
 * Minimal POSIX `path.Clean` analog matching Go's `path.Clean` for
 * the inputs we care about: collapses `./`, `a/./b`, and `a/../b`
 * segments. We intentionally do NOT resolve against a base — inputs
 * stay relative.
 */
function posixClean(p: string): string {
  const parts = p.split('/');
  const out: string[] = [];
  for (const part of parts) {
    if (part === '' || part === '.') {
      // Preserve leading/trailing slashes via an initial "" when not at start.
      if (part === '' && out.length === 0 && parts[0] === '') out.push('');
      continue;
    }
    if (part === '..') {
      if (out.length > 0 && out[out.length - 1] !== '..' && out[out.length - 1] !== '') {
        out.pop();
      } else {
        out.push('..');
      }
      continue;
    }
    out.push(part);
  }
  const joined = out.join('/');
  return joined === '' ? '.' : joined;
}

function sha256Hex(data: Uint8Array): string {
  return createHash('sha256').update(data).digest('hex');
}

// ─── Reader ──────────────────────────────────────────────────────

export interface KlzReaderOptions {
  /** Verify each part's SHA-256 on access. Defaults to true. */
  verifyHashes?: boolean;
}

/**
 * Parse a .klz archive into an in-memory structure. Decompresses
 * lazily on `read`; the manifest is decoded eagerly so `documents()`,
 * `targets()`, and hash checks work without re-inflating.
 */
export class KlzReader {
  private readonly entries: Record<string, Uint8Array>;
  readonly manifest: Manifest;
  private readonly verifyHashes: boolean;

  constructor(archive: Uint8Array, opts: KlzReaderOptions = {}) {
    this.entries = unzipSync(archive);
    const manifestBytes = this.entries[MANIFEST_PATH];
    if (!manifestBytes) {
      throw new Error('klz: archive is missing manifest.json');
    }
    this.manifest = JSON.parse(new TextDecoder().decode(manifestBytes)) as Manifest;
    this.verifyHashes = opts.verifyHashes ?? true;
  }

  /** Raw bytes for a manifested part. Verifies SHA-256 if enabled. */
  read(partPath: string): Uint8Array {
    const entry = this.manifest.parts.find((p) => p.path === partPath);
    if (!entry) {
      throw new Error(`klz: part ${JSON.stringify(partPath)} not in manifest`);
    }
    const bytes = this.entries[partPath];
    if (!bytes) {
      throw new Error(`klz: part ${JSON.stringify(partPath)} missing from archive`);
    }
    if (this.verifyHashes) {
      const got = sha256Hex(bytes);
      if (got !== entry.sha256) {
        throw new Error(
          `klz: part ${JSON.stringify(partPath)} hash mismatch — manifest ${entry.sha256}, actual ${got}`,
        );
      }
    }
    return bytes;
  }

  /** All source documents parsed from `documents/*.klf` parts. */
  documents(): File[] {
    return this.partsByRole('document').map((p) => this.parseFile(p.path));
  }

  /** Target overlays for a given locale parsed from `targets/{locale}/*.klf`. */
  targets(locale: string): File[] {
    const prefix = `targets/${locale}/`;
    return this.partsByRole('target')
      .filter((p) => p.path.startsWith(prefix))
      .map((p) => this.parseFile(p.path));
  }

  /** Bytes for every annotation sidecar (`annotations/*.klfl`). */
  annotationFiles(): Uint8Array[] {
    return this.partsByRole('annotation').map((p) => this.read(p.path));
  }

  /**
   * Iterate every source Block across every document. Mirrors the
   * Go `core/klz` iteration helper; consumers that want to flatten
   * targets per locale loop over the yield.
   */
  *blocks(): IterableIterator<{ file: File; block: Block }> {
    for (const file of this.documents()) {
      for (const doc of file.documents) {
        for (const block of doc.blocks) {
          yield { file, block };
        }
      }
    }
  }

  private partsByRole(role: PartRole): ManifestPartInfo[] {
    return this.manifest.parts.filter((p) => p.role === role);
  }

  private parseFile(partPath: string): File {
    const bytes = this.read(partPath);
    return JSON.parse(new TextDecoder().decode(bytes)) as File;
  }
}

// flattenRuns moved to `./runs.ts` — it's a pure Run encoder used by
// browser-side runtime code, and klz.ts pulls in node:crypto + fflate.
