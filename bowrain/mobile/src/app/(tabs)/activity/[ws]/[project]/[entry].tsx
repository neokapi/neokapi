// One feed entry, in the feed's own shape: who did what as a sentence under
// their avatar, when, and the stream, languages and coordinates as chips.
// Below it, what the entry is about: a recipe change lists its collections,
// languages and settings in sections; a review entry lists its decisions with
// Undo; any other entry shows the preview its feed card shows. An entry about
// a language offers its review deck.
import type { FeedCollectionChange, FeedEntry, FeedPage } from "@neokapi/bowrain-client";
import { countsOf, targetChanges } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import type { InfiniteData } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { router, Stack, useLocalSearchParams } from "expo-router";
import { useState, type ReactNode } from "react";
import { Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { Avatar } from "../../../../../components/feed/Avatar";
import { Chip, ChipRow } from "../../../../../components/feed/parts";
import { Preview } from "../../../../../components/feed/Preview";
import { ReviewDecisions } from "../../../../../components/feed/ReviewDecisions";
import {
  actionOf,
  actorLabel,
  actorName,
  ageLabel,
  countLabel,
  deckLocaleOf,
  entriesOf,
  entryTitle,
  feedKey,
  languageLabel,
  timeOf,
} from "../../../../../lib/feed";

const CARD = "bg-card border-border mx-4 rounded-3xl border p-4";

/** Rows a section shows before Show all. */
const SECTION_ROWS = 6;

function Section({
  title,
  count,
  rows,
  testID,
}: {
  title: string;
  count: number;
  rows: ReactNode[];
  testID?: string;
}) {
  const { t } = useTranslation();
  const primary = useCSSVariable("--color-primary");
  const [all, setAll] = useState(false);
  const shown = all ? rows : rows.slice(0, SECTION_ROWS);
  return (
    <View testID={testID} className="mt-6 gap-2">
      <View className="mx-5 flex-row items-baseline gap-2">
        <Text accessibilityRole="header" className="text-foreground text-base font-semibold">
          {title}
        </Text>
        <Text className="text-muted-foreground text-sm tabular-nums">{count}</Text>
      </View>
      <View className="bg-card border-border mx-4 overflow-hidden rounded-3xl border">
        {shown.map((row, i) => (
          <View key={i} className={i > 0 ? "border-border border-t" : ""}>
            {row}
          </View>
        ))}
        {rows.length > SECTION_ROWS ? (
          <Pressable
            accessibilityRole="button"
            onPress={() => setAll((v) => !v)}
            className="border-border flex-row items-center justify-center gap-1 border-t py-3 active:opacity-60"
          >
            <Text className="text-primary text-sm font-medium">
              {all ? t("feed.showLess") : t("feed.detail.showAll", { count: rows.length })}
            </Text>
            <Ionicons
              name={all ? "chevron-up" : "chevron-down"}
              size={14}
              color={String(primary)}
            />
          </Pressable>
        ) : null}
      </View>
    </View>
  );
}

/** A collection and where it sits, or how its place changed. */
function CollectionRow({ c }: { c: FeedCollectionChange }) {
  const { t } = useTranslation();
  const before = c.before ?? {};
  const after = c.after ?? {};
  let chips: ReactNode[];
  if (c.change === "coordinates") {
    const axes = [...new Set([...Object.keys(before), ...Object.keys(after)])].sort();
    chips = axes
      .filter((a) => before[a] !== after[a])
      .map((a) => (
        <Chip key={a} tone="accent">
          {`${a}: ${before[a] ?? t("feed.detail.none")} → ${after[a] ?? t("feed.detail.none")}`}
        </Chip>
      ));
  } else {
    const at = c.change === "removed" ? before : after;
    chips = Object.entries(at)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([axis, value]) => <Chip key={axis}>{`${axis}: ${value}`}</Chip>);
  }
  return (
    <View testID={`collection-${c.name}`} className="gap-2 px-4 py-3">
      <Text className="text-foreground text-[15px] font-medium" numberOfLines={1}>
        {c.name}
      </Text>
      {chips.length > 0 ? (
        <ChipRow>{chips}</ChipRow>
      ) : (
        <Text className="text-muted-foreground text-sm">{t("feed.detail.noCoordinates")}</Text>
      )}
    </View>
  );
}

function LanguageRow({
  locale,
  added,
  onOpen,
}: {
  locale: string;
  added: boolean;
  onOpen?: () => void;
}) {
  const { i18n } = useTranslation();
  const [approve, reject, muted] = useCSSVariable([
    "--color-approve",
    "--color-reject",
    "--color-muted-foreground",
  ]);
  return (
    <Pressable
      disabled={!onOpen}
      onPress={onOpen}
      accessibilityRole={onOpen ? "button" : undefined}
      className="flex-row items-center gap-3 px-4 py-3 active:opacity-70"
    >
      <Ionicons
        name={added ? "add-circle" : "remove-circle"}
        size={20}
        color={String(added ? approve : reject)}
      />
      <Text className="text-foreground flex-1 text-[15px]">
        {languageLabel(locale, i18n.language)}
      </Text>
      {onOpen ? <Ionicons name="chevron-forward" size={16} color={String(muted)} /> : null}
    </Pressable>
  );
}

function RecipeChanges({
  entry,
  onOpenLanguage,
}: {
  entry: FeedEntry;
  onOpenLanguage: (locale: string) => void;
}) {
  const { t } = useTranslation();
  const collections = entry.diff?.collections ?? [];
  const groups: { key: FeedCollectionChange["change"]; title: string }[] = [
    { key: "added", title: t("feed.detail.collectionsAdded") },
    { key: "coordinates", title: t("feed.detail.collectionsMoved") },
    { key: "removed", title: t("feed.detail.collectionsRemoved") },
  ];
  const { added, removed } = targetChanges(entry);
  const settings = entry.diff?.settings ?? [];
  return (
    <>
      {added.length + removed.length > 0 ? (
        <Section
          testID="detail-languages"
          title={t("feed.detail.targets")}
          count={added.length + removed.length}
          rows={[
            ...added.map((l) => (
              <LanguageRow key={`+${l}`} locale={l} added onOpen={() => onOpenLanguage(l)} />
            )),
            ...removed.map((l) => <LanguageRow key={`-${l}`} locale={l} added={false} />),
          ]}
        />
      ) : null}
      {groups.map((g) => {
        const list = collections.filter((c) => c.change === g.key);
        return list.length > 0 ? (
          <Section
            key={g.key}
            testID={`detail-collections-${g.key}`}
            title={g.title}
            count={list.length}
            rows={list.map((c) => (
              <CollectionRow key={c.name} c={c} />
            ))}
          />
        ) : null;
      })}
      {settings.length > 0 ? (
        <Section
          testID="detail-settings"
          title={t("feed.detail.settings")}
          count={settings.length}
          rows={settings.map((s) => (
            <View key={s.name} className="gap-1 px-4 py-3">
              <Text className="text-foreground text-[15px] font-medium">{s.name}</Text>
              <ChipRow>
                <Chip tone="removed">{s.before || t("feed.detail.none")}</Chip>
                <Chip tone="added">{s.after || t("feed.detail.none")}</Chip>
              </ChipRow>
            </View>
          ))}
        />
      ) : null}
    </>
  );
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
  const [muted, primaryForeground] = useCSSVariable([
    "--color-muted-foreground",
    "--color-primary-foreground",
  ]);
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
  const openLanguage = (locale: string) =>
    router.push({ pathname: "/[ws]/[project]/[locale]", params: { ws, project, locale } });
  const deckLocale = deckLocaleOf(entry);
  const when = new Date(entry.at);
  const date = Number.isNaN(when.getTime())
    ? entry.at
    : `${new Intl.DateTimeFormat(lang, { dateStyle: "medium" }).format(when)}, ${timeOf(entry.at, lang)}`;
  const counts = countsOf(entry).filter(
    (c) => entry.kind !== "coverage_milestone" || c.key !== "percent",
  );
  const forName =
    entry.actor.kind !== "person" && entry.on_behalf_of?.name
      ? entry.on_behalf_of.viewer
        ? t("feed.forYou")
        : t("feed.forMember", { name: entry.on_behalf_of.name })
      : undefined;
  const icon = (name: "git-branch-outline" | "terminal-outline" | "location-outline") => (
    <Ionicons name={name} size={13} color={String(muted)} />
  );

  return (
    <ScrollView
      className="bg-background flex-1"
      contentInsetAdjustmentBehavior="automatic"
      contentContainerClassName="pt-2 pb-32"
    >
      <Stack.Screen options={{ title: entryTitle(entry, t, lang) }} />

      <View testID="detail-header" className={`${CARD} gap-4`}>
        <View className="flex-row gap-3">
          <Avatar actor={entry.actor} name={actorLabel(entry, t)} size={48} />
          <View className="min-w-0 flex-1 justify-center gap-0.5">
            <Text className="text-foreground text-[17px] leading-6">
              <Text className="font-semibold">{actorName(entry, t)}</Text>{" "}
              {actionOf(entry, t, lang)}
            </Text>
            <Text className="text-muted-foreground text-[13px]">
              {[forName, date, ageLabel(entry.at, t, lang)].filter(Boolean).join(" · ")}
            </Text>
          </View>
        </View>

        {counts.length > 0 && entry.kind !== "recipe_change" ? (
          <ChipRow>
            {counts.map((c) => (
              <Chip key={c.key} tone="accent">
                {countLabel(t, c)}
              </Chip>
            ))}
          </ChipRow>
        ) : null}

        <ChipRow>
          {entry.stream ? <Chip leading={icon("git-branch-outline")}>{entry.stream}</Chip> : null}
          {(entry.locales ?? []).map((l) => (
            <Chip key={l} onPress={() => openLanguage(l)}>
              {languageLabel(l, lang)}
            </Chip>
          ))}
          {Object.entries(entry.coordinates ?? {}).map(([axis, value]) => (
            <Chip key={axis} leading={icon("location-outline")}>{`${axis}: ${value}`}</Chip>
          ))}
          {entry.actor.session ? (
            <Chip leading={icon("terminal-outline")}>
              {[entry.actor.host, entry.actor.session].filter(Boolean).join(" · ")}
            </Chip>
          ) : null}
        </ChipRow>
      </View>

      {entry.kind === "recipe_change" ? (
        <RecipeChanges entry={entry} onOpenLanguage={openLanguage} />
      ) : entry.kind === "review_summary" ? (
        <ReviewDecisions ws={ws} project={project} entry={entry.id} />
      ) : (
        <View className={`${CARD} mt-4`}>
          <Preview entry={entry} onOpenLanguage={openLanguage} />
        </View>
      )}

      {deckLocale ? (
        <Pressable
          testID="detail-open-deck"
          accessibilityRole="button"
          onPress={() => openLanguage(deckLocale)}
          className="bg-primary mx-4 mt-6 min-h-[50px] flex-row items-center justify-center gap-2 rounded-2xl px-4 py-3 active:opacity-80"
        >
          <Text className="text-primary-foreground text-base font-semibold">
            {t("feed.openDeck")}
          </Text>
          <Ionicons name="arrow-forward" size={18} color={String(primaryForeground)} />
        </Pressable>
      ) : null}
    </ScrollView>
  );
}
