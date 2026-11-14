// The icons the feed's detail view marks an entry's kind and actor with.
import type { FeedActorKind } from "@neokapi/bowrain-client";
import type Ionicons from "@expo/vector-icons/Ionicons";
import type { ComponentProps } from "react";

type IconName = ComponentProps<typeof Ionicons>["name"];

const KIND_ICONS: Record<string, IconName> = {
  push: "cloud-upload-outline",
  convergence_run: "sync-outline",
  review_summary: "checkmark-done-outline",
  recipe_change: "document-text-outline",
  context_change: "book-outline",
  change_set: "git-pull-request-outline",
  coverage_milestone: "flag-outline",
};

const ACTOR_ICONS: Record<FeedActorKind, IconName> = {
  person: "person-circle-outline",
  agent: "sparkles-outline",
  tool: "cog-outline",
};

export function kindIcon(kind: string): IconName {
  return KIND_ICONS[kind] ?? "ellipse-outline";
}

export function actorIcon(kind: FeedActorKind): IconName {
  return ACTOR_ICONS[kind] ?? ACTOR_ICONS.person;
}
