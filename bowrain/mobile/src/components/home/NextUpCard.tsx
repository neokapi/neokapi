// The first card on Home: the deck that waits for the person, with how much
// waits in it and a button that opens it. Below it, a link to the other decks
// waiting, which the Review tab lists.
import Ionicons from "@expo/vector-icons/Ionicons";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { languageLabel } from "../../lib/feed";
import type { NextUp } from "../../lib/home";
import { LocaleBadge } from "./parts";
import { ReviewMix } from "./ReviewMix";

export function NextUpCard({
  next,
  showWorkspace,
  onOpen,
  onMore,
}: {
  next: NextUp;
  showWorkspace: boolean;
  onOpen: () => void;
  onMore?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [primaryForeground, primary] = useCSSVariable([
    "--color-primary-foreground",
    "--color-primary",
  ]);
  const e = next.entry;
  const where = showWorkspace ? `${e.project_name} · ${e.workspace_name}` : e.project_name;
  const title =
    next.kind === "language" ? languageLabel(next.entry.locale, i18n.language) : t("context.title");
  const count = e.waiting;
  const label =
    next.kind === "language"
      ? t("home.next.languageLabel", { language: title, where, count })
      : t("context.entryLabel", { where, count });

  return (
    <View
      testID="home-next"
      className="bg-card border-border mx-4 overflow-hidden rounded-3xl border shadow-sm"
    >
      <View className="from-prism-1 via-prism-3 to-prism-5 h-1.5 bg-linear-to-r" />
      <View className="gap-4 p-5">
        <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
          {t("home.next.eyebrow")}
        </Text>
        <View className="flex-row items-center gap-3">
          {next.kind === "language" ? (
            <LocaleBadge locale={next.entry.locale} />
          ) : (
            <View
              accessibilityElementsHidden
              importantForAccessibility="no-hide-descendants"
              className="from-prism-4 to-prism-5 h-11 w-11 items-center justify-center rounded-xl bg-linear-to-br"
            >
              <Ionicons name="sparkles" size={22} color="#ffffff" />
            </View>
          )}
          <View className="flex-1 gap-0.5">
            <Text className="text-foreground text-2xl font-bold" numberOfLines={2}>
              {title}
            </Text>
            <Text className="text-muted-foreground text-sm" numberOfLines={1}>
              {where}
            </Text>
          </View>
          <View className="items-end">
            <Text className="text-foreground text-3xl font-bold tabular-nums">{count}</Text>
            <Text className="text-muted-foreground text-xs">{t("home.waiting")}</Text>
          </View>
        </View>
        {next.kind === "language" ? (
          <ReviewMix testID="home-next-mix" mix={next.entry} />
        ) : (
          <Text className="text-muted-foreground text-xs tabular-nums">
            {t("context.kinds", {
              voice: next.entry.by_kind.voice_rule ?? 0,
              terms: next.entry.by_kind.concept ?? 0,
              rules: next.entry.by_kind.term_rule ?? 0,
              notes: next.entry.by_kind.note ?? 0,
            })}
          </Text>
        )}
        <Pressable
          testID="home-next-open"
          accessibilityRole="button"
          accessibilityLabel={label}
          accessibilityHint={t("home.next.hint")}
          onPress={onOpen}
          className="bg-primary min-h-[50px] flex-row items-center justify-center gap-2 rounded-2xl px-4 py-3 active:opacity-80"
        >
          <Text className="text-primary-foreground text-base font-semibold">
            {t("home.next.start")}
          </Text>
          <Ionicons name="arrow-forward" size={18} color={String(primaryForeground)} />
        </Pressable>
      </View>
      {next.more > 0 && onMore ? (
        <Pressable
          testID="home-next-more"
          accessibilityRole="button"
          onPress={onMore}
          className="border-border min-h-[48px] flex-row items-center justify-between border-t px-5 py-3 active:opacity-70"
        >
          <Text className="text-primary text-sm font-medium">
            {t("home.next.more", { count: next.more })}
          </Text>
          <Ionicons name="chevron-forward" size={16} color={String(primary)} />
        </Pressable>
      ) : null}
    </View>
  );
}

/** The first card on Home when nothing waits anywhere. */
export function CaughtUpCard() {
  const { t } = useTranslation();
  const approve = useCSSVariable("--color-approve");
  return (
    <View
      testID="home-caught-up"
      accessible
      className="bg-card border-border mx-4 flex-row items-center gap-4 rounded-3xl border p-5"
    >
      <View
        accessibilityElementsHidden
        importantForAccessibility="no-hide-descendants"
        className="bg-muted h-12 w-12 items-center justify-center rounded-full"
      >
        <Ionicons name="checkmark-done" size={26} color={String(approve)} />
      </View>
      <View className="flex-1 gap-0.5">
        <Text className="text-foreground text-lg font-semibold">{t("inbox.emptyTitle")}</Text>
        <Text className="text-muted-foreground text-sm">{t("inbox.emptyBody")}</Text>
      </View>
    </View>
  );
}
