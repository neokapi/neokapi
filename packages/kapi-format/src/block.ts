/**
 * @neokapi/kapi-format — canonical extraction/translation-unit schema.
 *
 * Defines the in-memory shape of translatable content shared between
 * @neokapi/kapi-react (TS), neokapi (Go), and CAT-tool adapters. Field
 * names match what the Go port in neokapi's core/model/ will use
 * so codegen / hand-port is mechanical.
 *
 * The hierarchy is:
 *
 *   Block  →  Run[]
 *
 *   - Block is the unit of translation tracking. Extractors produce
 *     Blocks; mergers consume them; TM, status, targets, and
 *     annotations are all keyed per Block (per locale where
 *     applicable).
 *
 *   - A Run is one element of a Block's flat inline content
 *     sequence. A discriminated union: a text chunk, a placeholder,
 *     an opening or closing paired code, a reference to a subblock,
 *     or a structured plural / select node. The sequence is
 *     otherwise flat — paired codes (pcOpen / pcClose) use matching
 *     `id`s to identify themselves, and validators enforce
 *     well-formed nesting within the same scope.
 *
 *   - Plural and select constructs are represented as `plural` /
 *     `select` runs whose sub-content is itself `Run[]`. This is
 *     the only place the run sequence is recursive, and it only
 *     appears in Blocks that actually contain a plural or select.
 *     Simple blocks have perfectly flat `Run[]` with no recursion.
 *
 * THE RULE: neokapi-react extracts Blocks. kapi / neokapi stores,
 * processes, and renders them. Runs are the wire-level content
 * representation; internal processing is free to materialize a
 * coded-string form with PUA markers on demand for hot-path text
 * operations, but that form never leaves the hot path.
 */

// ─── Runs — the inline content primitive ──────────────────────────

/**
 * A Run is one element in a Block's content sequence. Discriminated
 * union keyed by the present field.
 */
export type Run =
  | TextRun
  | PlaceholderRun
  | PcOpenRun
  | PcCloseRun
  | SubRun
  | PluralRunWrapper
  | SelectRunWrapper;

/** A text chunk. Plain string, no markup. */
export interface TextRun {
  text: string;
}

/**
 * A self-closing placeholder: a variable, a conditional JSX
 * expression, a <br/>, a redaction, an icon, etc. The `data` field
 * carries the authoritative source slice for round-trip.
 */
export interface PlaceholderRun {
  ph: {
    /** Stable within the containing runs scope. */
    id: string;
    /** Vocabulary key, e.g. "jsx:var", "jsx:node", "html:br". */
    type: string;
    /** Fine-grained discriminator ("string" | "number" | tag name | …). */
    subType?: string;
    /** Original source slice (`{count}`, `<br/>`, etc.). Preserved verbatim. */
    data: string;
    /**
     * Stable, human-friendly identifier used by tools, LLM prompts,
     * and CAT-tool chips. For variables, the variable name; for
     * elements, the tag name.
     */
    equiv: string;
    /** Display label for chips. 1-3 chars usually. Falls back to `equiv`. */
    disp?: string;
    /** Constraint flags. Populated from vocabulary; can be overridden. */
    constraints?: RunConstraints;
  };
}

/**
 * The opening half of a paired code (an inline element that wraps
 * some content). The matching `PcCloseRun` uses the same `id`.
 */
export interface PcOpenRun {
  pcOpen: {
    id: string;
    type: string;
    subType?: string;
    /** The raw opening source ("<span className=\"muted\">"). */
    data: string;
    equiv: string;
    disp?: string;
    constraints?: RunConstraints;
  };
}

/**
 * The closing half of a paired code. Shares `id` with its PcOpen
 * inside the same runs scope, and repeats `type` / `subType` /
 * `equiv` for locality so renderers can render a single PcClose
 * without walking back for its partner.
 */
export interface PcCloseRun {
  pcClose: {
    id: string;
    type: string;
    subType?: string;
    /** The raw closing source ("</span>"). */
    data: string;
    equiv?: string;
  };
}

/**
 * A reference to a subblock. Used for sub-filter output: when an
 * outer format (HTML, YAML, etc.) extracts a field whose value is
 * itself a mini-document in another format (HTML inside a YAML
 * field, for example), the subfilter produces a separate Block and
 * the outer Block contains a SubRun pointing at it by id.
 */
export interface SubRun {
  sub: {
    /** Stable within the containing runs scope. */
    id: string;
    /** The id of the referenced subblock. */
    ref: string;
    /** Human-friendly label for the reference. */
    equiv: string;
  };
}

/**
 * A structured plural construct. The outer wrapper is one Run in
 * the enclosing sequence; its `forms` map contains a Run[] per
 * plural form, each with its own ID scope for paired codes.
 *
 * Inline markup (pcOpen / pcClose) inside plural forms is a
 * first-class typed run, not text. Inline variable references
 * (ph) inside plural forms work the same way. The pivot variable
 * that drives plural selection is named in `pivot` and is declared
 * in the Block's `placeholders` array with kind `'icu-pivot'`.
 */
export interface PluralRunWrapper {
  plural: PluralRun;
}

export interface PluralRun {
  /** Variable name driving plural selection. Must also appear in Block.placeholders with kind 'icu-pivot'. */
  pivot: string;
  /**
   * Map from plural form to that form's runs. Not every form
   * needs to be present; consumers that encounter a form not in
   * the map fall back to 'other' (ICU convention).
   */
  forms: Partial<Record<PluralForm, Run[]>>;
}

/**
 * A structured select construct. Symmetric to `PluralRunWrapper`
 * but keyed by arbitrary string values (typically a discrete
 * categorical variable like gender, status, role) instead of the
 * fixed plural-form enum.
 */
export interface SelectRunWrapper {
  select: SelectRun;
}

export interface SelectRun {
  /** Variable name driving case selection. */
  pivot: string;
  /**
   * Map from select value to that case's runs. Convention: include
   * an 'other' key as the fallback for values not otherwise
   * matched, matching ICU MessageFormat semantics.
   */
  cases: Record<string, Run[]>;
}

export type PluralForm = "zero" | "one" | "two" | "few" | "many" | "other";

export interface RunConstraints {
  /**
   * May a target drop this run? False for required variables —
   * validators reject targets that omit them. True for optional
   * conditional nodes where some languages legitimately don't
   * render the element.
   */
  deletable: boolean;
  /**
   * May a target duplicate this run? True for variables that some
   * languages repeat (formal/informal pronouns referring to the
   * same entity, for instance).
   */
  cloneable: boolean;
  /** May a target reorder this run relative to its siblings? */
  reorderable: boolean;
}

// ─── Block — the translatable unit and tracking primitive ─────────

/**
 * A Block is the unit of translation tracking. Typically a JSX
 * element (<h2>, <button>, <p>), an HTML paragraph, a Markdown
 * heading, or one attribute value (alt, placeholder). Extractors
 * produce Blocks; TM, status, targets, merge, and annotations are
 * all keyed on the Block.
 */
export interface Block {
  id: string;
  /** Content hash over source runs — drives matching across re-extractions. */
  hash: string;
  translatable: boolean;
  /** Coarse classification; drives preview layout decisions. */
  type: BlockType;

  /** Source content as a flat sequence of Runs. Plurals and select groups live inside the sequence as nested runs. */
  source: Run[];

  /** Target content per locale. Each locale's target is its own Run sequence. */
  targets?: Record<LocaleID, Run[]>;

  /**
   * Placeholders referenced anywhere in the Block's runs — including
   * inside plural / select forms. Enumerated here so validators and
   * CAT tools can examine them without walking the run tree, and so
   * metadata that doesn't fit on a Run (jsType, sourceExpr,
   * optional, icu-pivot kind) has a place to live.
   */
  placeholders: Placeholder[];

  /** Translator-facing context: file, component, element, etc. */
  properties: BlockProperties;

  /** Optional preview hints for Level-2 / Level-3 renders. */
  preview?: BlockPreviewHints;
}

export type BlockType = "jsx:element" | "jsx:attribute" | "js:t";

export type LocaleID = string; // BCP-47 tag, e.g. "de", "ja-JP", "qps"

export interface BlockProperties {
  file: string;
  line: number;
  component: string;
  jsxPath: string;
  element: string;
  locNote?: string;
}

export interface BlockPreviewHints {
  /**
   * Storybook story id if one renders this component. Advisory
   * metadata; tools that want to drive a live preview can use it.
   * The framework only stores the hint.
   */
  storyId?: string;
  /** Path to a pre-rendered snapshot, if an offline render exists. */
  snapshotPath?: string;
  /** Sample values for placeholders, used by skeleton previews. */
  sampleValues?: Record<string, unknown>;
}

// ─── Placeholders ─────────────────────────────────────────────────

/**
 * Enumerates the variables and element tokens referenced by the
 * runs of a Block, including inside plural / select forms. One
 * entry per unique placeholder name. Drives validation (target
 * must preserve every required placeholder) and gives tools
 * metadata they don't want to dig out of the runs tree.
 */
export interface Placeholder {
  /** Matches the `equiv` of the corresponding Run(s), or the `pivot` of a plural/select construct. */
  name: string;
  kind: PlaceholderKind;
  /**
   * Type of the expression at the call site, when known. "number"
   * is a hint that this placeholder may drive plural selection.
   */
  jsType?: "string" | "number" | "boolean" | "Date" | "ReactNode" | string;
  /** Raw source expression, e.g. "user.name". */
  sourceExpr: string;
  /**
   * True when the run was derived from a conditional / logical JSX
   * expression (`a && <X/>`, `a ? <X/> : <Y/>`). Targets may
   * legitimately drop the corresponding run in some languages.
   */
  optional?: boolean;
}

export type PlaceholderKind =
  | "variable" // {name}, {user.name}
  | "element" // an inline JSX element captured as a pair of runs
  | "node" // {cond && <X/>} — ReactNode-valued expression
  | "icu-pivot"; // variable driving a plural / select construct

// ─── Document + File envelope ─────────────────────────────────────

/**
 * The .klf file envelope version. Incremented on non-backward-
 * compatible schema changes. Consumers MUST reject unknown major
 * versions and SHOULD accept unknown minor versions of their major.
 */
export const SchemaVersion = "1.0" as const;

/**
 * The .klf file kind discriminator. Lets a consumer confirm a JSON
 * blob is a KLF document before parsing, rather than a same-shaped
 * structure from a different producer.
 */
export const Kind = "kapi-localization-format" as const;

/** Format of an extracted document's source. */
export type DocumentType = "jsx" | "html" | "markdown" | "generic";

/**
 * Reference to the opaque skeleton payload a merge step consumes to
 * reconstruct the original source file with translated content spliced
 * back in. Mirrors Go `core/klf.Skeleton` (the canonical
 * implementation): a `ref` to an external payload and/or an `inline`
 * payload. Both are optional; at least one is present in practice.
 * Both serialize with omit-when-empty semantics on the Go side, so the
 * canonical serializer drops empty fields here too.
 */
export interface Skeleton {
  /** Reference to an external skeleton payload. Omitted when empty. */
  ref?: string;
  /** Inline skeleton payload. Omitted when empty. */
  inline?: string;
}

/**
 * One extracted source file, with its blocks. A .klf File wraps
 * one or more Documents.
 */
export interface Document {
  id: string;
  documentType: DocumentType;
  path: string;
  sourceHash?: string;
  skeleton?: Skeleton;
  blocks: Block[];
}

/** Identifies the extractor that produced a .klf. */
export interface Generator {
  id: string;
  version: string;
  capabilities?: string[];
}

/** Identifies the project a .klf belongs to. */
export interface Project {
  id: string;
  sourceLocale: LocaleID;
}

/**
 * Vocabulary declarations: the list of vocabulary packs whose span
 * type entries this file's runs expect consumers to have loaded.
 */
export interface Vocabulary {
  extends?: string[];
}

/**
 * Top-level .klf JSON envelope. This is what `marshalFile` emits
 * and what `core/klf.Unmarshal` parses on the Go side. Field order
 * and shape are normative; deterministic serialization is what
 * makes the manifest SHA-256 stable across languages.
 */
export interface File {
  schemaVersion: typeof SchemaVersion;
  kind: typeof Kind;
  created?: string;
  generator: Generator;
  project: Project;
  vocabulary?: Vocabulary;
  documents: Document[];
}
