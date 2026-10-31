// A digest for Fernwell, a studio-software project: a conflict about one
// word, rules that came into force on a merge and a correction, suggestions
// in two themes and two collections, and one rule the content is drifting
// from.

import {
  ADDRESS,
  CUSTOMER,
  FERNWELL,
  LOGIN_CONFLICT,
  QUICKCAST,
  digestItem,
} from "@neokapi/project-screens/fixtures";
import type { ContextDigest, DigestItem, DigestTermRule } from "../../types/api";

export { digestItem };

const hour = 3_600_000;
const ago = (hours: number) => new Date(Date.now() - hours * hour).toISOString();

/** The marker: the person last looked two days ago. */
export const LAST_LOOKED = ago(48);

/** The rule "Write studio, not business." as the digest holds it, for the widen stories. */
export function studioRule(): DigestItem {
  return STUDIO;
}

const STUDIO = digestItem({
  id: "0njy7pghcn00000000000000",
  status: "established",
  subject: { kind: "term", term: { term: "business", replacement: "studio" } },
  quote: { path: "docs/billing.md", quote: "Upgrade your business plan." },
  scope: "project product=studio",
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
  droppable: true,
  widenable: true,
  widen_to: ["workspace", "product"],
});
const WORKSPACE = digestItem({
  id: "0njy6aaaaa00000000000000",
  status: "established",
  new: false,
  subject: { kind: "term", term: { term: "project folder", replacement: "workspace" } },
  established_at: ago(120),
  how: ["kept by you"],
  keepable: false,
  droppable: true,
  widenable: true,
});

const RECORDING = digestItem({
  id: "0njy5recor00000000000000",
  status: "established",
  new: false,
  subject: { kind: "term", term: { term: "take", replacement: "recording" } },
  established_at: ago(400),
  how: ["kept by you"],
  keepable: false,
  droppable: true,
  widenable: true,
  widen_to: ["workspace"],
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
  conflicts: [LOGIN_CONFLICT],
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

// The KapiMart sample as a person first opens it (backend/sample/session.go):
// claude-code noticed two names, a word and how the store addresses the
// shopper, and recorded a correction a person made; codex took the other side
// of that correction and noticed one more name.
const CLAUDE_CODE = { kind: "agent", name: "claude-code", session: "kapimart-sample-claude-code" };
const CODEX = { kind: "agent", name: "codex", session: "kapimart-sample-codex" };
const STORE = "project brand=kapimart,channel=store,product=kapimart";
const WEB = "project brand=kapimart,channel=web,mode=how-to,product=kapimart";

const SAMPLE_LOG_IN = digestItem({
  id: "0q6dm6f0bzs50vgr4b1nh7as",
  status: "contested",
  sentence: "Write log in, not sign in.",
  subject: {
    kind: "term",
    term: { term: "sign in", replacement: "log in" },
    text: "The other storefronts say log in.",
  },
  quote: {
    path: "src/en/error-messages.properties",
    quote: "Please verify your email address before signing in.",
  },
  collection: "Online Store",
  standing: "1 against",
  noticed_by: CODEX,
  scope: "project brand=kapimart,channel=reference,product=kapimart",
});
const SAMPLE_SIGN_IN = digestItem({
  id: "0q6dm6eysj5jzp4grgg9bp05",
  status: "contested",
  sentence: "Write sign in, not log in.",
  subject: {
    kind: "term",
    term: {
      term: "log in",
      replacement: "sign in",
      note: "A person changed this in the error messages.",
    } as DigestTermRule,
  },
  quote: {
    path: "src/en/error-messages.properties",
    quote: "Your session has expired. Please sign in again to continue.",
  },
  collection: "Online Store",
  noticed_by: CLAUDE_CODE,
  scope: "project brand=kapimart,channel=reference,product=kapimart",
});
const SAMPLE_PAYPAL = digestItem({
  id: "0q6dm6f12j6qzqfsyse7f0f2",
  theme: "names",
  sentence: "Write PayPal, not Paypal, Pay Pal or Pay-Pal.",
  subject: {
    kind: "term",
    term: { term: "Paypal", replacement: "PayPal", forms: ["Pay Pal", "Pay-Pal"] },
    text: "The payment provider's name keeps its capital P.",
  },
  quote: {
    path: "src/en/checkout-flow.json",
    quote: "You will be redirected to PayPal to complete your purchase.",
  },
  collection: "Online Store",
  noticed_by: CODEX,
  scope: STORE,
});
const SAMPLE_CHECKOUT = digestItem({
  id: "0q6dm6ex30vj41tsgrfx4bkc",
  theme: "names",
  sentence: "Write checkout, not check-out or check out.",
  subject: {
    kind: "term",
    term: { term: "check-out", replacement: "checkout", forms: ["check out"] },
    text: "Checkout is one word, as a noun and as an adjective.",
  },
  quote: { path: "web/en/faq.md", quote: "checkout flow into any website or mobile application." },
  collection: "Website",
  noticed_by: CLAUDE_CODE,
  scope: WEB,
});
const SAMPLE_KAPIMART = digestItem({
  id: "0q6dm6ev9gn875qz5ep678hw",
  theme: "names",
  sentence: "Write KapiMart, not Kapi Mart or Kapi-Mart.",
  subject: {
    kind: "term",
    term: { term: "Kapi Mart", replacement: "KapiMart", forms: ["Kapi-Mart"] },
    text: "The product name is written as one word.",
  },
  quote: {
    path: "web/en/getting-started.md",
    quote: "Welcome to the KapiMart Developer Platform!",
  },
  collection: "Website",
  noticed_by: CLAUDE_CODE,
  scope: WEB,
});
const SAMPLE_CART = digestItem({
  id: "0q6dm6eww7s77c54ge15c58w",
  theme: "words",
  sentence: "Write cart, not basket.",
  subject: {
    kind: "term",
    term: { term: "basket", replacement: "cart" },
    text: "The store calls it a cart.",
  },
  quote: { path: "src/en/store-ui.json", quote: "Your cart is empty" },
  collection: "Online Store",
  noticed_by: CLAUDE_CODE,
  scope: STORE,
});
const SAMPLE_SHOPPER = digestItem({
  id: "0q6dm6ezjy7x0v3463469pv4",
  theme: "writing",
  sentence: "The store copy addresses the shopper as you.",
  subject: { kind: "note", text: "The store copy addresses the shopper as you." },
  quote: { path: "src/en/store-ui.json", quote: "Thank you! Your order has been confirmed." },
  collection: "Online Store",
  noticed_by: CLAUDE_CODE,
  scope: STORE,
  keepable: false,
});

/** The ids of the sample's items, for a story or a test that selects one. */
export const SAMPLE_IDS = {
  logIn: SAMPLE_LOG_IN.id,
  signIn: SAMPLE_SIGN_IN.id,
  payPal: SAMPLE_PAYPAL.id,
  checkout: SAMPLE_CHECKOUT.id,
  kapiMart: SAMPLE_KAPIMART.id,
  cart: SAMPLE_CART.id,
  shopper: SAMPLE_SHOPPER.id,
} as const;

/** The KapiMart sample on first open: one disagreement and five suggestions. */
export const SAMPLE_DIGEST: ContextDigest = {
  project: "KapiMart",
  project_name: "KapiMart",
  conflicts: [
    {
      sides: [SAMPLE_LOG_IN, SAMPLE_SIGN_IN],
      by_evidence: false,
      reason:
        "These rules say different things about the same word. Choose one, and the others are set aside.",
    },
  ],
  established: [],
  suggested: [
    {
      theme: "names",
      title: "Names and spellings",
      groups: [
        { collection: "Online Store", items: [SAMPLE_PAYPAL] },
        { collection: "Website", items: [SAMPLE_CHECKOUT, SAMPLE_KAPIMART] },
      ],
    },
    { theme: "words", title: "Words to avoid", groups: [{ items: [SAMPLE_CART] }] },
    { theme: "writing", title: "How the project writes", groups: [{ items: [SAMPLE_SHOPPER] }] },
  ],
  drift: [],
  numbers: { rules: 0, new_this_week: 0, suggested: 5, conflicts: 1, new: 7 },
};

const MANY_THEMES: Array<{ theme: string; title: string }> = [
  { theme: "names", title: "Names and spellings" },
  { theme: "words", title: "Words to avoid" },
  { theme: "writing", title: "How the project writes" },
  { theme: "memory", title: "Wording in other languages" },
];

/** A project with many suggestions across the four themes, for the list's theme headers. */
export function manySuggestions(count = 40): ContextDigest {
  const perTheme = Math.ceil(count / MANY_THEMES.length);
  let made = 0;
  const suggested = MANY_THEMES.map(({ theme, title }, t) => {
    const items: DigestItem[] = [];
    for (let i = 0; i < perTheme && made < count; i++, made++) {
      const n = t * perTheme + i + 1;
      const id = `0many${String(n).padStart(4, "0")}000000000000000`;
      if (theme === "writing") {
        items.push(
          digestItem({
            id,
            theme,
            subject: { kind: "note", text: `Release note ${n} leads with what changed.` },
            sentence: `Release note ${n} leads with what changed.`,
            keepable: false,
            noticed_by: i % 2 ? CODEX : CLAUDE_CODE,
          }),
        );
      } else if (theme === "memory") {
        items.push(
          digestItem({
            id,
            theme,
            subject: {
              kind: "memory",
              memory: {
                source: `checkout step ${n}`,
                target: `kassetrinn ${n}`,
                target_locale: "nb",
              },
            },
            noticed_by: i % 2 ? CODEX : CLAUDE_CODE,
          }),
        );
      } else {
        items.push(
          digestItem({
            id,
            theme,
            subject: { kind: "term", term: { term: `form ${n}`, replacement: `Form${n}` } },
            quote: { path: `docs/page-${n}.md`, quote: `Every form ${n} reads the same.` },
            collection: i % 3 ? "docs" : "app",
            noticed_by: i % 2 ? CODEX : CLAUDE_CODE,
          }),
        );
      }
    }
    return { theme, title, groups: [{ items }] };
  });
  return {
    ...EMPTY_DIGEST,
    since: LAST_LOOKED,
    suggested,
    numbers: { rules: 0, new_this_week: count, suggested: count, conflicts: 0, new: count },
  };
}

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
