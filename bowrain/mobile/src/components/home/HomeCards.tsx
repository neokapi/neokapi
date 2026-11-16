// The cards of the Home screens: a workspace, a project, a project's context
// waiting for review, and one of its languages. Each card says what waits for
// the person and opens the next screen in one tap.
import type { DeckCounts, Project, Workspace } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import type { TFunction } from "i18next";
import { memo } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { languageLabel } from "../../lib/feed";
import type { ProjectWaiting, Waiting } from "../../lib/home";
import { Monogram } from "./Monogram";
import { CountPill, LocaleBadge } from "./parts";
import { ReviewMix } from "./ReviewMix";

const CARD =
  "bg-card border-border mx-4 mb-3 flex-row items-center gap-3 rounded-3xl border p-4 active:opacity-80";

/** What waits, in a line: "12 translations · 3 context items". */
export function waitingSummary(w: Waiting | undefined, t: TFunction): string {
  const parts: string[] = [];
  if (w?.translations) parts.push(t("home.translations", { count: w.translations }));
  if (w?.context) parts.push(t("home.contextItems", { count: w.context }));
  return parts.length ? parts.join(" · ") : t("home.nothingWaiting");
}

function Chevron() {
  const muted = useCSSVariable("--color-muted-foreground");
  return <Ionicons name="chevron-forward" size={18} color={String(muted)} />;
}

export const WorkspaceCard = memo(function WorkspaceCard({
  workspace,
  waiting,
  onPress,
}: {
  workspace: Workspace;
  waiting?: Waiting;
  onPress: (w: Workspace) => void;
}) {
  const { t } = useTranslation();
  const summary = waitingSummary(waiting, t);
  const count = (waiting?.translations ?? 0) + (waiting?.context ?? 0);
  return (
    <Pressable
      testID={`workspace-${workspace.slug}`}
      accessibilityRole="button"
      accessibilityLabel={`${workspace.name}, ${summary}`}
      onPress={() => onPress(workspace)}
      className={CARD}
    >
      <Monogram name={workspace.name} seed={workspace.id} size={48} />
      <View className="flex-1 gap-0.5">
        <Text className="text-foreground text-[17px] font-semibold" numberOfLines={2}>
          {workspace.name}
        </Text>
        <Text className="text-muted-foreground text-sm" numberOfLines={2}>
          {summary}
        </Text>
      </View>
      <CountPill count={count} />
      <Chevron />
    </Pressable>
  );
});

// The badges a project card shows before it folds the rest into "+N".
const MAX_BADGES = 6;

export const ProjectCard = memo(function ProjectCard({
  project,
  waiting,
  onPress,
}: {
  project: Project;
  waiting?: ProjectWaiting;
  onPress: (p: Project) => void;
}) {
  const { t, i18n } = useTranslation();
  const prism = useCSSVariable("--color-prism-5");
  // The languages with work waiting come first, so the badges that are lit
  // are the ones a reader sees.
  const targets = [...project.target_languages].sort(
    (a, b) => Number(waiting?.byLocale.has(b) ?? false) - Number(waiting?.byLocale.has(a) ?? false),
  );
  const shown = targets.slice(0, MAX_BADGES);
  const hidden = targets.length - shown.length;
  const source = project.default_source_language
    ? languageLabel(project.default_source_language, i18n.language)
    : undefined;
  const meta = [
    source ? t("home.project.from", { language: source }) : undefined,
    t("projects.languages", { count: project.target_languages.length }),
  ]
    .filter(Boolean)
    .join(" · ");
  const summary = waitingSummary(waiting, t);
  return (
    <Pressable
      testID={`project-${project.id}`}
      accessibilityRole="button"
      accessibilityLabel={`${project.name}, ${meta}, ${summary}`}
      onPress={() => onPress(project)}
      className={`${CARD} items-start`}
    >
      <Monogram name={project.name} seed={project.id} size={48} />
      <View className="flex-1 gap-2">
        <View className="gap-0.5">
          <Text className="text-foreground text-[17px] font-semibold" numberOfLines={2}>
            {project.name}
          </Text>
          <Text className="text-muted-foreground text-sm" numberOfLines={1}>
            {meta}
          </Text>
        </View>
        {shown.length > 0 ? (
          <View className="flex-row flex-wrap items-center gap-1.5">
            {shown.map((l) => (
              <LocaleBadge
                key={l}
                locale={l}
                size="sm"
                active={waiting?.byLocale.has(l) ?? false}
              />
            ))}
            {hidden > 0 ? (
              <Text className="text-muted-foreground text-xs font-medium">
                {t("home.project.moreLanguages", { count: hidden })}
              </Text>
            ) : null}
          </View>
        ) : null}
        {waiting?.translations ? (
          <Text className="text-primary text-sm font-semibold">
            {t("home.project.waitingIn", {
              translations: t("home.translations", { count: waiting.translations }),
              count: waiting.languages,
            })}
          </Text>
        ) : null}
        {waiting?.context ? (
          <View className="flex-row items-center gap-1.5">
            <Ionicons name="sparkles" size={14} color={String(prism)} />
            <Text className="text-foreground text-sm font-medium">
              {t("context.entry", { count: waiting.context })}
            </Text>
          </View>
        ) : null}
        {!waiting?.translations && !waiting?.context ? (
          <Text className="text-muted-foreground text-sm">{t("home.nothingWaiting")}</Text>
        ) : null}
      </View>
      <View className="h-12 justify-center">
        <Chevron />
      </View>
    </Pressable>
  );
});

/** A project's context waiting for review: voice rules, terms and notes. */
export function ContextEntryCard({
  count,
  detail,
  onPress,
}: {
  count: number;
  detail?: string;
  onPress: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Pressable
      testID="project-context"
      accessibilityRole="button"
      accessibilityLabel={[t("context.entry", { count }), detail].filter(Boolean).join(", ")}
      onPress={onPress}
      className={CARD}
    >
      <View
        accessibilityElementsHidden
        importantForAccessibility="no-hide-descendants"
        className="from-prism-4 to-prism-5 h-11 w-11 items-center justify-center rounded-xl bg-linear-to-br"
      >
        <Ionicons name="sparkles" size={22} color="#ffffff" />
      </View>
      <View className="flex-1 gap-0.5">
        <Text className="text-foreground text-[17px] font-semibold">
          {t("context.entry", { count })}
        </Text>
        {detail ? (
          <Text className="text-muted-foreground text-sm tabular-nums" numberOfLines={2}>
            {detail}
          </Text>
        ) : null}
      </View>
      <Chevron />
    </Pressable>
  );
}

export const LanguageCard = memo(function LanguageCard({
  locale,
  counts,
  onPress,
}: {
  locale: string;
  /** The language's deck counts; undefined while they load. */
  counts?: Partial<DeckCounts> & { total: number };
  onPress: (locale: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const name = languageLabel(locale, i18n.language);
  const total = counts?.total;
  const status =
    total === undefined
      ? t("home.loading")
      : total > 0
        ? t("home.translations", { count: total })
        : t("home.nothingWaiting");
  return (
    <Pressable
      testID={`language-${locale}`}
      accessibilityRole="button"
      accessibilityLabel={`${name}, ${status}`}
      onPress={() => onPress(locale)}
      className={CARD}
    >
      <LocaleBadge locale={locale} active={(total ?? 0) > 0} />
      <View className="flex-1 gap-1.5">
        <Text className="text-foreground text-[17px] font-semibold" numberOfLines={2}>
          {name}
        </Text>
        {total !== undefined && total > 0 && counts?.fast !== undefined ? (
          <ReviewMix testID={`language-${locale}-mix`} mix={counts} />
        ) : (
          <Text className="text-muted-foreground text-sm">{status}</Text>
        )}
      </View>
      {total === undefined ? (
        <View className="bg-muted h-7 w-9 rounded-full" />
      ) : (
        <CountPill count={total} />
      )}
      <Chevron />
    </Pressable>
  );
});
