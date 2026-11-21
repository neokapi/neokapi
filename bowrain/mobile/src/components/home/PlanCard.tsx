// A followed project's review plan on the Review tab: the step the team is
// on, one button that opens it, the steps that only wait and why, the later
// steps as a short list, and one line per language whose translations wait
// for the context. The server decides every state and count and which step
// is next (the inbox's plan in brief); this card shows them.
import {
  nextPlanStep,
  type PlanLevel,
  type PlanStep,
  type PlanWait,
  type ReviewInboxProject,
} from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import type { TFunction } from "i18next";
import { memo } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { languageLabel, listOf } from "../../lib/feed";
import { Monogram } from "./Monogram";

/**
 * What the context's term scan is doing while the level has not started.
 * The server starts the scan when content arrives; this names the scan's state
 * or what it waits for.
 */
export function contextScanFact(
  scan: PlanStep["scan"],
  t: TFunction,
  language: string,
  opts: { progress?: number; quotaResetsAt?: string } = {},
): string {
  switch (scan) {
    case "queued":
    case "processing":
      return opts.progress
        ? t("plan.context.readingProgress", { progress: opts.progress })
        : t("plan.context.reading");
    case "failed":
      return t("plan.context.scanFailed");
    case "waiting_quota":
      return opts.quotaResetsAt
        ? t("plan.context.waitsForQuota", { date: resetDate(opts.quotaResetsAt, language) })
        : t("plan.context.waitsForQuotaSoon");
    case "waiting_credits":
      return t("plan.context.waitsForCredits");
    case "no_ai":
      return t("plan.context.waitsForProvider");
    case "waiting_push":
      return t("plan.context.waitsForPush");
    default:
      return t("plan.context.waitsForContent");
  }
}

/**
 * What the job proposing target terms is doing, or what it waits for, or
 * null when no job runs and nothing holds one back. The server starts the job
 * when a term scan completes, a target language is added or a push lands.
 */
export function contextProposalsFact(
  proposals: PlanStep["proposals"],
  t: TFunction,
  language: string,
  quotaResetsAt?: string,
): string | null {
  switch (proposals) {
    case "queued":
    case "processing":
      return t("plan.context.proposing");
    case "waiting_quota":
      return quotaResetsAt
        ? t("plan.context.proposalsWaitForQuota", { date: resetDate(quotaResetsAt, language) })
        : t("plan.context.proposalsWaitForQuotaSoon");
    case "waiting_credits":
      return t("plan.context.proposalsWaitForCredits");
    case "no_ai":
      return t("plan.context.proposalsWaitForProvider");
    default:
      return null;
  }
}

/** The AI limit's reset day in the reader's language. The limit resets at midnight UTC. */
function resetDate(iso: string, language: string): string {
  const when = new Date(iso);
  if (Number.isNaN(when.getTime())) return iso;
  return new Intl.DateTimeFormat(language, { dateStyle: "long", timeZone: "UTC" }).format(when);
}

/** The step's one fact for the Next card. */
export function stepFact(step: PlanStep, t: TFunction, language = "en"): string {
  switch (step.state) {
    case "done":
      return t("planCard.fact.done");
    case "attention":
      return t("planCard.fact.changed", { count: step.changed });
    case "not_started":
      if (step.level === "context") {
        return contextScanFact(step.scan, t, language, { quotaResetsAt: step.quotaResetsAt });
      }
      return t("planCard.fact.notStarted");
  }
  switch (step.level) {
    case "coordinates":
      return t("planCard.fact.coordinatesOpen");
    case "policy":
      return step.settle === "held"
        ? t("planCard.fact.policyWaits")
        : t("planCard.fact.policy", { count: step.count });
    case "context":
      // The Next card is the person's step: their own count, which an
      // older cached step does not carry.
      return t("planCard.fact.context", { count: step.mine ?? step.count });
    default:
      return t(`planCard.fact.${step.level}`, { count: step.count });
  }
}

/** What one step that only waits is waiting for. */
export function waitFact(w: PlanWait, t: TFunction, language: string): string {
  if (w.reason === "other_languages") {
    const names = (w.locales ?? []).map((l) => languageLabel(l, language));
    return names.length > 0
      ? t("plan.context.otherLanguages", { languages: listOf(names, t, language) })
      : t("plan.context.otherLanguagesUnnamed");
  }
  return contextScanFact(w.reason, t, language, { quotaResetsAt: w.quotaResetsAt });
}

/** The step's one count or state word for the list of later steps. */
export function stepValue(step: PlanStep, t: TFunction): string {
  switch (step.state) {
    case "done":
      return t("planCard.state.done");
    case "not_started":
      return t("planCard.state.not_started");
    case "attention":
      return t("planCard.state.attention", { count: step.changed });
  }
  if (step.level === "coordinates") return t("planCard.state.toConfirm");
  if (step.level === "policy" && step.settle === "held") return t("planCard.state.waits");
  return t("planCard.count", { count: step.count });
}

/**
 * The steps the server skipped for the next one because they only wait: one
 * row per step, its name and what it waits for. Home and the Review tab show
 * it beside the next step.
 */
export function PlanWaitingNote({ waiting, testID }: { waiting: PlanWait[]; testID: string }) {
  const { t, i18n } = useTranslation();
  if (!waiting?.length) return null;
  return (
    <View testID={testID} className="gap-1">
      {waiting.map((w) => (
        <View
          key={w.level}
          testID={`${testID}-${w.level}`}
          accessible
          className="flex-row items-start gap-2"
        >
          <Text className="text-muted-foreground text-sm font-semibold">
            {t(`planCard.steps.${w.level}`)}
          </Text>
          <Text className="text-muted-foreground flex-1 text-sm">
            {waitFact(w, t, i18n.language)}
          </Text>
        </View>
      ))}
    </View>
  );
}

/** One later step: its name and one count or state word. */
function StepRow({
  step,
  onPress,
  testID,
}: {
  step: PlanStep;
  onPress: () => void;
  testID: string;
}) {
  const { t } = useTranslation();
  const name = t(`planCard.steps.${step.level}`);
  const value = stepValue(step, t);
  const muted = step.state === "done" || step.state === "not_started";
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityLabel={t("planCard.rowLabel", { step: name, value })}
      onPress={onPress}
      className="flex-row items-center py-1.5 active:opacity-70"
    >
      <Text className="text-foreground flex-1 text-[15px]">{name}</Text>
      <Text
        className={`text-[15px] tabular-nums ${
          step.state === "attention"
            ? "text-warning font-semibold"
            : muted
              ? "text-muted-foreground"
              : "text-foreground font-semibold"
        }`}
      >
        {value}
      </Text>
    </Pressable>
  );
}

/**
 * A followed project's plan: its name and the step it is on, the Next card,
 * the later steps, the held translations, and, before the person has chosen
 * their languages, the button that opens the setup.
 */
export const PlanCard = memo(function PlanCard({
  project,
  showWorkspace,
  onStep,
  onSetup,
}: {
  project: ReviewInboxProject;
  showWorkspace: boolean;
  onStep: (p: ReviewInboxProject, level: PlanLevel) => void;
  onSetup: (p: ReviewInboxProject) => void;
}) {
  const { t, i18n } = useTranslation();
  const [primaryForeground] = useCSSVariable(["--color-primary-foreground"]);
  const plan = project.plan;
  const next = plan ? nextPlanStep(plan.steps, plan.next) : undefined;
  const index = next && plan ? plan.steps.indexOf(next) : -1;
  const later = plan ? (next ? plan.steps.slice(index + 1) : plan.steps) : [];
  return (
    <View
      testID={`plan-card-${project.project}`}
      className="bg-card border-border mx-4 mb-3 gap-3 rounded-3xl border p-4"
    >
      <View className="flex-row items-center gap-3">
        <Monogram name={project.projectName} seed={project.project} size={32} />
        <View className="flex-1">
          <Text numberOfLines={1} className="text-foreground text-[17px] font-semibold">
            {project.projectName}
          </Text>
          {showWorkspace ? (
            <Text numberOfLines={1} className="text-muted-foreground text-xs">
              {project.workspaceName}
            </Text>
          ) : null}
        </View>
        {plan ? (
          <Text
            testID={`plan-card-${project.project}-position`}
            className="text-muted-foreground text-xs"
          >
            {next
              ? t("planCard.stepOf", { n: index + 1, total: plan.steps.length })
              : (plan.waiting?.length ?? 0) > 0
                ? t("planCard.state.waits")
                : t("planCard.allDone")}
          </Text>
        ) : null}
      </View>

      {!plan ? <Text className="text-muted-foreground text-sm">{t("planCard.noPlan")}</Text> : null}

      {next ? (
        <View className="bg-primary/10 border-primary gap-1 rounded-2xl border p-3.5">
          <Text className="text-primary text-xs font-semibold uppercase">{t("planCard.next")}</Text>
          <Text className="text-foreground text-[17px] font-semibold">
            {t(`planCard.titles.${next.level}`)}
          </Text>
          <Text
            testID={`plan-card-${project.project}-fact`}
            className="text-muted-foreground text-sm"
          >
            {stepFact(next, t, i18n.language)}
          </Text>
          <Pressable
            testID={`plan-card-${project.project}-next`}
            accessibilityRole="button"
            onPress={() => onStep(project, next.level)}
            className="bg-primary mt-2 flex-row items-center justify-center gap-1 rounded-xl py-2.5 active:opacity-80"
          >
            <Text className="text-primary-foreground text-sm font-semibold">
              {next.level === "coordinates" ? t("planCard.openMap") : t("planCard.open")}
            </Text>
            <Ionicons name="chevron-forward" size={14} color={String(primaryForeground)} />
          </Pressable>
        </View>
      ) : null}

      {plan ? (
        <PlanWaitingNote
          testID={`plan-card-${project.project}-waiting`}
          waiting={plan.waiting ?? []}
        />
      ) : null}

      {later.length > 0 ? (
        <View>
          {later.map((s) => (
            <StepRow
              key={s.level}
              testID={`plan-card-${project.project}-${s.level}`}
              step={s}
              onPress={() => onStep(project, s.level)}
            />
          ))}
        </View>
      ) : null}

      {plan?.held.map((h) => (
        <Text
          key={h.locale}
          testID={`plan-card-${project.project}-held-${h.locale}`}
          className="text-muted-foreground text-sm"
        >
          {t("planCard.held", { count: h.count, language: languageLabel(h.locale, i18n.language) })}
        </Text>
      ))}

      {!project.saved ? (
        <Pressable
          testID={`plan-card-${project.project}-setup`}
          accessibilityRole="button"
          onPress={() => onSetup(project)}
          className="bg-primary items-center rounded-xl py-2.5 active:opacity-80"
        >
          <Text className="text-primary-foreground text-sm font-semibold">
            {t("planCard.setup")}
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
});
