// A project's home, under its name as a large title: the project's mark and
// languages, its context waiting for review, who worked on it lately, and the
// languages the person may review, each with the translations waiting and how
// the deck groups them. A language opens its deck in one tap. The server says
// which languages the review permission covers; the app lists those and no
// others. What waits comes from the review inbox the Review tab reads, so a
// project shows the numbers the Review tab shows.
import type { Project } from "@neokapi/bowrain-client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
  const inbox = useReviewHome();
  const waiting = summarize(inbox.data).byProject.get(projectKey(ws, project));
  const contextWaiting = waiting?.contextEntry?.waiting ?? 0;
  const byKind = waiting?.contextEntry?.by_kind ?? {};
  const contextDetail = t("context.kinds", {
    voice: byKind.voice_rule ?? 0,
    terms: byKind.concept ?? 0,
    rules: byKind.term_rule ?? 0,
    notes: byKind.note ?? 0,
  });

  const feed = useFeed(ws, project);
  const entries = entriesOf(feed.data);

  const openLanguage = useCallback(
    (locale: string) =>
      router.push({ pathname: "/[ws]/[project]/[locale]", params: { ws, project, locale } }),
    [ws, project],
  );
  const renderItem = useCallback(
    ({ item }: { item: string }) => (
      <LanguageCard
        locale={item}
        waiting={waiting?.byLocale.get(item)}
        loading={inbox.isPending}
        onPress={openLanguage}
      />
    ),
    [waiting, inbox.isPending, openLanguage],
  );

  const refresh = () => {
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
