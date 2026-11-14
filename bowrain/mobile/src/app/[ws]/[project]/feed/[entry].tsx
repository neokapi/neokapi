// One feed entry: its counts, who did it and for whom, when, where it sits,
// and for a recipe change the values before and after. An entry that links
// to a review deck offers one button to it.
import { countsOf, type FeedEntry, type FeedPage } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import type { InfiniteData } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { router, Stack, useLocalSearchParams } from "expo-router";
import type { ReactNode } from "react";
import { Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { actorIcon, kindIcon } from "../../../../components/FeedRow";
import {
  actorLabel,
  countLabel,
  deckLocaleOf,
  entriesOf,
  entryTitle,
  feedKey,
  languageLabel,
  timeOf,
} from "../../../../lib/feed";

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <View className="border-border flex-row gap-4 border-b px-4 py-3">
      <Text className="text-muted-foreground w-28 text-sm">{label}</Text>
      <View className="min-w-0 flex-1">{children}</View>
    </View>
  );
}

function Value({ children }: { children: ReactNode }) {
  return <Text className="text-foreground text-sm">{children}</Text>;
}

function coordinatesText(c: Record<string, string> | undefined, none: string): string {
  const parts = Object.entries(c ?? {}).map(([axis, value]) => `${axis}: ${value}`);
  return parts.length ? parts.join(", ") : none;
}

export default function FeedEntryDetail() {
  const { t, i18n } = useTranslation();
  const {
    ws,
    project,
    entry: entryId,
  } = useLocalSearchParams<{
    ws: string;
    project: string;
    entry: string;
  }>();
  const qc = useQueryClient();
  const [foreground, muted] = useCSSVariable(["--color-foreground", "--color-muted-foreground"]);
  const entry: FeedEntry | undefined = entriesOf(
    qc.getQueryData<InfiniteData<FeedPage>>(feedKey(ws, project)),
  ).find((e) => e.id === entryId);

  if (!entry) {
    return (
      <View className="bg-background flex-1 items-center p-8">
        <Text className="text-muted-foreground text-base">{t("feed.notFound")}</Text>
      </View>
    );
  }

  const lang = i18n.language;
  const title = entryTitle(entry, t, lang);
  const counts = countsOf(entry).filter(
    (c) => entry.kind !== "coverage_milestone" || c.key !== "percent",
  );
  const deckLocale = deckLocaleOf(entry);
  const when = new Date(entry.at);
  const date = Number.isNaN(when.getTime())
    ? entry.at
    : `${new Intl.DateTimeFormat(lang, { dateStyle: "medium" }).format(when)} ${timeOf(entry.at, lang)}`;
  const none = t("feed.detail.none");

  return (
    <ScrollView className="bg-background flex-1" contentInsetAdjustmentBehavior="automatic">
      <Stack.Screen options={{ title }} />
      {counts.length > 0 ? (
        <View className="flex-row flex-wrap items-center gap-2 px-4 pt-4 pb-4">
          <Ionicons name={kindIcon(entry.kind)} size={22} color={String(foreground)} />
          {counts.map((c) => (
            <View key={c.key} className="bg-card border-border rounded-lg border px-3 py-2">
              <Text className="text-foreground text-sm tabular-nums">{countLabel(t, c)}</Text>
            </View>
          ))}
        </View>
      ) : null}

      <View className="bg-card border-border border-t">
        <Fact label={t("feed.detail.who")}>
          <View className="flex-row items-center gap-1.5">
            <Ionicons name={actorIcon(entry.actor.kind)} size={16} color={String(muted)} />
            <Value>
              {actorLabel(entry, t)} · {t(`feed.actorKind.${entry.actor.kind}`)}
            </Value>
          </View>
        </Fact>
        {entry.on_behalf_of ? (
          <Fact label={t("feed.detail.for")}>
            <Value>{entry.on_behalf_of.name ?? entry.on_behalf_of.id}</Value>
          </Fact>
        ) : null}
        {entry.actor.session ? (
          <Fact label={t("feed.detail.session")}>
            <Value>{[entry.actor.host, entry.actor.session].filter(Boolean).join(" · ")}</Value>
          </Fact>
        ) : null}
        <Fact label={t("feed.detail.when")}>
          <Value>{date}</Value>
        </Fact>
        {entry.stream ? (
          <Fact label={t("feed.detail.stream")}>
            <Value>{entry.stream}</Value>
          </Fact>
        ) : null}
        {entry.locales?.length ? (
          <Fact label={t("feed.detail.languages")}>
            <Value>{entry.locales.map((l) => languageLabel(l, lang)).join(", ")}</Value>
          </Fact>
        ) : null}
        {entry.coordinates ? (
          <Fact label={t("feed.detail.where")}>
            <Value>{coordinatesText(entry.coordinates, none)}</Value>
          </Fact>
        ) : null}
        {entry.diff?.targets ? (
          <Fact label={t("feed.detail.targets")}>
            <Value>
              {t("feed.detail.before")}: {entry.diff.targets.before.join(", ") || none}
            </Value>
            <Value>
              {t("feed.detail.after")}: {entry.diff.targets.after.join(", ") || none}
            </Value>
          </Fact>
        ) : null}
        {(entry.diff?.collections ?? []).map((c) => (
          <Fact key={`c-${c.name}`} label={c.name}>
            <Value>
              {t("feed.detail.before")}: {coordinatesText(c.before, none)}
            </Value>
            <Value>
              {t("feed.detail.after")}: {coordinatesText(c.after, none)}
            </Value>
          </Fact>
        ))}
        {(entry.diff?.settings ?? []).map((s) => (
          <Fact key={`s-${s.name}`} label={s.name}>
            <Value>
              {s.before || none} → {s.after || none}
            </Value>
          </Fact>
        ))}
      </View>

      {deckLocale ? (
        <Pressable
          accessibilityRole="button"
          onPress={() =>
            router.push({
              pathname: "/[ws]/[project]/[locale]",
              params: { ws, project, locale: deckLocale },
            })
          }
          className="bg-primary m-4 items-center rounded-lg py-3 active:opacity-80"
        >
          <Text className="text-primary-foreground text-base font-semibold">
            {t("feed.openDeck")}
          </Text>
        </Pressable>
      ) : null}
    </ScrollView>
  );
}
