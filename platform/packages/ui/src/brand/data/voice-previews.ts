/**
 * Sample text snippets for the live brand voice preview panel.
 *
 * Keyed by formality + emotion combination. The preview component selects
 * the best match and displays it so the user can see what their configured
 * voice "sounds like" in practice.
 *
 * This file is intentionally data-only so it can be maintained by AI tooling.
 */

import type { ToneProfile } from "../types";

type PreviewKey = `${ToneProfile["formality"]}-${ToneProfile["emotion"]}`;

export const voicePreviewTexts: Record<PreviewKey, string> = {
  // Casual
  "casual-warm":
    "Hey there! We just rolled out something pretty exciting \u2014 a brand new way to manage your projects. It's super easy to get started, and we think you're going to love it. Give it a try and let us know what you think!",
  "casual-neutral":
    "We've added a new project management feature. It simplifies how you organize your work and should save you time on day-to-day tasks. Check it out when you get a chance.",
  "casual-authoritative":
    "We've built something that changes how you manage projects. No more juggling spreadsheets and emails. This is the tool you've been waiting for \u2014 and it works.",

  // Neutral
  "neutral-warm":
    "We're excited to share a new project management feature designed to make your workflow smoother. We've listened to your feedback and built something we think you'll really enjoy using every day.",
  "neutral-neutral":
    "We've released a project management feature that streamlines how you organize and track your work. It integrates with your existing tools and is available to all team members.",
  "neutral-authoritative":
    "Today we introduce a project management capability built on years of workflow research. It addresses the core challenges teams face when coordinating complex work across distributed environments.",

  // Formal
  "formal-warm":
    "We are delighted to announce the release of our new project management capability. Designed with your team's needs in mind, this feature represents our commitment to making your work life more productive and enjoyable.",
  "formal-neutral":
    "We are pleased to announce the availability of a new project management capability. This feature provides comprehensive tools for planning, tracking, and delivering projects within your organization.",
  "formal-authoritative":
    "We are introducing a project management platform that establishes a new standard for enterprise workflow coordination. This capability addresses critical gaps in how organizations plan, execute, and measure project outcomes.",

  // Technical
  "technical-warm":
    "The new project management API is live. We've designed the endpoints to feel intuitive \u2014 if you've used our other APIs, this will feel like home. Full OpenAPI spec is in the docs, and our team is here if you hit any snags.",
  "technical-neutral":
    "The project management API exposes CRUD operations for projects, tasks, and milestones. Authentication uses the standard Bearer token flow. Endpoints follow REST conventions with JSON request and response bodies.",
  "technical-authoritative":
    "The project management API implements the resource lifecycle model defined in RFC 7231. All endpoints enforce strict schema validation. Rate limiting applies at 1000 requests per minute per API key.",
};

/**
 * Get the best matching preview text for a given tone profile.
 * Falls back through emotion → neutral, then formality → neutral.
 */
export function getVoicePreview(
  formality: ToneProfile["formality"],
  emotion: ToneProfile["emotion"],
): string {
  const key: PreviewKey = `${formality}-${emotion}`;
  if (voicePreviewTexts[key]) return voicePreviewTexts[key];

  const fallback: PreviewKey = `${formality}-neutral`;
  if (voicePreviewTexts[fallback]) return voicePreviewTexts[fallback];

  return voicePreviewTexts["neutral-neutral"];
}
