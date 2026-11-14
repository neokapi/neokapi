// One block's translation as a card in the deck: the language and group as
// chips, the source above the translation with codes as chips, where the
// block sits, and what the server says about it.
import type { DeckCard } from "@neokapi/bowrain-client";
import { ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { RunText } from "./RunText";

/** The card's lift off the page; the border carries the edge in dark mode. */
export const CARD_SHADOW =
  "0px 14px 32px rgba(15, 23, 42, 0.14), 0px 2px 6px rgba(15, 23, 42, 0.08)";

function Tag({ label, testID, accent }: { label: string; testID?: string; accent?: boolean }) {
  return (
    <View testID={testID} className={`rounded-full px-3 py-1 ${accent ? "bg-chip" : "bg-muted"}`}>
      <Text
        className={`text-xs font-semibold ${accent ? "text-chip-foreground" : "text-muted-foreground"}`}
      >
        {label}
      </Text>
    </View>
  );
}

export function ReviewCard({ card, sourceLanguage }: { card: DeckCard; sourceLanguage: string }) {
  const { t } = useTranslation();
  const languages = sourceLanguage
    ? t("card.languages", {
        source: sourceLanguage.toUpperCase(),
        target: card.locale.toUpperCase(),
      })
    : card.locale.toUpperCase();
  const group = card.group === "fast" || card.group === "closer" ? card.group : null;
  return (
    <View
      testID="review-card"
      className="bg-card border-border flex-1 rounded-[28px] border"
      style={{ boxShadow: CARD_SHADOW }}
    >
      <View className="flex-row flex-wrap items-center gap-2 px-5 pt-5">
        <Tag label={languages} testID="card-languages" accent />
        {group ? <Tag label={t(`card.group.${group}`)} testID="card-group" /> : null}
        {card.preReview ? (
          <Tag label={t("card.preReview", { score: card.preReview.score })} />
        ) : null}
      </View>

      <ScrollView contentContainerClassName="gap-5 p-5">
        <View className="gap-2">
          <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
            {t("card.source")}
          </Text>
          <RunText runs={card.source} size="lg" muted testID="card-source" />
        </View>

        <View className="bg-border h-px" />

        <View className="gap-2">
          <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
            {t("card.translation")}
          </Text>
          {card.target.length > 0 ? (
            <RunText runs={card.target} size="xl" testID="card-target" />
          ) : (
            <Text className="text-muted-foreground text-lg italic">{t("card.noTranslation")}</Text>
          )}
        </View>

        <View className="gap-2">
          {card.failingFindingsCount > 0 ? (
            <Text className="text-corrective text-sm">
              {t("card.failingFindings", { count: card.failingFindingsCount })}
            </Text>
          ) : null}
          {card.notes.map((n, i) => (
            <View key={i} className="bg-muted rounded-xl px-3 py-2">
              <Text className="text-foreground text-sm">{n.text}</Text>
            </View>
          ))}
          {card.ownWork ? (
            <View className="bg-warning rounded-xl px-3 py-2">
              <Text className="text-warning-foreground text-sm">{t("card.ownWork")}</Text>
            </View>
          ) : null}
          {!card.preReview && card.notes.length === 0 && card.findingsCount === 0 ? (
            <Text className="text-muted-foreground text-sm">{t("card.noContext")}</Text>
          ) : null}
        </View>
      </ScrollView>

      <View className="border-border gap-0.5 border-t px-5 py-3">
        {card.name ? (
          <Text className="text-muted-foreground font-mono text-xs" numberOfLines={1}>
            {card.name}
          </Text>
        ) : null}
        <Text className="text-muted-foreground text-xs" numberOfLines={1}>
          {card.item}
        </Text>
      </View>
    </View>
  );
}
