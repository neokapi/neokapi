// Who worked on a project lately, as the feed records it: the avatars of the
// last few people and agents, and the newest entry as a sentence. It opens the
// Activity tab, which shows the project's feed whole.
import type { FeedEntry } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { Avatar } from "../feed/Avatar";
import { actionOf, actorName, ageLabel } from "../../lib/feed";

const MAX_AVATARS = 4;

/** The newest entry of each actor, newest first, at most `max`. */
export function recentActors(entries: FeedEntry[], max = MAX_AVATARS): FeedEntry[] {
  const seen = new Set<string>();
  const out: FeedEntry[] = [];
  for (const e of entries) {
    const id = `${e.actor.kind}:${e.actor.id || e.actor.name || ""}`;
    if (seen.has(id)) continue;
    seen.add(id);
    out.push(e);
    if (out.length === max) break;
  }
  return out;
}

export function ActivityStrip({ entries, onPress }: { entries: FeedEntry[]; onPress: () => void }) {
  const { t, i18n } = useTranslation();
  const muted = useCSSVariable("--color-muted-foreground");
  const latest = entries[0];
  if (!latest) return null;
  const actors = recentActors(entries);
  const sentence = `${actorName(latest, t)} ${actionOf(latest, t, i18n.language)}`;
  const age = ageLabel(latest.at, t, i18n.language);
  return (
    <Pressable
      testID="project-activity"
      accessibilityRole="button"
      accessibilityLabel={`${t("home.activity.title")}: ${sentence}, ${age}`}
      onPress={onPress}
      className="bg-card border-border mx-4 mb-3 gap-3 rounded-3xl border p-4 active:opacity-80"
    >
      <View className="flex-row items-center justify-between">
        <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
          {t("home.activity.title")}
        </Text>
        <Ionicons name="chevron-forward" size={16} color={String(muted)} />
      </View>
      <View className="flex-row items-center gap-3">
        <View className="flex-row">
          {actors.map((e, i) => (
            <View
              key={e.id}
              style={{ marginLeft: i === 0 ? 0 : -10, zIndex: actors.length - i }}
              className="border-card rounded-full border-2"
            >
              <Avatar actor={e.actor} name={actorName(e, t)} size={32} />
            </View>
          ))}
        </View>
        <View className="flex-1 gap-0.5">
          <Text className="text-foreground text-sm" numberOfLines={2}>
            <Text className="font-semibold">{actorName(latest, t)}</Text>{" "}
            {actionOf(latest, t, i18n.language)}
          </Text>
          <Text className="text-muted-foreground text-xs">{age}</Text>
        </View>
      </View>
    </Pressable>
  );
}
