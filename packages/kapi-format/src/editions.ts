/**
 * @neokapi/kapi-format: reading a block's editions, and reading a file in
 * either schema.
 *
 * A block carries its content as peer editions under their edition keys. The
 * accessors here are the one way a consumer reaches them, so code that wants
 * "the source" or "the nb translation" never spells the storage itself.
 *
 * A file in schema 1.0 carried `source` runs beside `targets` keyed by locale.
 * {@link parseFile} reads it as the editions it describes, the way Go
 * `core/kbf.Unmarshal` does, so every consumer sees one shape.
 */

import type { Block, BlockV1, Edition, EditionKey, File, Origin, Run } from "./block.ts";
import { ReadableKinds, SchemaVersion, SchemaVersionV1, SourceEdition } from "./block.ts";

/** The runs of the edition the block was read in. */
export function sourceRuns(block: Pick<Block, "editions">): Run[] {
  return block.editions?.[SourceEdition]?.runs ?? [];
}

/** The runs of the edition filed under `key`, or `undefined` when the block holds none there. */
export function editionRuns(block: Pick<Block, "editions">, key: EditionKey): Run[] | undefined {
  return block.editions?.[key]?.runs;
}

/** The key of every edition but the source, in the order a serializer writes them. */
export function targetKeys(block: Pick<Block, "editions">): EditionKey[] {
  return Object.keys(block.editions ?? {})
    .filter((key) => key !== SourceEdition)
    .sort();
}

/** An editions map holding `runs` as the source: the editions of a block nothing has translated. */
export function sourceEditions(runs: Run[]): Record<EditionKey, Edition> {
  return { [SourceEdition]: { runs } };
}

/**
 * Whether `key` names a language alone, with no tone and no channel: the key a
 * runtime catalog for that language is looked up by.
 */
export function isLanguageKey(key: EditionKey): boolean {
  return key !== SourceEdition && !key.includes(";");
}

/**
 * Read one block in either schema. A block in the schema 1.0 shape becomes
 * the editions it describes: the source runs under {@link SourceEdition}, each
 * target under its locale with the origin recorded for it, and a target under
 * the empty locale as the unlabelled edition. An origin recorded for a locale
 * with no target describes nothing and is not kept. A block that carries both
 * shapes is refused, because neither can be read without losing the other.
 *
 * Mirrors Go `core/kbf.Block.UnmarshalJSON`.
 */
export function upgradeBlock(raw: Block | BlockV1): Block {
  const value = raw as Partial<Block> & Partial<BlockV1>;
  const v1 =
    value.source !== undefined || value.targets !== undefined || value.targetOrigins !== undefined;
  if (!v1) return raw as Block;
  if (value.editions !== undefined || value.unlabelled !== undefined) {
    throw new Error(
      `kbf: block "${value.id}" carries both editions and the schema 1 source and targets`,
    );
  }
  const { source, targets, targetOrigins, ...rest } = value as BlockV1;
  const editions: Record<EditionKey, Edition> = sourceEditions(source ?? []);
  let unlabelled: Edition | undefined;
  for (const [locale, runs] of Object.entries(targets ?? {})) {
    const edition: Edition = { runs: runs ?? [] };
    const origin: Origin | undefined = targetOrigins?.[locale];
    if (origin) edition.origin = origin;
    if (locale === "") unlabelled = edition;
    else editions[locale] = edition;
  }
  return { ...rest, editions, ...(unlabelled ? { unlabelled } : {}) };
}

/**
 * Parse a `.kbf.json` document, refusing a root kind or a major version this
 * build does not read. A file in schema 1.0 comes back as the editions it
 * describes, stamped {@link SchemaVersion}; a minor of the current major keeps
 * the version it was written in.
 *
 * Mirrors Go `core/kbf.Unmarshal`.
 */
export function parseFile(input: unknown): File {
  const value = (typeof input === "string" ? JSON.parse(input) : input) as Partial<File> | null;
  if (value === null || typeof value !== "object") {
    throw new Error("kbf: decode: not a JSON object");
  }
  if (!value.kind) throw new Error(`kbf: missing kind (want "${ReadableKinds[0]}")`);
  if (!(ReadableKinds as readonly string[]).includes(value.kind)) {
    throw new Error(`kbf: unexpected kind "${value.kind}" (want "${ReadableKinds[0]}")`);
  }
  const major = majorOf(value.schemaVersion);
  if (major === null) throw new Error(`kbf: invalid schemaVersion "${value.schemaVersion}"`);
  const current = majorOf(SchemaVersion);
  const first = majorOf(SchemaVersionV1);
  if (major !== current && major !== first) {
    throw new Error(
      `kbf: unsupported major schemaVersion ${major} (this build reads ${SchemaVersionV1} and ${SchemaVersion})`,
    );
  }
  const documents = (value.documents ?? []).map((doc) => ({
    ...doc,
    blocks: (doc.blocks ?? []).map((block) => upgradeBlock(block as Block | BlockV1)),
  }));
  return {
    ...(value as File),
    schemaVersion: major === first ? SchemaVersion : (value.schemaVersion as string),
    documents,
  };
}

// majorOf reads the major of a `MAJOR.MINOR` version, or null when the text is
// not one. Mirrors Go `core/schemaversion.Major`.
function majorOf(version: string | undefined): number | null {
  const match = /^(\d+)\.(\d+)$/.exec(version ?? "");
  return match ? Number(match[1]) : null;
}
