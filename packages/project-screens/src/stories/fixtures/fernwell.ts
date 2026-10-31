// The Fernwell sample's suggestions and its one disagreement, as the digest
// holds them. The Map's rows and the desktop's digest fixtures share them, so
// a row's Decide lands on the same item everywhere.

import type { DigestConflict, DigestItem } from "../../types";

const hour = 3_600_000;
const ago = (hours: number) => new Date(Date.now() - hours * hour).toISOString();

/** The Fernwell project's key. */
export const FERNWELL_PROJECT = "prj_fernwellaaaaaaaaaaaaaa";

export function digestItem(over: Partial<DigestItem> & { id: string }): DigestItem {
  return {
    short: over.id.slice(0, 10),
    status: "suggested",
    theme: "words",
    sentence: "",
    subject: { kind: "term" },
    noticed_by: { kind: "agent", name: "claude", session: "s1" },
    at: ago(5),
    new: true,
    scope: "project",
    keepable: true,
    droppable: true,
    widenable: false,
    ...over,
  };
}

export const SIGN_IN = digestItem({
  id: "0njy7pwmtj00000000000000",
  status: "contested",
  subject: { kind: "term", term: { term: "login", replacement: "sign in" } },
  quote: { path: "docs/account.md", quote: "Login to see your invoices." },
  noticed_by: { kind: "agent", name: "claude", session: "s3" },
});
export const LOG_IN = digestItem({
  id: "0njy7pxnst00000000000000",
  status: "contested",
  subject: { kind: "term", term: { term: "login", replacement: "log in" } },
  quote: { path: "app/strings/en.json", quote: "Login" },
  noticed_by: { kind: "agent", name: "codex", session: "s4" },
});

/** Two assistants disagree about one word. */
export const LOGIN_CONFLICT: DigestConflict = {
  sides: [SIGN_IN, LOG_IN],
  by_evidence: false,
  reason:
    "These rules say different things about the same word. Choose one, and the others are set aside.",
};

export const QUICKCAST = digestItem({
  id: "0njy7pfhjp00000000000000",
  theme: "names",
  subject: {
    kind: "term",
    term: { term: "Quick cast", replacement: "Quickcast", forms: ["QuickCast", "Quick-cast"] },
  },
  quote: { path: "docs/intro.md", quote: "Try Quick cast for your next session." },
  collection: "docs",
  standing: "seen in 2 sessions · applied in 3 agent edits",
});
export const FERNWELL = digestItem({
  id: "0njy7pfzzz00000000000000",
  theme: "names",
  subject: { kind: "term", term: { term: "FernWell", replacement: "Fernwell" } },
  quote: { path: "app/strings/en.json", quote: "Welcome to FernWell" },
  collection: "app",
  new: false,
  at: ago(90),
});
export const CUSTOMER = digestItem({
  id: "0njy7qcust00000000000000",
  theme: "words",
  subject: { kind: "term", term: { term: "customer", replacement: "member" } },
  quote: { path: "docs/billing.md", quote: "Each customer gets a receipt." },
  collection: "docs",
  standing: "seen in 2 sessions",
});
export const ADDRESS = digestItem({
  id: "0njy7qaddr00000000000000",
  theme: "writing",
  subject: { kind: "note", text: "The docs address the reader as you, never as the user." },
  sentence: "The docs address the reader as you, never as the user.",
  keepable: false,
  collection: "docs",
});

/** The suggestions in the order the digest lists them: names, words, then how the project writes. */
export const FERNWELL_SUGGESTED: DigestItem[][] = [[FERNWELL, QUICKCAST], [CUSTOMER], [ADDRESS]];
