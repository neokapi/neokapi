// Shared Storybook fixtures for the Context hub's Profiles section. Lives under
// src/stories, beside brandHubFixtures, so the story data stays out of the
// shipped package.
import type { Decorator } from "@storybook/react";
import type { ApiAdapter } from "../api/adapter";
import type { ContextProfileVoice, ContextProfilesResponse } from "../types/context-profiles";
import type { ChannelAliasProposalsResponse } from "../types/channel-proposals";
import { createProvidersDecorator } from "./decorators";

const voice = (id: string, name: string, version: number): ContextProfileVoice => ({
  id,
  name,
  description: "How this point sounds, and what it may not say.",
  version,
  updated_at: "2026-06-13T10:00:00Z",
  preferred_terms: 14,
  forbidden_terms: 6,
  competitor_terms: 3,
  pattern_rules: 4,
  locale_overrides: 2,
  channel_overrides: 3,
});

/** A workspace with several declared points, one ungoverned, one voice unbound. */
export const populatedProfiles: ContextProfilesResponse = {
  axes: ["channel", "product"],
  terms: { concept_count: 248 },
  scan: {
    job_id: "scan-1",
    status: "completed",
    progress: 100,
    updated_at: "2026-06-12T08:30:00Z",
  },
  scan_scope: "workspace",
  profiles: [
    {
      slug: "default",
      name: "",
      label: "Brand",
      is_default: true,
      declared: true,
      collections: [
        {
          id: "col-misc",
          name: "uploads",
          project_id: "p-web",
          project_name: "Marketing Website",
          stream: "main",
          item_label: "item",
          owner: "workspace",
        },
      ],
      voice: voice("v-core", "Core voice", 7),
      pending_changes: 0,
    },
    {
      slug: "channel~docs.product~bowrain",
      name: "docs-bowrain",
      label: "docs-bowrain",
      is_default: false,
      declared: true,
      coordinates: { product: "bowrain", channel: "docs" },
      channel: "docs",
      collections: [
        {
          id: "col-docs",
          name: "bowrain-docs",
          project_id: "p-docs",
          project_name: "Docs Portal",
          stream: "main",
          item_label: "page",
          owner: "recipe",
        },
        {
          id: "col-guides",
          name: "bowrain-guides",
          project_id: "p-docs",
          project_name: "Docs Portal",
          stream: "main",
          item_label: "guide",
          owner: "recipe",
        },
      ],
      voice: voice("v-docs", "Bowrain docs voice", 3),
      pending_changes: 2,
      checks: {
        score: 87,
        scored_blocks: 412,
        findings: 23,
        last_checked_at: "2026-06-13T09:12:00Z",
      },
    },
    {
      slug: "channel~app.product~bowrain",
      name: "app-bowrain",
      label: "app-bowrain",
      is_default: false,
      declared: true,
      coordinates: { product: "bowrain", channel: "app" },
      channel: "app",
      collections: [
        {
          id: "col-app",
          name: "bowrain-app",
          project_id: "p-app",
          project_name: "Bowrain App",
          stream: "main",
          item_label: "string",
          owner: "recipe",
        },
      ],
      pending_changes: 0,
    },
    {
      slug: "voice~v-support",
      name: "",
      label: "Support voice",
      is_default: false,
      declared: false,
      collections: [],
      voice: voice("v-support", "Support voice", 1),
      pending_changes: 0,
    },
  ],
};

/** A workspace that has pushed nothing: the default point and nothing else. */
export const emptyProfiles: ContextProfilesResponse = {
  axes: [],
  terms: { concept_count: 0 },
  scan_scope: "workspace",
  profiles: [
    {
      slug: "default",
      name: "",
      label: "Brand",
      is_default: true,
      declared: false,
      collections: [],
      pending_changes: 0,
    },
  ],
};

/** A workspace whose two projects spell one channel two ways. */
export const fragmentedChannels: ChannelAliasProposalsResponse = {
  proposals: [
    {
      profile: "bowrain",
      proposed_channel: "help",
      existing_channel: "help-centre",
      evidence: "one slug is a prefix of the other within the same product",
      project_id: "p-support",
      collection: "support-articles",
      status: "proposed",
      created_at: "2026-06-13T07:00:00Z",
      updated_at: "2026-06-13T07:00:00Z",
    },
    {
      profile: "bowrain",
      proposed_channel: "marketing-site",
      existing_channel: "marketingsite",
      evidence: "the slugs differ only in case or separators",
      project_id: "p-web",
      collection: "landing",
      status: "dismissed",
      judged_by: "u-ada",
      judged_at: "2026-06-12T16:20:00Z",
      created_at: "2026-06-10T07:00:00Z",
      updated_at: "2026-06-13T07:00:00Z",
    },
  ],
};

/** Serves one aggregation to every profile hook the story renders. */
export const withProfiles = (
  response: ContextProfilesResponse,
  proposals: ChannelAliasProposalsResponse = { proposals: [] },
): Decorator => {
  const overrides: Partial<ApiAdapter> = {
    listContextProfiles: async () => response,
    listChannelProposals: async () => proposals,
  };
  return createProvidersDecorator(undefined, overrides);
};
