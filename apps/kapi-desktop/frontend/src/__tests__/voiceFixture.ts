// A project voice answer shaped the way the backend serves one: three points,
// a declared brand axis, a binding whose window closed, and a point nothing
// binds at. Shared by the page tests and the Storybook story so both exercise
// the same shape.

import type { FieldValueSet, ProjectVoiceResult } from "../types/voice";

/**
 * The value sets the backend serves from what validation applies.
 *
 * Tone is open — a register outside the list is kept and rendered — while the
 * style enums are closed, because the offline check reads them.
 */
export const valueSetsFixture: Record<string, FieldValueSet> = {
  "tone.formality": { values: ["casual", "neutral", "formal", "technical"], open: true },
  "tone.emotion": { values: ["warm", "neutral", "authoritative"], open: true },
  "tone.humor": { values: ["none", "light", "frequent"], open: true },
  "style.sentence_length": { values: ["short", "medium", "varied"], open: false },
  "style.person_pov": { values: ["first_plural", "second", "third"], open: false },
  "style.contractions": { values: ["always", "sometimes", "never"], open: false },
  "examples.category": { values: ["tone", "style", "vocabulary"], open: false },
  severity: { values: ["neutral", "minor", "major", "critical"], open: false },
  scope: { values: ["prose", "code", "heading"], open: false },
};

export const voiceFixture: ProjectVoiceResult = {
  at: "2026-08-30T09:00:00Z",
  points: [
    {
      label: "project default",
      point: { default: true, ref: "defaults.voice" },
      coordinates: { brand: "northsea" },
      collections: ["App", "Promo"],
      field: "defaults.voice",
      source: "/w/northsea/.kapi/voice.yaml",
      binding: { kind: "profile_file", value: ".kapi/voice.yaml" },
      termstore: ".kapi/terms.json",
      guide: "Write as Northsea: say the useful thing first.",
      edit: { target: ".kapi/voice.yaml", writable: true, exists: true, inherited: false },
      profile: {
        name: "Northsea",
        description: "How Northsea writes to everyone.",
        min_score: 80,
        tone: {
          personality: ["clear", "calm"],
          formality: "neutral",
          emotion: "measured",
          humor: "none",
          guidelines: "Say the useful thing first.",
        },
        style: {
          active_voice: true,
          sentence_length: "medium",
          person_pov: "second",
          contractions: "sometimes",
          prohibited_patterns: [
            {
              regex: "\\bsynergy\\b",
              description: "Corporate filler.",
              severity: "minor",
              rate: { max: 2, per_words: 1000 },
            },
          ],
          required_patterns: [
            {
              regex: "\\bplease\\b",
              description: "Ask, do not instruct.",
              severity: "neutral",
              scope: "prose",
            },
          ],
        },
        vocabulary: {
          preferred_terms: [
            {
              term: "log in",
              replacement: "sign in",
              severity: "major",
              note: "One spelling across the product.",
              concept_id: "c-signin",
            },
          ],
          forbidden_terms: [{ term: "bulletproof", severity: "critical" }],
          abbreviations: { API: "application programming interface" },
        },
        examples: [
          {
            before: "Utilize the portal.",
            after: "Use the portal.",
            explanation: "Plain words carry further.",
            category: "vocabulary",
          },
        ],
        locales: {
          "nb-NO": {
            formality: "informal",
            cultural_notes: "Norwegian readers expect direct address.",
          },
        },
        channels: { docs: { tone: { formality: "formal" } } },
        personas: { "support-agent": { tone: { emotion: "warm" } } },
      },
    },
    {
      label: "campaign",
      point: { profile: "campaign", default: false, ref: "defaults.voice" },
      coordinates: { brand: "northsea", product: "campaign" },
      channels: ["promo"],
      collections: [],
      field: "defaults.voice",
      source: "/w/northsea/.kapi/voice.yaml",
      binding: { kind: "profile_file", value: ".kapi/voice.yaml" },
      validity: { to: "2026-08-29T00:00:00Z", state: "expired" },
      fallback: {
        profile: "campaign",
        expired: true,
        boundary: "2026-08-29T00:00:00Z",
        governing: "",
        message: 'profile "campaign" expired 2026-08-29; governing with the project default',
      },
      guide: "Write as Northsea: say the useful thing first.",
      edit: {
        target: ".kapi/profiles/campaign/voice.yaml",
        writable: true,
        exists: false,
        inherited: true,
      },
      profile: { name: "Northsea", tone: { personality: ["clear"] } },
    },
    {
      label: "support",
      point: { profile: "support", default: false, ref: "profiles.support.voice" },
      coordinates: { brand: "northsea", product: "support" },
      channels: ["docs"],
      collections: ["Docs"],
      field: "profiles.support.voice",
      edit: {
        target: ".kapi/profiles/support/voice.yaml",
        writable: true,
        exists: false,
        inherited: false,
      },
      notes: ["no voice profile binds at this point"],
    },
  ],
};
