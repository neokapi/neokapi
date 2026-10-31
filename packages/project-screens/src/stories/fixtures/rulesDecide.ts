// The questions Decide asks about the KapiMart sample, in the shape
// RulesDecideView answers with: a name written three ways, a word the files
// avoid, two assistants disagreeing about one word, and a note.

import type { RuleDecision, RulesDecideView } from "../../types";

export const DECIDE_IDS = {
  quickcast: "0njy7pfhjp00000000000000",
  business: "0njy7pfhjp00000000000001",
  market: "0njy7pfhjp00000000000002",
  store: "0njy7pfhjp00000000000003",
  tone: "0njy7pfhjp00000000000004",
} as const;

const claude = { kind: "agent", name: "claude-code", session: "sess-1" };
const codex = { kind: "agent", name: "codex", session: "sess-2" };

export const NAME_DECISION: RuleDecision = {
  id: DECIDE_IDS.quickcast,
  short: "0njy7pfhjp",
  kind: "name",
  question: "Which one do your files use?",
  sentence: 'Write "Quickcast", not "Quick cast" or "QuickCast"',
  label: "Quickcast",
  avoided: 0,
  options: [
    {
      form: "Quickcast",
      blocks: 12,
      files: 5,
      consequence:
        'If you keep "Quickcast": the other 2 forms become findings in 4 blocks. Applies to the whole project.',
    },
    {
      form: "Quick cast",
      blocks: 3,
      files: 2,
      consequence:
        'If you keep "Quick cast": the other 2 forms become findings in 13 blocks. Applies to the whole project.',
    },
    {
      form: "QuickCast",
      blocks: 1,
      files: 1,
      consequence:
        'If you keep "QuickCast": the other 2 forms become findings in 15 blocks. Applies to the whole project.',
    },
  ],
  chosen: 0,
  other: true,
  seen: { path: "site/products/index.html", quote: "Try Quick cast today." },
  applies: "the whole project",
  noticed_by: claude,
};

export const WORD_DECISION: RuleDecision = {
  id: DECIDE_IDS.business,
  short: "0njy7pfhjq",
  kind: "word",
  question: "Keep this as a rule?",
  sentence: 'Write "studio", not "business"',
  label: "business",
  avoided: 1,
  options: [],
  chosen: 0,
  other: false,
  consequence:
    'If you keep it: "business" becomes a finding in 3 blocks. Applies to the whole project.',
  seen: { path: "site/about.html", quote: "Our business plan." },
  applies: "the whole project",
  noticed_by: claude,
};

export const DISAGREEMENT: RuleDecision = {
  kind: "disagreement",
  question: "Which one holds?",
  reason: "two suggestions replace one word with different words",
  sentence: "",
  label: "shop",
  avoided: 1,
  options: [
    {
      id: DECIDE_IDS.store,
      short: "0njy7pfhjs",
      sentence: 'Write "store", not "shop"',
      blocks: 6,
      files: 3,
      consequence:
        'If you keep "store": "shop" becomes a finding in 2 blocks. Applies to the whole project.',
      noticed_by: claude,
    },
    {
      id: DECIDE_IDS.market,
      short: "0njy7pfhjr",
      sentence: 'Write "market", not "shop"',
      blocks: 1,
      files: 1,
      consequence:
        'If you keep "market": "shop" becomes a finding in 2 blocks. Applies to the whole project.',
      noticed_by: codex,
    },
  ],
  chosen: 0,
  other: false,
  seen: { path: "site/index.html", quote: "Visit our shop." },
  applies: "the whole project",
  noticed_by: claude,
};

export const NOTE_DECISION: RuleDecision = {
  id: DECIDE_IDS.tone,
  short: "0njy7pfhjt",
  kind: "note",
  question: "Keep this as a rule?",
  sentence: "Address the reader as you.",
  label: "Address the reader as you.",
  avoided: 0,
  options: [],
  chosen: 0,
  other: false,
  consequence:
    "If you keep it: it is stated to writers and assistants where it applies. No check reads it. Applies to the whole project.",
  applies: "the whole project",
  noticed_by: { kind: "person" },
};

/** KapiMart with four questions waiting. */
export const DECIDE_VIEW: RulesDecideView = {
  project: "kapimart",
  project_name: "KapiMart",
  decisions: [DISAGREEMENT, NAME_DECISION, WORD_DECISION, NOTE_DECISION],
  rules: 14,
  suggested: 5,
  narrowed: 0,
};

/** One question: a name with its forms. */
export const NAME_VIEW: RulesDecideView = {
  ...DECIDE_VIEW,
  decisions: [NAME_DECISION],
  suggested: 1,
};

/** One question: a yes-or-no rule. */
export const WORD_VIEW: RulesDecideView = {
  ...DECIDE_VIEW,
  decisions: [WORD_DECISION],
  suggested: 1,
};

/** Nothing waits. */
export const EMPTY_DECIDE_VIEW: RulesDecideView = {
  ...DECIDE_VIEW,
  decisions: [],
  suggested: 0,
};

/** A name whose forms run long, for truncation. */
export const LONG_VIEW: RulesDecideView = {
  ...DECIDE_VIEW,
  decisions: [
    {
      ...NAME_DECISION,
      label: "KapiMart Quickcast Live Streaming Studio for Independent Retailers",
      options: NAME_DECISION.options.map((o) => ({
        ...o,
        form: `${o.form} Live Streaming Studio for Independent Retailers and Their Customers`,
      })),
      seen: {
        path: "site/products/streaming/independent-retailers/quickcast-live-studio.html",
        quote:
          "Try Quick cast Live Streaming Studio for Independent Retailers today, and bring your customers into the shop wherever they are.",
      },
    },
  ],
};
