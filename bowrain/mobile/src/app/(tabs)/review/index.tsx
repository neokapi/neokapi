// The review inbox: each followed project's plan as five rings, the projects
// whose context waits for the person's review, then every language they
// follow, across projects and workspaces, where translations are waiting,
// most waiting first. The server lists all three, counts them as each deck
// counts them and orders them (GET /auth/me/review); the app shows the lists
// as they come. A ring opens its step of the plan, a language its
// collections. The header chip names the languages followed and opens their
// settings.
import type {
  ContextInboxEntry,
  PlanLevel,
  ReviewInboxLanguage,
  ReviewInboxProject,
} from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useQuery } from "@tanstack/react-query";
import { router, Stack, useFocusEffect } from "expo-router";
import { useCallback } from "react";
import { FlatList, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { ContextEntryCard, LanguageCard } from "../../../components/home/HomeCards";
import { EmptyState, SkeletonCards } from "../../../components/home/parts";
import { PlanCard } from "../../../components/home/PlanCard";
import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { languageLabel } from "../../../lib/feed";
import { followedChipLabel } from "../../../lib/languages";
import { setRecentProject } from "../../../lib/recentProject";

const inboxKey = ["review-inbox"] as const;

/** Where an inbox entry sits: its project, and its workspace when the list spans several. */
function whereOf(
  e: { project_name: string; workspace_name: string },
  showWorkspace: boolean,
): string {
  return showWorkspace ? `${e.project_name} · ${e.workspace_name}` : e.project_name;
}

/** The header chip: the languages the person follows, opening their settings. */
export function FollowChip({ label, onPress }: { label: string; onPress: () => void }) {
  const { t } = useTranslation();
  const chip = useCSSVariable("--color-chip-foreground");
  return (
    <Pressable
      testID="follow-chip"
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityHint={t("follow.chipHint")}
      onPress={onPress}
      className="bg-chip flex-row items-center gap-1 rounded-full px-3 py-1 active:opacity-70"
    >
      <Text numberOfLines={1} className="text-chip-foreground max-w-40 text-sm font-semibold">
        {label}
      </Text>
      <Ionicons name="chevron-down" size={14} color={String(chip)} />
    </Pressable>
  );
}

/** Opens the review setup for a project, then the step the person asked for. */
function openSetup(p: ReviewInboxProject, next?: PlanLevel) {
  router.push({
    pathname: "/[ws]/[project]/languages",
    params: next
      ? { ws: p.workspace, project: p.project, setup: "1", next }
      : { ws: p.workspace, project: p.project, setup: "1" },
  });
}

/**
 * Opens one step of a project's plan; before the setup, the setup first. The
 * coordinates open their map, narrowed to the languages the person follows.
 */
function openStep(p: ReviewInboxProject, level: PlanLevel) {
  setRecentProject({ ws: p.workspace, project: p.project, name: p.projectName });
  if (!p.saved) {
    openSetup(p, level);
    return;
  }
  if (level === "coordinates") {
    router.push({
      pathname: "/[ws]/[project]/coordinates",
      params: { ws: p.workspace, project: p.project, languages: p.followed.join(",") },
    });
    return;
  }
  router.push({
    pathname: "/[ws]/[project]/plan",
    params: { ws: p.workspace, project: p.project, step: level },
  });
}

export default function ReviewInbox() {
  const { t, i18n } = useTranslation();
  const { client } = useAuth();
  const inbox = useQuery({ queryKey: inboxKey, queryFn: () => client.reviewHome() });
  const { refetch } = inbox;

  // Back from a deck, the counts have moved.
  useFocusEffect(
    useCallback(() => {
      void refetch();
    }, [refetch]),
  );

  const languages = inbox.data?.languages ?? [];
  const context = inbox.data?.context ?? [];
  const projects = inbox.data?.projects ?? [];
  const showWorkspace =
    new Set([
      ...languages.map((l) => l.workspace),
      ...context.map((c) => c.workspace),
      ...projects.map((p) => p.workspace),
    ]).size > 1;
  const chipLabel = followedChipLabel(
    projects.map((p) => p.followed),
    (code) => languageLabel(code, i18n.language),
    t,
  );
  const openSettings = () => {
    const only = projects.length === 1 ? projects[0] : undefined;
    if (only) {
      router.push({
        pathname: "/[ws]/[project]/languages",
        params: { ws: only.workspace, project: only.project },
      });
    } else {
      router.push("/review-languages");
    }
  };
  const openLanguage = useCallback((item: ReviewInboxLanguage) => {
    setRecentProject({ ws: item.workspace, project: item.project, name: item.project_name });
    router.push({
      pathname: "/review/[ws]/[project]/[locale]",
      params: { ws: item.workspace, project: item.project, locale: item.locale },
    });
  }, []);
  const openContext = (item: ContextInboxEntry) => {
    setRecentProject({ ws: item.workspace, project: item.project, name: item.project_name });
    router.push({
      pathname: "/[ws]/[project]/context",
      params: { ws: item.workspace, project: item.project },
    });
  };
  const renderItem = useCallback(
    ({ item }: { item: ReviewInboxLanguage }) => (
      <LanguageCard
        testID={`inbox-${item.project}-${item.locale}`}
        locale={item.locale}
        waiting={item}
        where={whereOf(item, showWorkspace)}
        onPress={() => openLanguage(item)}
      />
    ),
    [showWorkspace, openLanguage],
  );

  let empty = null;
  if (inbox.isPending) {
    empty = <SkeletonCards />;
  } else if (inbox.isSuccess && inbox.data === null) {
    empty = (
      <EmptyState icon="server" title={t("inbox.unsupported")} body={t("inbox.unsupportedBody")} />
    );
  } else if (inbox.isSuccess && context.length === 0 && projects.length === 0) {
    empty = (
      <EmptyState icon="checkmark-done" title={t("inbox.emptyTitle")} body={t("inbox.emptyBody")} />
    );
  }

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen
        options={{
          headerRight: inbox.data
            ? () => <FollowChip label={chipLabel} onPress={openSettings} />
            : undefined,
        }}
      />
      <FlatList
        testID="inbox-list"
        contentInsetAdjustmentBehavior="automatic"
        data={languages}
        keyExtractor={(l) => `${l.workspace}/${l.project}/${l.locale}`}
        contentContainerClassName="pt-2 pb-8"
        renderItem={renderItem}
        ListHeaderComponent={
          context.length > 0 || projects.length > 0 ? (
            <View testID="inbox-context">
              {projects.map((p) => (
                <PlanCard
                  key={`${p.workspace}/${p.project}`}
                  project={p}
                  showWorkspace={showWorkspace}
                  onStep={openStep}
                  onSetup={(proj) => openSetup(proj)}
                />
              ))}
              {context.map((c) => (
                <ContextEntryCard
                  key={`${c.workspace}/${c.project}`}
                  testID={`inbox-context-${c.project}`}
                  count={c.waiting}
                  detail={[
                    whereOf(c, showWorkspace),
                    t("context.kinds", {
                      voice: c.by_kind.voice_rule ?? 0,
                      terms: c.by_kind.concept ?? 0,
                      rules: c.by_kind.term_rule ?? 0,
                      notes: c.by_kind.note ?? 0,
                    }),
                  ].join("\n")}
                  onPress={() => openContext(c)}
                />
              ))}
            </View>
          ) : undefined
        }
        refreshing={inbox.isRefetching}
        onRefresh={() => void refetch()}
        ListEmptyComponent={empty ?? <QueryState query={inbox} />}
      />
    </View>
  );
}
