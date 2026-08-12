import type { SectionSpec } from "./sectionSignals";

// The narrative order, single-sourced.
//
// The page tells one argument monolingual-first: a rename is one decision and
// five different rules (2), so content has coordinates (3), which the work
// itself keeps current (4), enforced by checks you can run (5) — and a language
// is then one more coordinate, not a second product (6). `position` is what the
// section_viewed funnel is ordered by, so it is the narrative index, not the
// DOM index: the tail sections a buyer reads out of order (apps, licensing,
// plans) sit after the six beats and never displace them.
//
// Each object is a stable module constant because useSectionSignals takes it as
// an effect dependency; an inline literal would re-subscribe on every render.
export const SECTION_HERO: SectionSpec = { id: "hero", position: 1 };
export const SECTION_RENAME: SectionSpec = { id: "rename", position: 2 };
export const SECTION_HOW: SectionSpec = { id: "how", position: 3 };
export const SECTION_LOOP: SectionSpec = { id: "loop", position: 4 };
export const SECTION_PROOF: SectionSpec = { id: "proof", position: 5 };
export const SECTION_LANGUAGES: SectionSpec = { id: "languages", position: 6 };

export const SECTION_PRODUCT: SectionSpec = { id: "product", position: 7 };
export const SECTION_APPS: SectionSpec = { id: "apps", position: 8 };
export const SECTION_OPEN_SOURCE: SectionSpec = { id: "open-source", position: 9 };
export const SECTION_PLANS: SectionSpec = { id: "pricing", position: 10 };
export const SECTION_CTA: SectionSpec = { id: "get-started", position: 11 };
