/**
 * Structured options for the visual spectrum selectors.
 *
 * Each spectrum maps a TypeScript union type to display metadata: label,
 * description, and an example sentence showing the option in action.
 *
 * This file is intentionally data-only so it can be maintained by AI tooling.
 */

import type { ToneProfile, StyleRules } from "../types";

export interface SpectrumOption<T extends string> {
  value: T;
  label: string;
  description: string;
  exampleText: string;
}

// ---------------------------------------------------------------------------
// Tone spectrums
// ---------------------------------------------------------------------------

export const formalitySpectrum: SpectrumOption<ToneProfile["formality"]>[] = [
  {
    value: "casual",
    label: "Casual",
    description: "Relaxed, everyday language",
    exampleText: "Hey! Check out our new feature.",
  },
  {
    value: "neutral",
    label: "Neutral",
    description: "Balanced and approachable",
    exampleText: "We've released a new feature you might find useful.",
  },
  {
    value: "formal",
    label: "Formal",
    description: "Professional, polished language",
    exampleText: "We are pleased to announce the release of a new capability.",
  },
  {
    value: "technical",
    label: "Technical",
    description: "Precise, domain-specific terms",
    exampleText: "The endpoint accepts a JSON payload and returns a 201 status.",
  },
];

export const emotionSpectrum: SpectrumOption<ToneProfile["emotion"]>[] = [
  {
    value: "warm",
    label: "Warm",
    description: "Friendly and caring",
    exampleText: "We're here for you every step of the way.",
  },
  {
    value: "neutral",
    label: "Neutral",
    description: "Balanced and objective",
    exampleText: "This guide covers the setup process.",
  },
  {
    value: "authoritative",
    label: "Authoritative",
    description: "Expert and decisive",
    exampleText: "Organizations must adopt this approach to remain competitive.",
  },
];

export const humorSpectrum: SpectrumOption<ToneProfile["humor"]>[] = [
  {
    value: "none",
    label: "None",
    description: "Straightforward, no humor",
    exampleText: "Complete the form to submit your request.",
  },
  {
    value: "light",
    label: "Light",
    description: "Occasional wit and levity",
    exampleText: "Almost there \u2014 just one more step and you're golden!",
  },
  {
    value: "frequent",
    label: "Frequent",
    description: "Humor woven throughout",
    exampleText: "Buckle up, because this feature is about to blow your mind.",
  },
];

// ---------------------------------------------------------------------------
// Style spectrums
// ---------------------------------------------------------------------------

export const sentenceLengthSpectrum: SpectrumOption<StyleRules["sentence_length"]>[] = [
  {
    value: "short",
    label: "Short",
    description: "Punchy, to the point",
    exampleText: "Click Save. The changes apply immediately.",
  },
  {
    value: "medium",
    label: "Medium",
    description: "Clear with moderate detail",
    exampleText: "Click Save to apply your changes, which will take effect immediately.",
  },
  {
    value: "varied",
    label: "Varied",
    description: "Mix of short and long for rhythm",
    exampleText:
      "Click Save. Your changes will take effect immediately, giving you a chance to verify everything looks right before moving on.",
  },
];

export const povSpectrum: SpectrumOption<StyleRules["person_pov"]>[] = [
  {
    value: "first_plural",
    label: "We",
    description: 'First person plural \u2014 "we" voice',
    exampleText: "We've designed this feature with your workflow in mind.",
  },
  {
    value: "second",
    label: "You",
    description: 'Second person \u2014 "you" voice',
    exampleText: "You can configure this feature to match your workflow.",
  },
  {
    value: "third",
    label: "They",
    description: 'Third person \u2014 "the user" voice',
    exampleText: "The user can configure this feature to match their workflow.",
  },
];

export const contractionsSpectrum: SpectrumOption<StyleRules["contractions"]>[] = [
  {
    value: "always",
    label: "Always",
    description: "Use contractions freely",
    exampleText: "You'll find it's easy to get started. Don't worry about setup.",
  },
  {
    value: "sometimes",
    label: "Sometimes",
    description: "Use selectively for flow",
    exampleText: "You'll find it easy to get started. Do not worry about setup.",
  },
  {
    value: "never",
    label: "Never",
    description: "Fully expanded forms only",
    exampleText:
      "You will find it is straightforward to begin. Do not be concerned about the setup process.",
  },
];
