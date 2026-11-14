// One feed entry as a row: the kind's icon, a primary line and a meta line of
// at most three facts (who, the main count, the time). The actor's mark sits
// in the meta line: a person, an agent or a tool.
import type { FeedActorKind, FeedEntry } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import type { ComponentProps } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { entryMeta, entryTitle } from "../lib/feed";

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

export function FeedRow({ entry, onPress }: { entry: FeedEntry; onPress: () => void }) {
  const { t, i18n } = useTranslation();
  const [foreground, muted] = useCSSVariable(["--color-foreground", "--color-muted-foreground"]);
  const title = entryTitle(entry, t, i18n.language);
  const meta = entryMeta(entry, t, i18n.language);
  return (
    <Pressable
      testID={`feed-entry-${entry.id}`}
      accessibilityRole="button"
      accessibilityLabel={`${title}. ${meta}`}
      onPress={onPress}
      className="bg-card border-border flex-row items-center gap-3 border-b px-4 py-3 active:opacity-70"
    >
      <Ionicons name={kindIcon(entry.kind)} size={22} color={String(foreground)} />
      <View className="min-w-0 flex-1 gap-0.5">
        <Text className="text-foreground text-base" numberOfLines={1}>
          {title}
        </Text>
        <View className="flex-row items-center gap-1">
          <Ionicons name={actorIcon(entry.actor.kind)} size={14} color={String(muted)} />
          <Text className="text-muted-foreground flex-1 text-sm tabular-nums" numberOfLines={1}>
            {meta}
          </Text>
        </View>
      </View>
      <Ionicons name="chevron-forward" size={16} color={String(muted)} />
    </Pressable>
  );
}
