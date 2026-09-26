/**
 * Default value factories for voice profile fields.
 *
 * Shared between the wizard (new profiles) and any component that needs
 * a blank starting state.
 */

import type { ToneProfile, StyleRules } from "./types";

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
