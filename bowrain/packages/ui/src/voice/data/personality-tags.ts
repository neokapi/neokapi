/**
 * Suggested personality tags for the tag picker.
 *
 * Grouped by broad category so the UI can show them in logical clusters.
 * Users can always type custom tags — these are just suggestions to spark ideas.
 *
 * This file is intentionally data-only so it can be maintained by AI tooling.
 */

export interface TagCategory {
  label: string;
  tags: string[];
}

export const personalityTagCategories: TagCategory[] = [
  {
    label: "Professional",
    tags: [
      "professional",
      "authoritative",
      "trustworthy",
      "knowledgeable",
      "confident",
      "polished",
    ],
  },
  {
    label: "Friendly",
    tags: ["friendly", "approachable", "warm", "cheerful", "optimistic", "welcoming"],
  },
  {
    label: "Creative",
    tags: ["creative", "bold", "witty", "playful", "imaginative", "expressive"],
  },
  {
    label: "Supportive",
    tags: ["empathetic", "helpful", "reassuring", "patient", "caring", "encouraging"],
  },
  {
    label: "Technical",
    tags: ["precise", "clear", "concise", "instructive", "analytical", "direct"],
  },
];

/** Flat list of all suggested tags for simple lookups. */
export const allSuggestedTags: string[] = personalityTagCategories.flatMap((c) => c.tags);
