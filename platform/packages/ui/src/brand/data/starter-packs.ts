/**
 * Starter pack metadata for the template picker UI.
 *
 * Each entry mirrors a server-side YAML pack in core/brand/packs/ but adds
 * display-specific fields (icon, accent color, sample text). The `name` field
 * must match the YAML filename (without extension) so the API can resolve it.
 *
 * This file is intentionally data-only (no JSX) so it can be maintained by AI
 * tooling or non-frontend contributors.
 */

import type { ToneProfile } from "../types";

export interface StarterPackMeta {
  /** Server pack key, e.g. "professional-b2b" — must match YAML filename */
  name: string;
  /** Human-readable display name */
  label: string;
  /** Short description (from YAML) */
  description: string;
  /** Lucide icon name used in the picker card */
  icon: "Briefcase" | "Heart" | "Pencil" | "Headset" | "FileCode";
  /** Accent color for the card top border (works in both light/dark) */
  accentColor: string;
  /** One-line hook shown on the card */
  tagline: string;
  /** Example sentence showing the voice in action */
  sampleText: string;
  /** Representative personality tags */
  personalityTags: string[];
  /** Formality level for visual badge */
  formality: ToneProfile["formality"];
}

export const starterPacks: StarterPackMeta[] = [
  {
    name: "professional-b2b",
    label: "Professional B2B",
    description: "Formal, authoritative voice for business-to-business communication",
    icon: "Briefcase",
    accentColor: "oklch(0.65 0.15 250)",
    tagline: "Formal, authoritative, data-driven",
    sampleText:
      "We are pleased to introduce a new capability that addresses a key challenge in enterprise workflows.",
    personalityTags: ["professional", "knowledgeable", "trustworthy"],
    formality: "formal",
  },
  {
    name: "friendly-dtc",
    label: "Friendly DTC",
    description: "Casual, warm voice for direct-to-consumer brands",
    icon: "Heart",
    accentColor: "oklch(0.70 0.18 25)",
    tagline: "Casual, warm, authentic",
    sampleText: "Great news \u2014 your order's on its way! You'll love what's inside.",
    personalityTags: ["friendly", "approachable", "authentic"],
    formality: "casual",
  },
  {
    name: "marketing-blog",
    label: "Marketing Blog",
    description: "Conversational, storytelling voice for blog posts and content marketing",
    icon: "Pencil",
    accentColor: "oklch(0.68 0.16 145)",
    tagline: "Engaging, conversational, insightful",
    sampleText:
      "Ever shipped a bug that a five-minute test would have caught? Let's talk about how to avoid that.",
    personalityTags: ["engaging", "conversational", "insightful"],
    formality: "neutral",
  },
  {
    name: "customer-support",
    label: "Customer Support",
    description: "Empathetic, solution-focused voice for customer support communication",
    icon: "Headset",
    accentColor: "oklch(0.68 0.14 300)",
    tagline: "Empathetic, helpful, reassuring",
    sampleText:
      "I understand your frustration. Here's what we can do to get this resolved for you right away.",
    personalityTags: ["empathetic", "helpful", "reassuring"],
    formality: "neutral",
  },
  {
    name: "technical-docs",
    label: "Technical Docs",
    description: "Precise, clear voice for technical documentation and developer guides",
    icon: "FileCode",
    accentColor: "oklch(0.65 0.12 70)",
    tagline: "Precise, clear, instructive",
    sampleText:
      "Send a GET request to the /data endpoint. The response includes a JSON array of records.",
    personalityTags: ["precise", "clear", "instructive"],
    formality: "technical",
  },
];
