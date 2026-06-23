// Shared content + vocabulary for the animated hero prototypes. One Acme Series-B
// financials slide, the terms it protects, the figures it redacts, the languages
// it fans into, and the AI models it calls — so all three concepts tell the same
// story with the same data.

export interface Bullet {
  id: string;
  /** Static lead-in (e.g. "FY27 revenue:"). */
  label?: string;
  /** The sensitive value, masked when its param is active. */
  value?: string;
  /** Which redaction param masks `value`. */
  redactKey?: string;
  /** A plain bullet with no redaction (the editable claim). */
  text?: string;
}

export const SLIDE = {
  file: "deck.pptx",
  kicker: "Financials",
  title: "Acme — Series B",
  bullets: [
    { id: "b1", label: "FY27 revenue:", value: "$48M", redactKey: "revenue" },
    { id: "b2", label: "Net burn:", value: "$1.2M/mo", redactKey: "burn" },
    { id: "b3", text: "Category-leading growth." },
  ] as Bullet[],
  footerLeft: "Acme, Inc.",
  footerRight: "Confidential",
};

// Protected terms — fed from the termbase and held verbatim through translation.
export interface Term {
  term: string;
  domain: string;
}
export const TERMS: Term[] = [
  { term: "Acme", domain: "brand" },
  { term: "Series B", domain: "finance" },
];

// Redaction parameters — what the policy masks before anything reaches a model.
export const REDACT_PARAMS: { key: string; label: string }[] = [
  { key: "revenue", label: "revenue" },
  { key: "burn", label: "burn rate" },
];

// AI models called, one fan of languages each.
export const MODELS = ["Claude", "GPT-4o", "Gemini"];

// The languages the deck fans into. `reused` = served from translation memory
// instead of a fresh model call.
export interface Lang {
  code: string;
  label: string;
  title: string;
  reused?: boolean;
  rtl?: boolean;
}
export const LANGS: Lang[] = [
  { code: "ja", label: "日本語", title: "Acme — シリーズ B" },
  { code: "fr", label: "Français", title: "Acme — Série B" },
  { code: "de", label: "Deutsch", title: "Acme — Serie B", reused: true },
  { code: "es", label: "Español", title: "Acme — Serie B" },
  { code: "zh", label: "中文", title: "Acme — B 轮融资" },
  { code: "ko", label: "한국어", title: "Acme — 시리즈 B" },
  { code: "pt", label: "Português", title: "Acme — Série B", reused: true },
  { code: "ar", label: "العربية", title: "Acme — الجولة B", rtl: true },
];

// The translation-memory hit — the editable claim, recovered 1:1 from a prior run.
export const TM_HIT = { lineId: "b3", match: "100%" };
