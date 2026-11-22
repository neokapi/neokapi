// The first card on Home: the next step of one project's plan, as the server
// names it, with one button that opens it. A deck step shows the language and
// how much waits there; the setup, the coordinates, the context and the
// settle pass show the step's title and one fact. The steps the server
// skipped because they only wait sit beside it, each with what it waits for.
// Below, a link to the other decks waiting, which the Review tab lists.
import Ionicons from "@expo/vector-icons/Ionicons";
import type { ComponentProps } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { languageLabel } from "../../lib/feed";
import { formatCount } from "../../lib/format";
import type { HomeStep } from "../../lib/home";
import { LocaleBadge } from "./parts";
import { PlanWaitingNote, stepFact } from "./PlanCard";
import { ReviewMix } from "./ReviewMix";

type IconName = ComponentProps<typeof Ionicons>["name"];

/** The icon a step's card leads with. */
function iconOf(next: HomeStep): IconName {
  if (next.kind === "setup") return "language";
  if (next.kind === "waiting") return "hourglass-outline";
  switch (next.step.level) {
    case "coordinates":
      return "map-outline";
    case "context":
      return "sparkles";
    default:
      return "checkmark-done";
  }
}

export function NextUpCard({
  next,
  showWorkspace,
  onOpen,
  onMore,
}: {
  next: HomeStep;
  showWorkspace: boolean;
  onOpen: () => void;
  onMore?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [primaryForeground, primary] = useCSSVariable([
    "--color-primary-foreground",
    "--color-primary",
  ]);
  const p = next.project;
  const where = showWorkspace ? `${p.projectName} · ${p.workspaceName}` : p.projectName;

  let eyebrow = t("home.next.step");
  let title: string;
  let fact: string | null = null;
  let button: string | null;
  let label: string;
  switch (next.kind) {
    case "deck":
      eyebrow = t("home.next.eyebrow");
      title = languageLabel(next.entry.locale, i18n.language);
      button = t("home.next.start");
      label = t("home.next.languageLabel", { language: title, where, count: next.entry.waiting });
      break;
    case "setup":
      title = t("home.next.setupTitle");
      fact = t("home.next.setupFact");
      button = t("planCard.setup");
      label = `${title}, ${where}`;
      break;
    case "step":
      title = t(`planCard.titles.${next.step.level}`);
      fact = stepFact(next.step, t, i18n.language);
      button = next.step.level === "coordinates" ? t("planCard.openMap") : t("planCard.open");
      label = `${title}, ${where}, ${fact}`;
      break;
    case "waiting":
      title = t("home.next.waitingTitle");
      button = null;
      label = `${title}, ${where}`;
      break;
  }

  return (
    <View
      testID="home-next"
      className="bg-card border-border mx-4 overflow-hidden rounded-3xl border shadow-sm"
    >
      <View className="from-prism-1 via-prism-3 to-prism-5 h-1.5 bg-linear-to-r" />
      <View className="gap-4 p-5">
        <Text
          testID="home-next-eyebrow"
          className="text-muted-foreground text-xs font-semibold tracking-wide uppercase"
        >
          {eyebrow}
        </Text>
        <View className="flex-row items-center gap-3">
          {next.kind === "deck" ? (
            <LocaleBadge locale={next.entry.locale} />
          ) : (
            <View
              accessibilityElementsHidden
              importantForAccessibility="no-hide-descendants"
              className="from-prism-4 to-prism-5 h-11 w-11 items-center justify-center rounded-xl bg-linear-to-br"
            >
              <Ionicons name={iconOf(next)} size={22} color="#ffffff" />
            </View>
          )}
          <View className="flex-1 gap-0.5">
            <Text
              testID="home-next-title"
              className="text-foreground text-2xl font-bold"
              numberOfLines={2}
            >
              {title}
            </Text>
            <Text className="text-muted-foreground text-sm" numberOfLines={1}>
              {where}
            </Text>
          </View>
          {next.kind === "deck" ? (
            <View className="items-end">
              <Text className="text-foreground text-3xl font-bold tabular-nums">
                {formatCount(next.entry.waiting, i18n.language)}
              </Text>
              <Text className="text-muted-foreground text-xs">{t("home.waiting")}</Text>
            </View>
          ) : null}
        </View>
        {next.kind === "deck" ? <ReviewMix testID="home-next-mix" mix={next.entry} /> : null}
        {fact ? (
          <Text testID="home-next-fact" className="text-muted-foreground text-sm">
            {fact}
          </Text>
        ) : null}
        <PlanWaitingNote testID="home-next-waiting" waiting={next.waiting} />
        {button ? (
          <Pressable
            testID="home-next-open"
            accessibilityRole="button"
            accessibilityLabel={label}
            accessibilityHint={next.kind === "deck" ? t("home.next.hint") : undefined}
            onPress={onOpen}
            className="bg-primary min-h-[50px] flex-row items-center justify-center gap-2 rounded-2xl px-4 py-3 active:opacity-80"
          >
            <Text className="text-primary-foreground text-base font-semibold">{button}</Text>
            <Ionicons name="arrow-forward" size={18} color={String(primaryForeground)} />
          </Pressable>
        ) : null}
      </View>
      {next.more > 0 && onMore ? (
        <Pressable
          testID="home-next-more"
          accessibilityRole="button"
          onPress={onMore}
          className="border-border min-h-[48px] flex-row items-center justify-between border-t px-5 py-3 active:opacity-70"
        >
          <Text className="text-primary text-sm font-medium">
            {next.kind === "deck"
              ? t("home.next.more", { count: next.more })
              : t("home.next.decks", { count: next.more })}
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
