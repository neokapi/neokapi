// A followed project's review plan on the Review tab: the step the team is
// on, one button that opens it, the later steps as a short list, and one line
// per language whose translations wait for the key language. The server
// decides every state and count (the inbox's plan in brief); this card shows
// them.
import {
  nextPlanStep,
  type PlanLevel,
  type PlanStep,
  type ReviewInboxProject,
} from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import type { TFunction } from "i18next";
import { memo } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { languageLabel } from "../../lib/feed";
import { Monogram } from "./Monogram";

/** The step's one fact for the Next card. */
export function stepFact(step: PlanStep, t: TFunction): string {
  switch (step.state) {
    case "done":
      return t("planCard.fact.done");
    case "attention":
      return t("planCard.fact.changed", { count: step.changed });
    case "not_started":
      if (step.level === "key_language") {
        if (step.scan === "queued" || step.scan === "processing") {
          return t("planCard.fact.keyScanning");
        }
        return step.scan === "failed"
          ? t("planCard.fact.keyScanFailed")
          : t("planCard.fact.keyNotStarted");
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
    default:
      return t(`planCard.fact.${step.level}`, { count: step.count });
  }
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
            {stepFact(next, t)}
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
