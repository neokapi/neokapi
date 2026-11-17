/**
 * How a surface shows an inline code to a person. A code carries three
 * strings, and two of them must never reach a reader:
 *
 * - `data` is the native source slice (`<span className="muted">`, `{count}`),
 *   kept for the round trip.
 * - `equiv` is the code's identifier for tools. Extractors synthesize it when
 *   the source has no name, and a synthesized one is internal: the React
 *   extractor names an element `=m0`, which a review card once drew as
 *   `<=m0><serverURL/></=m0>`.
 *
 * A `CodeDisplay` holds only what is safe to show: the code's role, a kind a
 * surface words in its own language, and a name when the code has a human
 * one (a variable such as `serverURL`). It is branded, so a renderer typed to
 * take one cannot be handed a raw string, and `codeDisplays` is the only way
 * to make one. No field ever holds markup: a name is letters, digits and a
 * few separators, and a code without such a name shows its kind.
 */
import type { Run } from "./block.ts";
import { projectRuns, type ModelRunSpec } from "./run-projection.ts";

/** What part a code plays in the sequence. */
export type CodeRole =
  /** A self-closing code: a variable, a line break, an icon. */
  | "placeholder"
  /** The opening half of a paired code, wrapping content. */
  | "open"
  /** The closing half of a paired code. */
  | "close"
  /** A reference to a subblock held elsewhere. */
  | "reference"
  /** The variable a plural or select chooses its form by. */
  | "choice";

/**
 * What a code is, for a surface to word: `link`, `bold`, `italic`, `code`,
 * `break`, `image`, `variable`, `content` (an optional piece of UI), `element`
 * (any other markup) or `unknown` (a run kind the surface does not know).
 */
export type CodeKind =
  | "link"
  | "bold"
  | "italic"
  | "code"
  | "break"
  | "image"
  | "variable"
  | "content"
  | "element"
  | "unknown";

declare const codeDisplayBrand: unique symbol;

/** A code as a person sees it. Made only by `codeDisplays`. */
export interface CodeDisplay {
  readonly role: CodeRole;
  readonly kind: CodeKind;
  /**
   * The code's human name, such as a variable name, when it has one. Absent
   * for a code whose only names are internal or markup: the surface then
   * words `kind`.
   */
  readonly name?: string;
  readonly [codeDisplayBrand]: true;
}

/**
 * A name a person can read: it starts with a letter and holds only letters,
 * digits, spaces, `_`, `.` and `-`, at most 40 characters. Anything else,
 * markup and synthesized identifiers included, is not a name.
 */
const HUMAN_NAME = /^\p{L}[\p{L}\p{N} _.-]{0,39}$/u;

/**
 * The delimiters formats wrap a variable's name in: a vocabulary's [B],
 * {{name}} and {{.Name}}, {name}, ${name}, %(name)s and :name. Only the name
 * inside is kept, and it still has to pass as a human name.
 */
const WRAPPED_NAME = [
  /^\[\/?(.+)\]$/,
  /^\{\{\s*\.?(.+?)\s*\}\}$/,
  /^\$\{(.+)\}$/,
  /^\{(.+)\}$/,
  /^%\((.+)\)[a-z]$/,
  /^:(.+)$/,
];

/** The human name among candidates, with the delimiters around a name dropped. */
export function humanCodeName(...candidates: (string | undefined)[]): string | undefined {
  for (const c of candidates) {
    if (!c) continue;
    const raw = c.trim();
    let name = raw;
    for (const re of WRAPPED_NAME) {
      const m = re.exec(raw);
      if (m) {
        name = m[1];
        break;
      }
    }
    if (HUMAN_NAME.test(name)) return name;
  }
  return undefined;
}

const TAG_KINDS: Record<string, CodeKind> = {
  a: "link",
  link: "link",
  strong: "bold",
  b: "bold",
  em: "italic",
  i: "italic",
  code: "code",
  kbd: "code",
  samp: "code",
  br: "break",
  img: "image",
  image: "image",
};

/** The kind of a code from its vocabulary type and subtype. */
export function codeKind(type: string, subType?: string): CodeKind {
  const tag = (subType ?? type.split(":").pop() ?? "").toLowerCase();
  if (TAG_KINDS[tag]) return TAG_KINDS[tag];
  if (/(^|:)(var|variable|param|placeholder)$/.test(type)) return "variable";
  if (/(^|:)node$/.test(type)) return "content";
  if (/(^|:)(link|hyperlink)$/.test(type)) return "link";
  if (/(^|:)(bold|strong)$/.test(type)) return "bold";
  if (/(^|:)(italic|emphasis)$/.test(type)) return "italic";
  if (/(^|:)(break|lb|linebreak)$/.test(type)) return "break";
  if (/(^|:)(image|img)$/.test(type)) return "image";
  return "element";
}

function make(role: CodeRole, kind: CodeKind, name?: string): CodeDisplay {
  return (name ? { role, kind, name } : { role, kind }) as CodeDisplay;
}

/**
 * Name a variable or element code: its display label or identifier when one
 * is a human name. An element's tag is not a name; it is the kind.
 */
function codeName(
  kind: CodeKind,
  disp?: string,
  equiv?: string,
  data?: string,
): string | undefined {
  const name = humanCodeName(disp, equiv, data);
  if (!name) return undefined;
  // An identifier that only repeats the tag (`a`, `strong`, `br`) adds
  // nothing to the kind.
  return TAG_KINDS[name.toLowerCase()] === kind ? undefined : name;
}

/** The display of every code in one run sequence, keyed as the runs index it. */
export interface CodeDisplays {
  ph(ph: {
    id: string;
    type: string;
    subType?: string;
    equiv?: string;
    disp?: string;
    data?: string;
  }): CodeDisplay;
  pcOpen(pc: {
    id: string;
    type: string;
    subType?: string;
    equiv?: string;
    disp?: string;
  }): CodeDisplay;
  pcClose(pc: { id: string; type: string; subType?: string; equiv?: string }): CodeDisplay;
  sub(sub: { id: string; equiv?: string }): CodeDisplay;
  choice(pivot: string): CodeDisplay;
  unknown(): CodeDisplay;
}

const OPENINGS: ModelRunSpec<readonly [string, CodeDisplay] | null> = {
  text: { dropped: "text is no code" },
  ph: { dropped: "a placeholder has no closing half to name" },
  pcOpen: ({ pcOpen }) => {
    const kind = codeKind(pcOpen.type, pcOpen.subType);
    return [pcOpen.id, make("close", kind, codeName(kind, pcOpen.disp, pcOpen.equiv))];
  },
  pcClose: { dropped: "the closing half takes its opening half's display" },
  sub: { dropped: "a subblock reference has no closing half" },
  plural: { dropped: "a form's pairs are named with the form" },
  select: { dropped: "a case's pairs are named with the case" },
  fallback: () => null,
};

/** The displays of the codes in one run sequence. */
export function codeDisplays(runs: readonly Run[]): CodeDisplays {
  const closings = new Map(
    projectRuns(runs, OPENINGS).filter((e): e is readonly [string, CodeDisplay] => e !== null),
  );
  return {
    ph: (ph) => {
      // A placeholder with no type stands for a value.
      const kind = ph.type || ph.subType ? codeKind(ph.type, ph.subType) : "variable";
      // A code finder's placeholder carries its name only in its native form
      // (i18next's `{{name}}`); humanCodeName takes the name from inside it
      // and rejects anything else.
      return make("placeholder", kind, codeName(kind, ph.disp, ph.equiv, ph.data));
    },
    pcOpen: (pc) => {
      const kind = codeKind(pc.type, pc.subType);
      return make("open", kind, codeName(kind, pc.disp, pc.equiv));
    },
    pcClose: (pc) => {
      const known = closings.get(pc.id);
      if (known) return known;
      const kind = codeKind(pc.type, pc.subType);
      return make("close", kind, codeName(kind, undefined, pc.equiv));
    },
    sub: (sub) => make("reference", "content", humanCodeName(sub.equiv)),
    choice: (pivot) => make("choice", "variable", humanCodeName(pivot)),
    unknown: () => make("placeholder", "unknown"),
  };
}

/**
 * The English word for a code with no human name. A surface with a catalog
 * words `kind` itself; this is the default for surfaces that write their
 * labels in English source.
 */
export const CODE_KIND_WORDS: Readonly<Record<CodeKind, string>> = {
  link: "link",
  bold: "bold",
  italic: "italic",
  code: "code",
  break: "line break",
  image: "image",
  variable: "value",
  content: "content",
  element: "styled text",
  unknown: "code",
};

/** What a chip says for a code: its human name, else the word for its kind. */
export function codeWord(d: CodeDisplay): string {
  return d.name ?? CODE_KIND_WORDS[d.kind];
}
