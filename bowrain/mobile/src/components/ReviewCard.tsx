// One block's translation as a card: source and translation with codes as
// chips, where the block sits, and what the server says about it.
import type { DeckCard } from "@neokapi/bowrain-client";
import { ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { RunText } from "./RunText";

export function ReviewCard({ card, sourceLanguage }: { card: DeckCard; sourceLanguage: string }) {
  const { t } = useTranslation();
  return (
    <View testID="review-card" className="bg-card border-border flex-1 rounded-3xl border p-5 shadow-sm">
      <ScrollView contentContainerClassName="gap-5">
        <View className="gap-1">
          {card.name ? (
            <Text className="text-muted-foreground font-mono text-xs" numberOfLines={1}>
              {card.name}
            </Text>
          ) : null}
          <Text className="text-muted-foreground text-xs" numberOfLines={1}>
            {card.item}
          </Text>
        </View>

        <View className="gap-2">
          <Text className="text-muted-foreground text-xs font-semibold uppercase">{sourceLanguage}</Text>
          <RunText runs={card.source} size="lg" testID="card-source" />
        </View>

        <View className="bg-border h-px" />

        <View className="gap-2">
          <Text className="text-muted-foreground text-xs font-semibold uppercase">{card.locale}</Text>
          {card.target.length > 0 ? (
            <RunText runs={card.target} size="lg" testID="card-target" />
          ) : (
            <Text className="text-muted-foreground text-base italic">{t("card.noTranslation")}</Text>
          )}
        </View>

        <View className="gap-2">
          {card.preReview ? (
            <Text className="text-muted-foreground text-sm">
              {t("card.preReview", { score: card.preReview.score })}
            </Text>
          ) : null}
          {card.failingFindingsCount > 0 ? (
            <Text className="text-corrective text-sm">
              {t("card.failingFindings", { count: card.failingFindingsCount })}
            </Text>
          ) : null}
          {card.notes.map((n, i) => (
            <View key={i} className="bg-muted rounded-lg px-3 py-2">
              <Text className="text-foreground text-sm">{n.text}</Text>
            </View>
          ))}
          {card.ownWork ? (
            <View className="bg-warning rounded-lg px-3 py-2">
              <Text className="text-warning-foreground text-sm">{t("card.ownWork")}</Text>
            </View>
          ) : null}
          {!card.preReview && card.notes.length === 0 && card.findingsCount === 0 ? (
            <Text className="text-muted-foreground text-sm">{t("card.noContext")}</Text>
          ) : null}
        </View>
      </ScrollView>
    </View>
  );
}
