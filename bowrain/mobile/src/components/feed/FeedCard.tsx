// One card of the project feed: the actor's avatar, their name in bold with
// what they did, the time, and a preview of the change. A run of one actor's
// changes of one kind folds into one card that expands to list them. A
// coverage milestone is a card of its own. The card leaves room for a footer
// action row below the preview.
import type { FeedCard as Card, FeedEntry } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { memo, useState } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { actionOf, actorName, ageLabel, foldedActionOf, languageLabel } from "../../lib/feed";
import { Avatar } from "./Avatar";
import { Preview } from "./Preview";

export interface FeedCardProps {
  card: Card;
  onOpen: (entry: FeedEntry) => void;
  onOpenLanguage: (locale: string) => void;
}

const CARD = "bg-card border-border mx-4 mb-3 rounded-3xl border p-4 shadow-sm";

function MilestoneCard({
  entry,
  onOpenLanguage,
}: {
  entry: FeedEntry;
  onOpenLanguage: (l: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const percent = entry.counts.percent ?? 0;
  const locale = entry.locales?.[0];
  const language = locale ? languageLabel(locale, i18n.language) : "";
  return (
    <Pressable
      testID={`feed-card-${entry.id}`}
      accessibilityRole="button"
      accessibilityLabel={actionOf(entry, t, i18n.language)}
      onPress={() => locale && onOpenLanguage(locale)}
      className={`${CARD} overflow-hidden p-0 active:opacity-90`}
    >
      <View className="from-prism-1 via-prism-3 to-prism-5 h-1.5 bg-linear-to-r" />
      <View className="items-center gap-1 px-5 pt-5 pb-5">
        <Text className="text-muted-foreground text-[12px] font-semibold tracking-wide uppercase">
          {t("feed.milestoneLabel")}
        </Text>
        <Text className="text-foreground text-[56px] leading-[64px] font-bold tabular-nums">
          {`${percent}%`}
        </Text>
        <Text className="text-foreground text-[17px] font-semibold">{language}</Text>
        <View className="bg-muted mt-3 h-2 w-full overflow-hidden rounded-full">
          <View
            className="from-prism-1 via-prism-3 to-prism-5 h-full rounded-full bg-linear-to-r"
            style={{ width: `${Math.min(100, percent)}%` }}
          />
        </View>
        <Text className="text-muted-foreground mt-2 text-[13px]">
          {ageLabel(entry.at, t, i18n.language)}
        </Text>
      </View>
    </Pressable>
  );
}

function FoldedList({ entries }: { entries: FeedEntry[] }) {
  const { t, i18n } = useTranslation();
  return (
    <View className="border-border mt-3 gap-2 border-t pt-3">
      {entries.map((e) => (
        <View key={e.id} className="flex-row items-baseline gap-2">
          <View className="bg-muted-foreground h-1.5 w-1.5 translate-y-[-2px] rounded-full" />
          <Text className="text-foreground flex-1 text-[14px]" numberOfLines={2}>
            {actionOf(e, t, i18n.language)}
          </Text>
          <Text className="text-muted-foreground text-[13px]">
            {ageLabel(e.at, t, i18n.language)}
          </Text>
        </View>
      ))}
    </View>
  );
}

function FeedCardView({ card, onOpen, onOpenLanguage }: FeedCardProps) {
  const { t, i18n } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const primary = useCSSVariable("--color-primary");
  const lead = card.entries[0];
  if (lead.kind === "coverage_milestone") {
    return <MilestoneCard entry={lead} onOpenLanguage={onOpenLanguage} />;
  }
  const name = actorName(lead, t);
  const action = foldedActionOf(card, t, i18n.language);
  const folded = card.entries.length > 1;
  const meta = [
    lead.actor.kind === "agent" && lead.on_behalf_of?.name
      ? t("feed.forMember", { name: lead.on_behalf_of.name })
      : undefined,
    ageLabel(lead.at, t, i18n.language),
  ]
    .filter(Boolean)
    .join(" · ");

  // The header is the card's one accessible button. The preview opens the
  // same place when tapped, but stays a group, so a language chip inside it
  // is its own control rather than folded into the card's label; the expand
  // control sits outside both for the same reason.
  return (
    <View testID={`feed-card-${lead.id}`} className={CARD}>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={`${name} ${action}. ${meta}`}
        onPress={() => onOpen(lead)}
        className="flex-row gap-3 active:opacity-70"
      >
        <Avatar actor={lead.actor} name={name} />
        <View className="min-w-0 flex-1 justify-center">
          <Text className="text-foreground text-[15px] leading-5">
            <Text className="font-semibold">{name}</Text> {action}
          </Text>
          <Text className="text-muted-foreground mt-0.5 text-[13px]">{meta}</Text>
        </View>
      </Pressable>

      <Pressable accessible={false} onPress={() => onOpen(lead)} className="mt-3">
        <Preview entry={lead} onOpenLanguage={onOpenLanguage} />
      </Pressable>

      {folded ? (
        <>
          {expanded ? <FoldedList entries={card.entries.slice(1)} /> : null}
          <Pressable
            testID={`feed-expand-${lead.id}`}
            accessibilityRole="button"
            hitSlop={8}
            onPress={() => setExpanded((v) => !v)}
            className="mt-3 flex-row items-center gap-1 self-start active:opacity-60"
          >
            <Text className="text-primary text-[14px] font-medium">
              {expanded
                ? t("feed.showLess")
                : t("feed.showAll", { count: card.entries.length - 1 })}
            </Text>
            <Ionicons
              name={expanded ? "chevron-up" : "chevron-down"}
              size={14}
              color={String(primary)}
            />
          </Pressable>
        </>
      ) : null}
    </View>
  );
}

export const FeedCardItem = memo(FeedCardView);
