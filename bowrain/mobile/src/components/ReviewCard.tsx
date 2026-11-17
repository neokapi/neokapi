// One block's translation as a card in the deck: the language and group as
// chips, the product and surface the text ships on, the source above the
// translation with codes as chips, why the card needs a closer look, and what
// the server says about it. The footer names the file and key, and opens the
// details sheet.
import type { DeckCard } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { RunText } from "./RunText";
import { fileName } from "../lib/where";

/** The reason codes a card's reason line words; another code shows nothing. */
const REASONS = new Set(["stale_source", "redraft", "long_source", "no_pre_review"]);

/**
 * Why the card is not a quick one, worded in the reader's language. Findings
 * and the pre-review score already show as notes and chips, so only the
 * reasons nothing else on the card says are worded.
 */
export function reasonKey(card: DeckCard): string | null {
  if (card.group !== "closer" && card.group !== "desk") return null;
  if (!card.reasonCode || !REASONS.has(card.reasonCode)) return null;
  return `card.why.${card.reasonCode}`;
}

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

export function ReviewCard({
  card,
  sourceLanguage,
  where,
  onDetails,
}: {
  card: DeckCard;
  sourceLanguage: string;
  /** The product and surface the text ships on, such as ["Neokapi", "Desktop"]. */
  where?: string[];
  /** Opens the details sheet; without it the footer is plain text. */
  onDetails?: () => void;
}) {
  const { t } = useTranslation();
  const muted = useCSSVariable("--color-muted-foreground") as string | undefined;
  const why = reasonKey(card);
  const file = fileName(card.item);
  const footer = (
    <>
      <View className="min-w-0 flex-1 gap-0.5">
        <Text className="text-muted-foreground text-xs" numberOfLines={1}>
          {file}
        </Text>
        {card.name ? (
          <Text className="text-muted-foreground font-mono text-xs" numberOfLines={1}>
            {card.name}
          </Text>
        ) : null}
      </View>
      {onDetails ? (
        <View className="flex-row items-center gap-1">
          <Text className="text-primary text-sm font-semibold">{t("card.details")}</Text>
          <Ionicons name="chevron-forward" size={14} color={muted} />
        </View>
      ) : null}
    </>
  );
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
      {where && where.length > 0 ? (
        <Text
          testID="card-where"
          className="text-muted-foreground px-5 pt-2 text-sm font-medium"
          numberOfLines={1}
        >
          {where.join(" · ")}
        </Text>
      ) : null}

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
          {why ? (
            <Text testID="card-why" className="text-muted-foreground text-sm">
              {t(why)}
            </Text>
          ) : null}
          {card.copies && card.copies.length > 0 ? (
            <Text testID="card-copies" className="text-foreground text-sm">
              {t("card.copies", { count: card.copies.length })}
            </Text>
          ) : null}
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
        </View>
      </ScrollView>

      {onDetails ? (
        <Pressable
          testID="card-details"
          accessibilityRole="button"
          accessibilityLabel={t("card.detailsLabel", { file })}
          onPress={onDetails}
          className="border-border flex-row items-center gap-3 border-t px-5 py-3 active:opacity-60"
        >
          {footer}
        </Pressable>
      ) : (
        <View className="border-border flex-row items-center gap-3 border-t px-5 py-3">
          {footer}
        </View>
      )}
    </View>
  );
}
