/**
 * Default value factories for brand voice profile fields.
 *
 * Shared between the wizard (new profiles) and any component that needs
 * a blank starting state.
 */

import type { ToneProfile, StyleRules, VocabularyRules } from "./types";

export function defaultTone(): ToneProfile {
  return {
    personality: [],
    formality: "neutral",
    emotion: "neutral",
    humor: "none",
  };
}

export function defaultStyle(): StyleRules {
  return {
    active_voice: true,
    sentence_length: "medium",
    person_pov: "second",
    contractions: "sometimes",
    prohibited_patterns: [],
    required_patterns: [],
  };
}

export function defaultVocabulary(): VocabularyRules {
  return { preferred_terms: [], forbidden_terms: [], competitor_terms: [] };
}
