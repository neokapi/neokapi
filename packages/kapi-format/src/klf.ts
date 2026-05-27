/**
 * @neokapi/kapi-format — deterministic .klf JSON serialization.
 *
 * Produces the same bytes `core/klf.Marshal` produces in Go: 2-space
 * indent, no HTML escaping, trailing newline, fields emitted in the
 * struct-definition order the spec pins down. The byte output is
 * stable for hashing and git diffing across runs, machines, and
 * language implementations.
 */

import type {
  Block,
  Document,
  File,
  Generator,
  Placeholder,
  Project,
  Run,
  Skeleton,
  Vocabulary,
} from "./block.ts";
import { Kind, SchemaVersion } from "./block.ts";

/**
 * Serialize a .klf File to UTF-8 bytes. Deterministic: identical
 * input yields identical bytes across runs, machines, and language
 * implementations.
 */
export function marshalFile(file: File): Uint8Array {
  const canon = canonicalFile(file);
  return jsonBytes(canon);
}

/**
 * Serialize a single Block deterministically. Used for manifest
 * computation and for tests that assert byte-level parity against
 * Go goldens without wrapping in a File envelope.
 */
export function marshalBlock(block: Block): Uint8Array {
  return jsonBytes(canonicalBlock(block));
}

/**
 * Build a minimal File envelope around one or more documents.
 * Convenience for extractors that produce documents but don't want
 * to hand-assemble the envelope fields.
 */
export function newFile(opts: {
  generator: Generator;
  project: Project;
  documents: Document[];
  created?: string;
  vocabulary?: Vocabulary;
}): File {
  return {
    schemaVersion: SchemaVersion,
    kind: Kind,
    ...(opts.created != null ? { created: opts.created } : {}),
    generator: opts.generator,
    project: opts.project,
    ...(opts.vocabulary ? { vocabulary: opts.vocabulary } : {}),
    documents: opts.documents,
  };
}

// ─── Canonical ordering helpers ──────────────────────────────────

function canonicalFile(f: File): unknown {
  return omitUndefined({
    schemaVersion: f.schemaVersion ?? SchemaVersion,
    kind: f.kind ?? Kind,
    created: f.created,
    generator: canonicalGenerator(f.generator),
    project: canonicalProject(f.project),
    vocabulary: f.vocabulary ? canonicalVocabulary(f.vocabulary) : undefined,
    documents: f.documents.map(canonicalDocument),
  });
}

function canonicalGenerator(g: Generator): unknown {
  return omitUndefined({
    id: g.id,
    version: g.version,
    capabilities: nonEmpty(g.capabilities),
  });
}

function canonicalProject(p: Project): unknown {
  return { id: p.id, sourceLocale: p.sourceLocale };
}

function canonicalVocabulary(v: Vocabulary): unknown {
  return omitUndefined({ extends: nonEmpty(v.extends) });
}

function canonicalDocument(d: Document): unknown {
  return omitUndefined({
    id: d.id,
    documentType: d.documentType,
    path: d.path,
    sourceHash: d.sourceHash,
    skeleton: d.skeleton ? canonicalSkeleton(d.skeleton) : undefined,
    blocks: d.blocks.map(canonicalBlock),
  });
}

function canonicalSkeleton(s: Skeleton): unknown {
  // Field order mirrors Go core/klf.Skeleton: ref then inline.
  return omitUndefined({ ref: s.ref, inline: s.inline });
}

function canonicalBlock(b: Block): unknown {
  return omitUndefined({
    id: b.id,
    hash: b.hash,
    translatable: b.translatable,
    type: b.type,
    source: b.source.map(canonicalRun),
    targets: canonicalTargets(b.targets),
    placeholders: nonEmpty(b.placeholders?.map(canonicalPlaceholder)),
    properties: b.properties,
    preview: b.preview,
  });
}

function canonicalTargets(t: Block["targets"] | undefined): Record<string, unknown> | undefined {
  if (!t) return undefined;
  const keys = Object.keys(t).sort();
  if (keys.length === 0) return undefined;
  const out: Record<string, unknown> = {};
  for (const k of keys) {
    out[k] = t[k]?.map(canonicalRun);
  }
  return out;
}

function canonicalRun(r: Run): unknown {
  if ("text" in r) return { text: r.text };
  if ("ph" in r) return { ph: omitUndefined({ ...r.ph }) };
  if ("pcOpen" in r) return { pcOpen: omitUndefined({ ...r.pcOpen }) };
  if ("pcClose" in r) return { pcClose: omitUndefined({ ...r.pcClose }) };
  if ("sub" in r) return { sub: omitUndefined({ ...r.sub }) };
  if ("plural" in r) {
    const formsIn = r.plural.forms;
    const keys = Object.keys(formsIn).sort();
    const forms: Record<string, unknown> = {};
    for (const k of keys) forms[k] = formsIn[k as keyof typeof formsIn]?.map(canonicalRun);
    return {
      plural: omitUndefined({
        pivot: r.plural.pivot,
        forms,
      }),
    };
  }
  if ("select" in r) {
    const casesIn = r.select.cases;
    const keys = Object.keys(casesIn).sort();
    const cases: Record<string, unknown> = {};
    for (const k of keys) cases[k] = casesIn[k]?.map(canonicalRun);
    return {
      select: omitUndefined({ pivot: r.select.pivot, cases }),
    };
  }
  throw new Error("klf: run has no recognized discriminator");
}

function canonicalPlaceholder(p: Placeholder): unknown {
  return omitUndefined({
    name: p.name,
    kind: p.kind,
    jsType: p.jsType,
    sourceExpr: p.sourceExpr,
    optional: p.optional,
  });
}

// ─── Marshal plumbing ────────────────────────────────────────────

function jsonBytes(value: unknown): Uint8Array {
  const text = `${JSON.stringify(value, null, 2)}\n`;
  return new TextEncoder().encode(text);
}

function omitUndefined<T extends object>(obj: T): T {
  for (const key of Object.keys(obj) as Array<keyof T>) {
    if (obj[key] === undefined) delete obj[key];
  }
  return obj;
}

function nonEmpty<T>(arr: T[] | undefined): T[] | undefined {
  return arr && arr.length > 0 ? arr : undefined;
}
