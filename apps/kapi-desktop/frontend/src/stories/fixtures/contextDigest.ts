// A digest for Fernwell, a studio-software project: a conflict about one
// word, rules that came into force on a merge and a correction, suggestions
// in two themes and two collections, and one rule the content is drifting
// from.

import type { ContextDigest, DigestItem } from "../../types/api";

const hour = 3_600_000;
const ago = (hours: number) => new Date(Date.now() - hours * hour).toISOString();

/** The marker: the person last looked two days ago. */
export const LAST_LOOKED = ago(48);

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
    revertible: false,
    ...over,
  };
}

const SIGN_IN = digestItem({
  id: "0njy7pwmtj00000000000000",
  status: "contested",
  subject: { kind: "term", term: { term: "login", replacement: "sign in" } },
  quote: { path: "docs/account.md", quote: "Login to see your invoices." },
  noticed_by: { kind: "agent", name: "claude", session: "s3" },
});
const LOG_IN = digestItem({
  id: "0njy7pxnst00000000000000",
  status: "contested",
  subject: { kind: "term", term: { term: "login", replacement: "log in" } },
  quote: { path: "app/strings/en.json", quote: "Login" },
  noticed_by: { kind: "agent", name: "codex", session: "s4" },
});

const STUDIO = digestItem({
  id: "0njy7pghcn00000000000000",
  status: "established",
  subject: { kind: "term", term: { term: "business", replacement: "studio" } },
  quote: { path: "docs/billing.md", quote: "Upgrade your business plan." },
  standing: "seen in 3 sessions · 41 of 43 uses in docs/ · merged in #412",
  usage: {
    preferred: "studio",
    preferred_count: 41,
    rejected: ["business"],
    rejected_count: 2,
    within: "docs/",
    line: 'docs/ says "studio" 41 times and "business" twice',
  },
  established_at: ago(20),
  how: ["merged in #412", "your correction in docs/billing.md"],
  keepable: false,
  droppable: false,
  revertible: true,
});
const WORKSPACE = digestItem({
  id: "0njy6aaaaa00000000000000",
  status: "established",
  new: false,
  subject: { kind: "term", term: { term: "project folder", replacement: "workspace" } },
  established_at: ago(120),
  how: ["kept by you"],
  keepable: false,
  droppable: false,
  revertible: true,
});

const QUICKCAST = digestItem({
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
const FERNWELL = digestItem({
  id: "0njy7pfzzz00000000000000",
  theme: "names",
  subject: { kind: "term", term: { term: "FernWell", replacement: "Fernwell" } },
  quote: { path: "app/strings/en.json", quote: "Welcome to FernWell" },
  collection: "app",
  new: false,
  at: ago(90),
});
const CUSTOMER = digestItem({
  id: "0njy7qcust00000000000000",
  theme: "words",
  subject: { kind: "term", term: { term: "customer", replacement: "member" } },
  quote: { path: "docs/billing.md", quote: "Each customer gets a receipt." },
  collection: "docs",
  standing: "seen in 2 sessions",
});
const ADDRESS = digestItem({
  id: "0njy7qaddr00000000000000",
  theme: "writing",
  subject: { kind: "note", text: "The docs address the reader as you, never as the user." },
  sentence: "The docs address the reader as you, never as the user.",
  keepable: false,
  collection: "docs",
});

const RECORDING = digestItem({
  id: "0njy5recor00000000000000",
  status: "established",
  new: false,
  subject: { kind: "term", term: { term: "take", replacement: "recording" } },
  established_at: ago(400),
  how: ["kept by you"],
  keepable: false,
  droppable: false,
  revertible: true,
  usage: {
    preferred: "recording",
    preferred_count: 11,
    rejected: ["take"],
    rejected_count: 6,
    within: "docs/",
    line: 'docs/ says "recording" 11 times and "take" 6 times',
  },
});

/** Every section at once. */
export const CONTEXT_DIGEST: ContextDigest = {
  project: "prj_fernwellaaaaaaaaaaaaaa",
  project_name: "Fernwell",
  since: LAST_LOOKED,
  conflicts: [
    {
      sides: [SIGN_IN, LOG_IN],
      by_evidence: false,
      reason:
        "These rules say different things about the same word. Choose one, and the others are set aside.",
    },
  ],
  established: [STUDIO, WORKSPACE],
  suggested: [
    {
      theme: "names",
      title: "Names and spellings",
      groups: [
        { collection: "app", items: [FERNWELL] },
        { collection: "docs", items: [QUICKCAST] },
      ],
    },
    { theme: "words", title: "Words to avoid", groups: [{ items: [CUSTOMER] }] },
    { theme: "writing", title: "How the project writes", groups: [{ items: [ADDRESS] }] },
  ],
  drift: [{ rule: RECORDING, line: RECORDING.usage!.line, rejected: 6, before: 1 }],
  numbers: { rules: 23, new_this_week: 4, suggested: 4, conflicts: 1, new: 5 },
};

/** Nothing new since the person last looked. */
export const QUIET_DIGEST: ContextDigest = {
  project: "prj_fernwellaaaaaaaaaaaaaa",
  project_name: "Fernwell",
  since: LAST_LOOKED,
  conflicts: [],
  established: [WORKSPACE],
  suggested: [{ theme: "names", title: "Names and spellings", groups: [{ items: [FERNWELL] }] }],
  drift: [],
  numbers: { rules: 23, new_this_week: 0, suggested: 1, conflicts: 0, new: 0 },
};

/** A project nobody has recorded anything about. */
export const EMPTY_DIGEST: ContextDigest = {
  project: "prj_fernwellaaaaaaaaaaaaaa",
  project_name: "Fernwell",
  conflicts: [],
  established: [],
  suggested: [],
  drift: [],
  numbers: { rules: 0, new_this_week: 0, suggested: 0, conflicts: 0, new: 0 },
};

/** One section of the full digest, the rest emptied. */
export function onlySection(
  section: "conflicts" | "established" | "suggested" | "drift",
): ContextDigest {
  return {
    ...CONTEXT_DIGEST,
    conflicts: section === "conflicts" ? CONTEXT_DIGEST.conflicts : [],
    established: section === "established" ? CONTEXT_DIGEST.established : [],
    suggested: section === "suggested" ? CONTEXT_DIGEST.suggested : [],
    drift: section === "drift" ? CONTEXT_DIGEST.drift : [],
  };
}
