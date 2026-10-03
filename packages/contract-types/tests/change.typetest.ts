// Type-level tests of the change contract (change.gen.ts). `vp check` compiles
// this file; nothing runs it. Each `@ts-expect-error` is a change set the Go
// decoder refuses, so the types must refuse it too, and an unused directive
// fails the check.
import {
  CHANGE_ERROR_EXIT_CODE,
  CHANGE_ERROR_HTTP_STATUS,
  CHANGE_OP_KINDS,
  CHANGE_RESULT_SCHEMA_ID,
  CHANGE_SCHEMA_ID,
  type BlockRead,
  type ChangeErrorCode,
  type ChangeOp,
  type ChangeOpKind,
  type ChangeOpStatus,
  type ChangeRef,
  type ChangeResult,
  type ChangeSet,
  type FormatDescription,
  type OpResult,
  type ReadPage,
  type ResultOpKind,
} from "../src/index.ts";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends <T>() => T extends B ? 1 : 2 ? true : false;
type Expect<T extends true> = T;

const at: ChangeRef = { doc: "docs/guide.html", block: "p" };
const rev = "r:3f9a1c0e7b2d4a55";

export type Checks = [
  // Every operation the schema lists is a kind the results name, and no other.
  Expect<Equal<ChangeOp["op"], ChangeOpKind>>,
  Expect<Equal<(typeof CHANGE_OP_KINDS)[number], ChangeOpKind>>,
  // An operation's result may name a kind a tool applies in process; a block
  // accepts only the kinds a change set carries.
  Expect<Equal<OpResult["op"], ResultOpKind>>,
  Expect<Equal<Exclude<ResultOpKind, ChangeOpKind>, "provenance">>,
  Expect<Equal<BlockRead["ops"], ChangeOpKind[]>>,
  // A result names its own schema.
  Expect<Equal<ChangeResult["schema"], "kapi.change-result/v1">>,
  Expect<Equal<typeof CHANGE_RESULT_SCHEMA_ID, "kapi.change-result/v1">>,
  // Every refusal has a transport mapping.
  Expect<Equal<keyof typeof CHANGE_ERROR_HTTP_STATUS, ChangeErrorCode>>,
  Expect<Equal<keyof typeof CHANGE_ERROR_EXIT_CODE, ChangeErrorCode>>,
];

// The operations of the edit model's examples (edit-model section 2.3).
export const examples: ChangeSet = {
  schema: CHANGE_SCHEMA_ID,
  mode: "preview",
  gate: "enforce",
  note: "Point the guide link at the handbook and add the Norwegian edition",
  evidence: [{ url: "https://example.com/issue/42" }],
  ops: [
    {
      op: "set_content",
      at,
      if_match: rev,
      text: 'Read the <x id="1"/>handbook<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.',
    },
    { op: "replace_text", at, if_match: rev, edits: [{ find: "shop guide", text: "handbook" }] },
    { op: "replace_text", at, if_match: rev, edits: [{ start: 9, end: 19, text: "handbook" }] },
    {
      op: "replace_text",
      at,
      if_match: rev,
      edits: [
        { range: { start: { run: 2, offset: 0 }, end: { run: 2, offset: 10 } }, text: "handbook" },
      ],
    },
    {
      op: "set_attribute",
      at,
      if_match: rev,
      code: "1",
      name: "href",
      value: "https://new.example/handbook",
    },
    { op: "mark", at, if_match: rev, range: { find: "before you" }, type: "fmt:bold" },
    {
      op: "set_content",
      at: { ...at, edition: "nb" },
      if_match: "absent",
      basis: rev,
      text: 'Les <x id="1"/>håndboka<x id="/1"/> før du <x id="2"/>bestiller<x id="/2"/>.',
    },
    {
      op: "replace_text",
      at: { doc: "locales/en.json", block: "cart.items" },
      if_match: "r:77c0a1d2e3f40516",
      edits: [{ path: [1, { plural: "one" }], find: "item", text: "article" }],
    },
    { op: "remove_edition", at: { ...at, edition: "de" }, if_match: "r:0c55e1f2a3b4c5d6" },
    {
      op: "annotate",
      at,
      type: "note",
      id: "n1",
      anchor: { kind: "range", start: { run: 2, offset: 0 }, end: { run: 2, offset: 10 } },
      value: { text: "Is it a handbook or a guide?" },
    },
    { op: "unannotate", at, type: "note", id: "n1" },
    {
      op: "insert_block",
      doc: "locales/en.json",
      after: "nav.cart",
      name: "nav.checkout",
      editions: { en: { text: "Checkout" } },
    },
    {
      op: "delete_block",
      at: { doc: "locales/en.json", block: "nav.legacy" },
      if_match: { en: "r:5e10a2b3c4d5e6f7" },
    },
    {
      op: "decide",
      at: { doc: "docs/guide.md", block: "install/p", edition: "fr" },
      if_match: "r:74dbfac2ed1c9d6e",
      outcome: "advise",
      score: 80,
      reasons: ["terminology"],
    },
    { op: "term", action: "upsert", term: "handbook", status: "preferred", replaces: "shop guide" },
    {
      op: "native",
      doc: "report.docx",
      if_match: "sha256:9c1e5b2f7a8d4c3e6f1a0b9d8c7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e",
      name: "docx.append_paragraph",
      args: { runs: [{ text: "Results" }], style: "Heading2" },
    },
    {
      op: "set_content",
      at,
      if_match: "*",
      runs: [
        { text: "You have " },
        {
          plural: {
            pivot: "count",
            forms: { "=0": [{ text: "nothing" }], one: [{ text: "one item" }] },
          },
        },
        { pcOpen: { id: "1", type: "fmt:bold" } },
        { text: "now", noTranslate: true },
        { pcClose: { id: "1" } },
      ],
    },
  ],
};

// A read hands an operation everything it needs: the reference, the revision,
// and the path to a plural's branch.
export function editFromRead(read: BlockRead): ChangeOp[] {
  const ops: ChangeOp[] = [
    { op: "set_content", at: read.ref, if_match: read.rev, text: read.text },
  ];
  for (const s of read.structures ?? []) {
    ops.push({ op: "set_content", at: read.ref, if_match: read.rev, path: s.path, text: "x" });
  }
  return ops;
}

// A stale refusal carries what a resend needs.
export function rebase(op: OpResult, page: ReadPage): ChangeOp | undefined {
  const read = page.blocks[0];
  if (op.error?.code !== "stale" || op.current === undefined || read === undefined)
    return undefined;
  return { op: "set_content", at: read.ref, if_match: op.current.rev, text: op.current.text ?? "" };
}

// A change set refused as a whole (it did not decode) is a result like any
// other, so one parse covers every answer: its error says why, and its docs
// and ops are empty.
export const refusedWhole: ChangeResult = {
  schema: CHANGE_RESULT_SCHEMA_ID,
  status: "refused",
  record: null,
  docs: [],
  ops: [],
  error: { code: "invalid", pointer: "/ops/0/wording", message: 'unknown field "wording"' },
};

// Why a result was refused: the set's error, else the first refused operation's.
export function refusal(r: ChangeResult): ChangeErrorCode | undefined {
  return r.error?.code ?? r.ops.find((op) => op.status === "refused")?.error?.code;
}

// Every operation status is handled, and a new one would fail this switch.
export function settled(status: ChangeOpStatus): boolean {
  switch (status) {
    case "applied":
    case "unchanged":
    case "previewed":
      return true;
    case "refused":
    case "not_applied":
      return false;
    default: {
      const never: never = status;
      return never;
    }
  }
}

// A format that writes no attribute says so with null.
export function writesAttributes(d: FormatDescription): boolean {
  return d.ops.set_attribute !== null && d.ops.insert_block !== undefined;
}

export const refused: ChangeOp[] = [
  // @ts-expect-error set_content takes exactly one of text or runs
  { op: "set_content", at, if_match: rev, text: "a", runs: [{ text: "a" }] },
  // @ts-expect-error set_content without content
  { op: "set_content", at, if_match: rev },
  // @ts-expect-error set_content needs a precondition
  { op: "set_content", at, text: "a" },
  {
    op: "replace_text",
    at,
    if_match: rev,
    // @ts-expect-error an edit names its text one way only
    edits: [{ find: "a", start: 0, end: 1, text: "b" }],
  },
  {
    op: "set_content",
    at,
    if_match: rev,
    // @ts-expect-error a runs payload never carries native data
    runs: [{ ph: { id: "1", data: "<br/>" } }],
  },
  {
    op: "replace_text",
    at,
    if_match: rev,
    // @ts-expect-error a plural form is a CLDR category or =N
    edits: [{ path: [1, { plural: "several" }], find: "a", text: "b" }],
  },
  {
    op: "replace_text",
    at,
    if_match: rev,
    // @ts-expect-error a path step names a plural form or a select case, never both
    edits: [{ path: [1, { plural: "one", select: "male" }], find: "a", text: "b" }],
  },
  {
    op: "set_content",
    at,
    if_match: rev,
    // @ts-expect-error a run is text or one code, never both
    runs: [{ text: "a", ph: { id: "1" } }],
  },
  {
    op: "set_content",
    at,
    if_match: rev,
    // @ts-expect-error a run is one code
    runs: [{ ph: { id: "1" }, pcOpen: { id: "2" } }],
  },
  {
    op: "set_content",
    at,
    if_match: rev,
    // @ts-expect-error noTranslate goes with a text run
    runs: [{ ph: { id: "1" }, noTranslate: true }],
  },
  // @ts-expect-error delete_block addresses a block, never an edition
  { op: "delete_block", at: { ...at, edition: "fr" }, if_match: { en: rev } },
  // @ts-expect-error insert_block names the document, not a block
  { op: "insert_block", at, editions: { en: { text: "x" } } },
  // @ts-expect-error a decision outcome is establish, reject, withdraw or advise
  { op: "decide", at, if_match: rev, outcome: "approve" },
  // @ts-expect-error no such operation
  { op: "rewrite", at, if_match: rev, text: "a" },
];

// @ts-expect-error the transport says who sent a change set; it has no actor field
export const withActor: ChangeSet = { actor: "agent", ops: [] };

// @ts-expect-error a result's reference may leave the block out; an operation's may not
export const fromResult = (r: OpResult): ChangeRef | undefined => r.at;
