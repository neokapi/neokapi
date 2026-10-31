// Reader fixtures: one file of the Fernwell sample as ReaderFile answers it,
// in the states the page draws. Shared by the stories and the tests.

import type { ReaderFile, ReaderRule } from "../../types";

/** The context answer for docs/billing.md, in the order `kapi context` prints it. */
export const BILLING_RULES: ReaderRule[] = [
  {
    kind: "tone",
    rule: "Plain and direct. Second person, short sentences.",
    status: "established",
    from: "voice fernwell",
  },
  { kind: "word", rule: "Sign in, not Login", status: "established", from: "terms" },
  { kind: "name", rule: "Fernwell, not Fern Well", status: "established", from: "terms" },
  { kind: "word", rule: "Avoid utilize", status: "established", from: "pack technical-docs" },
  {
    kind: "note",
    rule: "Address the reader as you.",
    status: "established",
    by: "ada",
    at: "2026-10-20T09:00:00Z",
  },
  {
    kind: "word",
    rule: "plan, not tier",
    status: "suggested",
    by: "claude-code",
    at: "2026-10-28T14:10:00Z",
  },
];

/** Findings with fixes: two term rules with a replacement, and a voice rule. */
export const READER_WITH_FIXES: ReaderFile = {
  path: "docs/billing.md",
  abs: "/work/fernwell/docs/billing.md",
  format: "markdown",
  verdict: "failed",
  findings: 3,
  fixable: 2,
  rules: BILLING_RULES,
  rules_total: BILLING_RULES.length,
  attention: [],
  refused: [],
  document: [],
  blocks: [
    { id: "p1", runs: [{ text: "Billing" }], findings: [] },
    {
      id: "p2",
      runs: [{ text: "Login to see your invoices, then utilize the export to download them." }],
      findings: [
        {
          id: "a1f0-1",
          block_id: "p2",
          position: { kind: "range", start: { run: 0, offset: 0 }, end: { run: 0, offset: 5 } },
          span: { start: 0, end: 5 },
          kind: "word",
          label: "Sign in, not Login",
          fails: true,
          fixable: true,
        },
        {
          id: "b2c3-1",
          block_id: "p2",
          position: { kind: "range", start: { run: 0, offset: 33 }, end: { run: 0, offset: 40 } },
          span: { start: 33, end: 40 },
          kind: "word",
          label: "use, not utilize",
          fails: true,
          fixable: true,
        },
      ],
    },
    {
      id: "p3",
      runs: [
        { text: "Your credits reset on " },
        { ph: { id: "1", type: "var", data: "{date}", equiv: "date" } },
        { text: ". We would be absolutely delighted to help you plan ahead." },
      ],
      findings: [
        {
          id: "c4d5-1",
          block_id: "p3",
          position: { kind: "range", start: { run: 2, offset: 14 }, end: { run: 2, offset: 34 } },
          span: { start: 36, end: 56 },
          kind: "tone",
          label: "Prohibited pattern: no intensifiers",
          fails: false,
          fixable: false,
        },
      ],
    },
  ],
};

/** Findings without fixes: a voice rule and a placeholder missing from a translation. */
export const READER_WITHOUT_FIXES: ReaderFile = {
  ...READER_WITH_FIXES,
  findings: 2,
  fixable: 0,
  blocks: [
    READER_WITH_FIXES.blocks[0],
    {
      id: "p2",
      runs: [{ text: "Sign in to see your invoices." }],
      findings: [],
    },
    {
      ...READER_WITH_FIXES.blocks[2],
      findings: [
        ...READER_WITH_FIXES.blocks[2].findings,
        {
          id: "e6f7-1",
          block_id: "p3",
          position: { kind: "run", runId: "1" },
          kind: "placeholder",
          label: "Placeholder {date} is missing",
          fails: true,
          fixable: false,
          locale: "fr",
        },
      ],
    },
  ],
};

/** No findings: the blocks and the margin. */
export const READER_CLEAN: ReaderFile = {
  ...READER_WITH_FIXES,
  verdict: "passed",
  findings: 0,
  fixable: 0,
  blocks: [
    READER_WITH_FIXES.blocks[0],
    { id: "p2", runs: [{ text: "Sign in to see your invoices." }], findings: [] },
    {
      id: "p3",
      runs: [
        { text: "Your credits reset on " },
        { ph: { id: "1", type: "var", data: "{date}", equiv: "date" } },
        { text: "." },
      ],
      findings: [],
    },
  ],
};

/** A fix the change service refused, with its reason, under the finding it was for. */
export const READER_REFUSED: ReaderFile = {
  ...READER_WITH_FIXES,
  refused: [
    {
      finding_id: "b2c3-1",
      reason:
        'the edit introduces 1 failing finding in docs/billing.md#p2, listed with the document\'s findings: Prohibited pattern: Write "use"',
    },
  ],
};
