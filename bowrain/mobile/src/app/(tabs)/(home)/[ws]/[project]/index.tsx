// A project's home, under its name as a large title: the project's mark and
// languages, its context waiting for review, who worked on it lately, and the
// languages the person may review, each with the translations waiting and how
// the deck groups them. A language opens its deck in one tap. The server says
// which languages the review permission covers; the app lists those and no
// others.
import type { DeckCounts, Project } from "@neokapi/bowrain-client";
import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { router, Stack, useLocalSearchParams } from "expo-router";
import { useCallback, useEffect } from "react";
import { FlatList, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState } from "../../../../../components/List";
import { ActivityStrip } from "../../../../../components/home/ActivityStrip";
import { ContextEntryCard, LanguageCard } from "../../../../../components/home/HomeCards";
import { Monogram } from "../../../../../components/home/Monogram";
import { EmptyState, SectionHeader, SkeletonCards } from "../../../../../components/home/parts";
import { useAuth } from "../../../../../lib/auth";
import { entriesOf, languageLabel, useFeed } from "../../../../../lib/feed";
import { projectKey, summarize, useReviewHome } from "../../../../../lib/home";
import { setRecentProject } from "../../../../../lib/recentProject";

type Counts = Partial<DeckCounts> & { total: number };

function ProjectHeader({ project }: { project: Project }) {
  const { t, i18n } = useTranslation();
  const facts = [
    project.default_source_language
      ? t("home.project.from", {
          language: languageLabel(project.default_source_language, i18n.language),
        })
      : undefined,
    t("projects.languages", { count: project.target_languages.length }),
    project.block_count ? t("home.project.blocks", { count: project.block_count }) : undefined,
  ].filter(Boolean);
  return (
    <View testID="project-header" className="flex-row items-center gap-4 px-5 pt-2 pb-4">
      <Monogram name={project.name} seed={project.id} size={56} />
      <Text className="text-muted-foreground flex-1 text-sm">{facts.join(" · ")}</Text>
    </View>
  );
}

export default function ProjectHome() {
  const { t } = useTranslation();
  const { ws, project } = useLocalSearchParams<{ ws: string; project: string }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const scope = { workspace: ws, project };

  const projects = useQuery({
    queryKey: ["projects", ws],
    queryFn: () => client.projects(ws),
    initialData: () => qc.getQueryData<Project[]>(["projects", ws]),
  });
  const info = projects.data?.find((p) => p.id === project);
  // The Activity tab follows the project the person last opened.
  const projectName = info?.name;
  useEffect(() => setRecentProject({ ws, project, name: projectName }), [ws, project, projectName]);
  const perms = useQuery({
    queryKey: ["permissions", ws, project],
    queryFn: () => client.permissions(ws, project),
  });

  const mayReview = perms.data?.permissions.includes("review") ?? false;
  const scoped = perms.data?.languages ?? [];
  const languages = mayReview
    ? (info?.target_languages ?? []).filter((l) => scoped.length === 0 || scoped.includes(l))
    : [];

  // The context waits before the languages: a translation is checked against it.
  const context = useQuery({
    queryKey: ["context-count", ws, project],
    queryFn: () => client.contextDeck(ws, project, { limit: 1 }),
    retry: false,
  });
  const contextWaiting = context.data?.counts.total ?? 0;
  // The inbox counts the context by kind; the deck alone splits it by author.
  const inbox = useReviewHome();
  const byKind = summarize(inbox.data).byProject.get(projectKey(ws, project))?.contextEntry
    ?.by_kind;
  const contextDetail = byKind
    ? t("context.kinds", {
        voice: byKind.voice_rule ?? 0,
        terms: byKind.concept ?? 0,
        rules: byKind.term_rule ?? 0,
        notes: byKind.note ?? 0,
      })
    : t("context.split", {
        agent: context.data?.counts.agent ?? 0,
        person: context.data?.counts.person ?? 0,
      });

  const feed = useFeed(ws, project);
  const entries = entriesOf(feed.data);

  const counts = useQueries({
    queries: languages.map((locale) => ({
      queryKey: ["counts", ws, project, locale],
      queryFn: async (): Promise<Counts> => {
        const deck = await client.deck(scope, locale, { limit: 1 });
        // A server without the deck route counts the queue but does not group it.
        return deck.counts ?? { total: deck.total };
      },
    })),
  });

  const openLanguage = useCallback(
    (locale: string) =>
      router.push({ pathname: "/[ws]/[project]/[locale]", params: { ws, project, locale } }),
    [ws, project],
  );
  const renderItem = useCallback(
    ({ item, index }: { item: string; index: number }) => (
      <LanguageCard locale={item} counts={counts[index]?.data} onPress={openLanguage} />
    ),
    [counts, openLanguage],
  );

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["counts", ws, project] });
    void context.refetch();
    void feed.refetch();
    void inbox.refetch();
  };

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen
        options={{ title: info?.name ?? t("languages.title"), headerLargeTitle: true }}
      />
      <FlatList
        testID="language-list"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pb-8"
        data={languages}
        keyExtractor={(l) => l}
        renderItem={renderItem}
        refreshing={false}
        onRefresh={refresh}
        ListHeaderComponent={
          <View>
            {info ? <ProjectHeader project={info} /> : null}
            {contextWaiting > 0 ? (
              <ContextEntryCard
                count={contextWaiting}
                detail={contextDetail}
                onPress={() =>
                  router.push({ pathname: "/[ws]/[project]/context", params: { ws, project } })
                }
              />
            ) : null}
            <ActivityStrip entries={entries} onPress={() => router.navigate("/activity")} />
            {languages.length > 0 ? <SectionHeader title={t("home.languagesHeader")} /> : null}
          </View>
        }
        ListEmptyComponent={
          perms.isPending || (projects.isPending && !info) ? (
            <SkeletonCards testID="languages-loading" />
          ) : perms.isSuccess ? (
            <EmptyState
              icon={mayReview ? "language-outline" : "lock-closed-outline"}
              title={mayReview ? t("languages.empty") : t("languages.noPermission")}
            />
          ) : (
            <QueryState query={perms} />
          )
        }
      />
    </View>
  );
}
