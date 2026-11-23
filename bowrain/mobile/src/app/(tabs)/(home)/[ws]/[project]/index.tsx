// A project's home, under its name as a large title: the project's mark and
// languages, its review plan, the languages the person may review, each with
// the translations waiting and how the deck groups them, and then its context
// waiting for review and who worked on it lately. A language opens its deck in one tap. The server says
// which languages the review permission covers; the app lists those and no
// others. What waits comes from the review inbox the Review tab reads, so a
// project shows the numbers the Review tab shows.
import type { Project } from "@neokapi/bowrain-client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Redirect, router, Stack, useLocalSearchParams } from "expo-router";
import { useCallback, useEffect } from "react";
import { FlatList, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState } from "../../../../../components/List";
import { ActivityStrip } from "../../../../../components/home/ActivityStrip";
import { ContextEntryCard, LanguageCard } from "../../../../../components/home/HomeCards";
import { Monogram } from "../../../../../components/home/Monogram";
import {
  CountsNotice,
  EmptyState,
  SectionHeader,
  SkeletonCards,
} from "../../../../../components/home/parts";
import { useAuth } from "../../../../../lib/auth";
import { entriesOf, languageLabel, useFeed } from "../../../../../lib/feed";
import {
  inboxCounts,
  inboxFailed,
  projectKey,
  summarize,
  useReviewHome,
} from "../../../../../lib/home";
import { setRecentProject } from "../../../../../lib/recentProject";
import { reviewPlanKey } from "../../../../../lib/plan";
import { usePullRefresh } from "../../../../../lib/pullRefresh";
import { routeValue } from "../../../../../lib/routeParams";

/** The title key of each plan level, for the project's plan card. */
const planStepTitle = {
  coordinates: "plan.coordinates.title",
  context: "plan.context.title",
  exceptions: "plan.exceptions.title",
  interfaces: "plan.interfaces.title",
  policy: "plan.policy.title",
} as const;

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

/** The header's place while the project's details load: the same row, with the mark's size. */
function ProjectHeaderPlaceholder() {
  return (
    <View
      testID="project-header-loading"
      className="flex-row items-center gap-4 px-5 pt-2 pb-4"
      importantForAccessibility="no-hide-descendants"
    >
      <View className="bg-muted h-14 w-14 rounded-2xl" />
      <View className="bg-muted h-3 flex-1 rounded-full" />
    </View>
  );
}

// The screen waits for a workspace and a project: a parameter that is a route
// group's name is neither, and leads back to the workspace list.
export default function ProjectHome() {
  const params = useLocalSearchParams<{ ws: string; project: string }>();
  const ws = routeValue(params.ws);
  const project = routeValue(params.project);
  if (ws === undefined || project === undefined) return <Redirect href="/workspaces" />;
  return <Languages ws={ws} project={project} />;
}

function Languages({ ws, project }: { ws: string; project: string }) {
  const { t } = useTranslation();
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

  // The context waiting for review, from the inbox the Review tab reads.
  const inbox = useReviewHome();
  const waiting = summarize(inbox.data).byProject.get(projectKey(ws, project));
  const counts = inboxCounts(inbox);
  const contextWaiting = waiting?.contextEntry?.waiting ?? 0;
  const byKind = waiting?.contextEntry?.by_kind ?? {};
  const contextDetail = t("context.kinds", {
    voice: byKind.voice_rule ?? 0,
    terms: byKind.concept ?? 0,
    rules: byKind.term_rule ?? 0,
    notes: byKind.note ?? 0,
  });

  // The review plan leads: what to decide next, from the coordinates down.
  const plan = useQuery({
    queryKey: reviewPlanKey(ws, project),
    queryFn: () => client.reviewPlan({ workspace: ws, project }),
  });
  const planDetail = plan.data
    ? plan.data.next
      ? t("plan.homeNext", { step: t(planStepTitle[plan.data.next]) })
      : t("plan.homeDone")
    : undefined;

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
        loading={counts === "loading"}
        unknown={counts === "unknown"}
        onPress={openLanguage}
      />
    ),
    [waiting, counts, openLanguage],
  );

  const pull = usePullRefresh(() => Promise.all([feed.refetch(), inbox.refetch(), plan.refetch()]));

  // Each section fills in place as its query answers. The header, the plan
  // card and the language cards hold their size while loading. The context
  // row and the activity strip show only when something is there, so they sit
  // below the languages and appear once everything above them has its final
  // size: a late one is added below the content, never above it.
  const languagesFinal = !perms.isPending && (info !== undefined || !projects.isPending);
  const extrasReady = languagesFinal && !inbox.isPending && !feed.isPending;
  // Before the permissions answer, the project's own languages hold the place
  // of the list the permissions narrow, as cards still loading.
  const listed = perms.isPending ? (info?.target_languages ?? []) : languages;
  const planText = planDetail ?? (plan.isError ? t("common.loadFailed") : t("home.loading"));

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen
        options={{ title: info?.name ?? t("languages.title"), headerLargeTitle: true }}
      />
      <FlatList
        testID="language-list"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pb-8"
        data={listed}
        keyExtractor={(l) => l}
        renderItem={renderItem}
        refreshing={pull.refreshing}
        onRefresh={pull.onRefresh}
        ListHeaderComponent={
          <View>
            {info ? <ProjectHeader project={info} /> : <ProjectHeaderPlaceholder />}
            <Pressable
              testID="project-plan"
              accessibilityRole="button"
              accessibilityState={{ busy: plan.isPending }}
              onPress={() =>
                router.push({ pathname: "/[ws]/[project]/plan", params: { ws, project } })
              }
              className="border-primary/40 bg-primary/10 mx-4 mb-3 rounded-3xl border p-4 active:opacity-80"
            >
              <Text className="text-foreground text-[17px] font-semibold">{t("plan.home")}</Text>
              <Text className="text-muted-foreground text-sm" numberOfLines={1}>
                {planText}
              </Text>
            </Pressable>
            {listed.length > 0 || !languagesFinal ? (
              <SectionHeader title={t("home.languagesHeader")} />
            ) : null}
          </View>
        }
        ListEmptyComponent={
          !languagesFinal ? (
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
        ListFooterComponent={
          extrasReady ? (
            <View className="pt-3">
              {inboxFailed(inbox) && !inbox.data ? (
                <CountsNotice onRetry={() => void inbox.refetch()} />
              ) : null}
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
            </View>
          ) : undefined
        }
      />
    </View>
  );
}
