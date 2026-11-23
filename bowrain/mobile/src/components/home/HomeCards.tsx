// The cards of the Home screens: a workspace, a project, a project's context
// waiting for review, and one of its languages. Each card says what waits for
// the person and opens the next screen in one tap.
import type { Project, ReviewInboxLanguage, Workspace } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import type { TFunction } from "i18next";
import { memo } from "react";
import { Pressable, Text, useWindowDimensions, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { languageLabel } from "../../lib/feed";
import type { ProjectWaiting, Waiting } from "../../lib/home";
import { Monogram } from "./Monogram";
import { CountPill, LocaleBadge, UnknownPill } from "./parts";
import { ReviewMix } from "./ReviewMix";

const CARD =
  "bg-card border-border mx-4 mb-3 flex-row items-center gap-3 rounded-3xl border p-4 active:opacity-80";

/** What waits, in a line: "12 translations · 3 context items". */
export function waitingSummary(w: Waiting | undefined, t: TFunction, unknown = false): string {
  if (unknown) return t("home.countsUnavailable");
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
  unknown = false,
  onPress,
}: {
  workspace: Workspace;
  waiting?: Waiting;
  /** What waits could not be loaded: the card says so rather than showing none. */
  unknown?: boolean;
  onPress: (w: Workspace) => void;
}) {
  const { t } = useTranslation();
  const summary = waitingSummary(waiting, t, unknown);
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
        <Text className="text-muted-foreground text-sm" numberOfLines={1}>
          {summary}
        </Text>
      </View>
      {unknown ? <UnknownPill /> : <CountPill count={count} />}
      <Chevron />
    </Pressable>
  );
});

// The badges a project card shows before it folds the rest into "+N".
const MAX_BADGES = 6;

export const ProjectCard = memo(function ProjectCard({
  project,
  waiting,
  unknown = false,
  onPress,
}: {
  project: Project;
  waiting?: ProjectWaiting;
  /** What waits could not be loaded: the card says so rather than showing none. */
  unknown?: boolean;
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
  const summary = waitingSummary(waiting, t, unknown);
  const translationsWaiting = t("home.translations", { count: waiting?.translations ?? 0 });
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
                unknown={unknown}
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
        {/* Two lines, whatever waits, so the card keeps its height when the counts arrive. */}
        <View className="gap-0.5">
          {waiting?.translations ? (
            <Text className="text-primary text-sm font-semibold" numberOfLines={1}>
              {t("home.project.waitingIn", {
                translations: translationsWaiting,
                count: waiting.languages,
              })}
            </Text>
          ) : (
            <Text className="text-muted-foreground text-sm" numberOfLines={1}>
              {unknown ? t("home.countsUnavailable") : t("home.nothingWaiting")}
            </Text>
          )}
          {waiting?.context ? (
            <View className="flex-row items-center gap-1.5">
              <Ionicons name="sparkles" size={14} color={String(prism)} />
              <Text className="text-foreground text-sm font-medium" numberOfLines={1}>
                {t("context.entry", { count: waiting.context })}
              </Text>
            </View>
          ) : (
            // An empty line in the same type, so it is as tall at every text size.
            <Text accessibilityElementsHidden className="text-sm">
              {"\u00a0"}
            </Text>
          )}
        </View>
      </View>
      <View className="h-12 justify-center">
        <Chevron />
      </View>
    </Pressable>
  );
});

// CARD with its parts stacked: the row for the largest text sizes.
const CARD_STACKED =
  "bg-card border-border mx-4 mb-3 gap-3 rounded-3xl border p-4 active:opacity-80";

/** The text scale from which a fixed-height row lets its lines wrap. */
const LARGE_TEXT = 1.6;

/** A project's context waiting for review: voice rules, terms and notes. */
export function ContextEntryCard({
  count,
  title,
  detail,
  detailLines = 2,
  trailing,
  onPress,
  testID = "project-context",
}: {
  count: number;
  /** The title in place of the count, for a row that is always present. */
  title?: string;
  detail?: string;
  /** The lines the detail may take; one keeps the card's height fixed. */
  detailLines?: number;
  /** Shown before the chevron, such as the count of what waits. */
  trailing?: React.ReactNode;
  onPress: () => void;
  testID?: string;
}) {
  const { t } = useTranslation();
  const heading = title ?? t("context.entry", { count });
  // At the largest text sizes the row cannot hold the title beside the icon
  // and the count, so the title and the detail take a line of their own
  // under them, at full width and wrapping, rather than cutting a word.
  const large = useWindowDimensions().fontScale >= LARGE_TEXT;
  const icon = (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      className="from-prism-4 to-prism-5 h-11 w-11 items-center justify-center rounded-xl bg-linear-to-br"
    >
      <Ionicons name="sparkles" size={22} color="#ffffff" />
    </View>
  );
  const text = (
    <View className={large ? "gap-0.5" : "flex-1 gap-0.5"}>
      <Text
        className="text-foreground text-[17px] font-semibold"
        numberOfLines={large ? undefined : detailLines}
      >
        {heading}
      </Text>
      {detail ? (
        <Text
          className="text-muted-foreground text-sm tabular-nums"
          numberOfLines={large ? undefined : detailLines}
        >
          {detail}
        </Text>
      ) : null}
    </View>
  );
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityLabel={[heading, detail].filter(Boolean).join(", ")}
      onPress={onPress}
      className={large ? CARD_STACKED : CARD}
    >
      {large ? (
        <>
          <View className="flex-row items-center gap-4">
            {icon}
            <View className="flex-1" />
            {trailing}
            <Chevron />
          </View>
          {text}
        </>
      ) : (
        <>
          {icon}
          {text}
          {trailing}
          <Chevron />
        </>
      )}
    </Pressable>
  );
}

export const LanguageCard = memo(function LanguageCard({
  locale,
  waiting,
  loading,
  unknown = false,
  where,
  testID,
  onPress,
}: {
  locale: string;
  /** The inbox's entry for the language; undefined when nothing waits. */
  waiting?: ReviewInboxLanguage;
  /** The inbox is still loading. */
  loading?: boolean;
  /** The inbox could not be loaded: the card says so rather than showing none. */
  unknown?: boolean;
  /** The project (and workspace) the language belongs to, where the list spans several. */
  where?: string;
  testID?: string;
  onPress: (locale: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const name = languageLabel(locale, i18n.language);
  const count = waiting?.waiting ?? 0;
  const status = loading
    ? t("home.loading")
    : unknown
      ? t("home.countsUnavailable")
      : count > 0
        ? t("home.translations", { count })
        : t("home.nothingWaiting");
  // At the largest text sizes the name, the bar and the status take the
  // card's full width under the badge and the count, and wrap.
  const large = useWindowDimensions().fontScale >= LARGE_TEXT;
  const badge = <LocaleBadge locale={locale} active={count > 0} />;
  const pill = loading ? (
    <View className="bg-muted h-7 w-9 rounded-full" />
  ) : unknown ? (
    <UnknownPill />
  ) : (
    <CountPill count={count} />
  );
  const text = (
    <View className={large ? "gap-1.5" : "flex-1 gap-1.5"}>
      <Text
        className="text-foreground text-[17px] font-semibold"
        numberOfLines={large ? undefined : 2}
      >
        {name}
      </Text>
      {where ? (
        <Text className="text-muted-foreground text-sm" numberOfLines={large ? undefined : 1}>
          {where}
        </Text>
      ) : null}
      {waiting && count > 0 ? (
        <ReviewMix testID={`language-${locale}-mix`} mix={waiting} />
      ) : (
        // The same shape as the review mix, an empty bar over one line, so
        // the card keeps its height when the counts arrive.
        <View className="gap-1.5">
          <View className="bg-muted h-2 rounded-full" />
          <Text className="text-muted-foreground text-xs">{status}</Text>
        </View>
      )}
    </View>
  );
  return (
    <Pressable
      testID={testID ?? `language-${locale}`}
      accessibilityRole="button"
      accessibilityLabel={[name, where, status].filter(Boolean).join(", ")}
      onPress={() => onPress(locale)}
      className={large ? CARD_STACKED : CARD}
    >
      {large ? (
        <>
          <View className="flex-row items-center gap-4">
            {badge}
            <View className="flex-1" />
            {pill}
            <Chevron />
          </View>
          {text}
        </>
      ) : (
        <>
          {badge}
          {text}
          {pill}
          <Chevron />
        </>
      )}
    </Pressable>
  );
});
