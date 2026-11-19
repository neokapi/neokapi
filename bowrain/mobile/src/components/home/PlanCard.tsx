// A project's review plan as five rings: coordinates, key language,
// exceptions, pages and policy. Each ring fills with the team's progress
// through its step and carries what waits there. The server counts every
// step (the inbox's plan in brief); a ring opens its step.
import type { PlanLevel, PlanStep, ReviewInboxProject } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { memo, type ReactNode } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { formatCount } from "../../lib/format";
import { Monogram } from "./Monogram";

const RING = 40;
const STROKE = 4;

/**
 * A progress ring drawn with two clipped half circles, each a circle whose
 * left and bottom borders are coloured, turned to show its share of the arc.
 */
export function Ring({
  progress,
  color,
  track,
  children,
}: {
  progress: number;
  color: string;
  track: string;
  children?: ReactNode;
}) {
  const p = Math.max(0, Math.min(100, progress));
  const half = RING / 2;
  const arc = {
    position: "absolute" as const,
    width: RING,
    height: RING,
    borderRadius: half,
    borderWidth: STROKE,
    borderColor: "transparent",
    borderLeftColor: color,
    borderBottomColor: color,
  };
  const right = Math.min(p, 50) * 3.6;
  const left = Math.max(p - 50, 0) * 3.6;
  return (
    <View style={{ width: RING, height: RING }}>
      <View
        style={{
          position: "absolute",
          width: RING,
          height: RING,
          borderRadius: half,
          borderWidth: STROKE,
          borderColor: track,
        }}
      />
      {p > 0 ? (
        <View
          style={{
            position: "absolute",
            left: half,
            width: half,
            height: RING,
            overflow: "hidden",
          }}
        >
          <View style={{ ...arc, left: -half, transform: [{ rotate: `${45 + right}deg` }] }} />
        </View>
      ) : null}
      {p > 50 ? (
        <View
          style={{ position: "absolute", left: 0, width: half, height: RING, overflow: "hidden" }}
        >
          <View style={{ ...arc, left: 0, transform: [{ rotate: `${left - 135}deg` }] }} />
        </View>
      ) : null}
      <View className="absolute inset-0 items-center justify-center">{children}</View>
    </View>
  );
}

/** One step: its ring, what waits inside it, and its name under it. */
function StepRing({
  step,
  next,
  onPress,
  testID,
}: {
  step: PlanStep;
  next: boolean;
  onPress: () => void;
  testID: string;
}) {
  const { t, i18n } = useTranslation();
  const [primary, approve, border, muted] = useCSSVariable([
    "--color-primary",
    "--color-approve",
    "--color-border",
    "--color-muted-foreground",
  ]);
  const name = t(`planCard.steps.${step.level}`);
  const held = step.level === "policy" && step.settle === "held";
  const label = step.done
    ? t("planCard.ringDone", { step: name })
    : held
      ? t("planCard.ringHeld", { step: name })
      : t("planCard.ringWaiting", { step: name, count: step.count, progress: step.progress });
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityLabel={label}
      onPress={onPress}
      className="flex-1 items-center gap-1.5 active:opacity-70"
    >
      <Ring
        progress={step.done ? 100 : step.progress}
        color={String(step.done ? approve : primary)}
        track={String(border)}
      >
        {step.done ? (
          <Ionicons name="checkmark" size={18} color={String(approve)} />
        ) : held ? (
          <Ionicons name="pause" size={16} color={String(muted)} />
        ) : (
          <Text className="text-foreground text-xs font-semibold tabular-nums">
            {formatCount(step.count, i18n.language)}
          </Text>
        )}
      </Ring>
      <Text
        numberOfLines={2}
        className={`text-center text-[11px] leading-[14px] ${
          next ? "text-foreground font-semibold" : "text-muted-foreground"
        }`}
      >
        {name}
      </Text>
      {step.changed > 0 ? (
        <View
          testID={`${testID}-changed`}
          className="bg-warning absolute top-0 right-1 min-w-4 items-center rounded-full px-1"
        >
          <Text className="text-warning-foreground text-[10px] font-bold">{step.changed}</Text>
        </View>
      ) : null}
    </Pressable>
  );
}

/**
 * A followed project's plan: its name, the five rings, and, before the person
 * has chosen their languages, the button that opens the setup.
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
  const { t } = useTranslation();
  const plan = project.plan;
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
        <Text className="text-muted-foreground text-xs font-semibold uppercase">
          {t("planCard.title")}
        </Text>
      </View>
      {plan ? (
        <View className="flex-row">
          {plan.steps.map((s) => (
            <StepRing
              key={s.level}
              testID={`plan-card-${project.project}-${s.level}`}
              step={s}
              next={plan.next === s.level}
              onPress={() => onStep(project, s.level)}
            />
          ))}
        </View>
      ) : (
        <Text className="text-muted-foreground text-sm">{t("planCard.noPlan")}</Text>
      )}
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
