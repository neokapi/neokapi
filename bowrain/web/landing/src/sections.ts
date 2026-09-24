import type { SectionSpec } from "./sectionSignals";

// Stable section IDs and analytics positions. Position follows the intended
// reading order, independently of DOM placement. Keep these as module constants:
// useSectionSignals uses each object as an effect dependency.
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
