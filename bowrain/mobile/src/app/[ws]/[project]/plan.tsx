// A project's review plan: five steps, one row each, ordered by the reach of
// each decision. Each row shows its state, its title and one fact; only the
// next step carries an action. The server decides every state and count
// (GET /:ws/:id/review/plan), including which translations the language decks
// hold until the context is decided. The involvement setting lives on
// the project's review languages screen. The policy step shows how the
// server settles after each run and its last pass; the phone has no settle
// button.
import {
  PLAN_LEVELS,
  planRuleLabel,
  type PlanLevel,
  type PlanRule,
  type PlanStep,
  type PlanStepState,
  type ReviewPlan,
} from "@neokapi/bowrain-client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { router, Stack, useLocalSearchParams } from "expo-router";
import { useRef, type ComponentRef, type ReactNode } from "react";
import { Pressable, RefreshControl, ScrollView, Text, View, type ViewProps } from "react-native";
import { useTranslation } from "react-i18next";

import { contextProposalsFact, contextScanFact, waitFact } from "../../../components/home/PlanCard";
import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { reviewPlanKey } from "../../../lib/plan";
import { ageLabel, languageLabel } from "../../../lib/feed";

/**
 * The plan's step for a level. A server that sends no steps reads as the plan
 * named them: the next level open, the others done.
 */
function stepOf(p: ReviewPlan, level: PlanLevel): PlanStep {
  return (
    p.steps.find((s) => s.level === level) ?? {
      level,
      state: p.next === level ? "open" : "done",
      count: 0,
      progress: 0,
      done: p.next !== level,
      changed: 0,
    }
  );
}

/** The step's number, or a mark for its state. */
function Marker({ n, state, next }: { n: number; state: PlanStepState; next: boolean }) {
  const look = next
    ? "bg-primary"
    : state === "done"
      ? "bg-approve"
      : state === "attention"
        ? "bg-warning"
        : "bg-muted";
  const ink = next || state === "done" ? "text-primary-foreground" : "text-muted-foreground";
  return (
    <View className={`h-7 w-7 items-center justify-center rounded-full ${look}`}>
      <Text
        className={`text-sm font-semibold ${state === "attention" ? "text-warning-foreground" : ink}`}
      >
        {state === "done" && !next ? "✓" : state === "attention" ? "!" : n}
      </Text>
    </View>
  );
}

/** One step: its marker, title and one fact, and, on the next step, its action. */
function Row({
  n,
  level,
  state,
  next,
  title,
  fact,
  note,
  onLayout,
  children,
}: {
  n: number;
  level: PlanLevel;
  state: PlanStepState;
  next: boolean;
  title: string;
  fact: string;
  /** Shown on the step whether or not it is next. */
  note?: ReactNode;
  onLayout?: ViewProps["onLayout"];
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <View
      onLayout={onLayout}
      testID={`plan-${level}`}
      className={`mx-4 mb-2 gap-2 rounded-2xl border px-4 py-3 ${
        next ? "border-primary bg-primary/10" : "border-border bg-card"
      }`}
    >
      <View className="flex-row items-center gap-3">
        <Marker n={n} state={state} next={next} />
        <View className="flex-1">
          <Text className="text-foreground text-base font-semibold">{title}</Text>
          <Text testID={`plan-${level}-fact`} className="text-muted-foreground text-sm">
            {fact}
          </Text>
        </View>
        {next ? (
          <Text
            testID={`plan-${level}-next`}
            className="text-primary text-xs font-semibold uppercase"
          >
            {t("plan.next")}
          </Text>
        ) : null}
      </View>
      {note}
      {next ? children : null}
    </View>
  );
}

/** The next step's button. */
function Action({
  testID,
  label,
  onPress,
}: {
  testID: string;
  label: string;
  onPress: () => void;
}) {
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      onPress={onPress}
      className="bg-primary items-center rounded-xl py-2.5 active:opacity-80"
    >
      <Text className="text-primary-foreground text-sm font-semibold">{label}</Text>
    </Pressable>
  );
}

/**
 * The rules behind the most findings in one place: the source, or a target
 * language. A rule with onOpen opens the deck narrowed to it.
 */
function RuleList({
  testID,
  title,
  rules,
  onOpen,
}: {
  testID: string;
  title: string;
  rules: PlanRule[];
  onOpen?: (r: PlanRule) => void;
}) {
  const { t } = useTranslation();
  if (rules.length === 0) return null;
  return (
    <View testID={testID} className="gap-1">
      <Text className="text-muted-foreground text-xs font-semibold uppercase">{title}</Text>
      {rules.map((r) => (
        <Pressable
          key={r.key}
          testID={`${testID}-${r.key}`}
          accessibilityRole={onOpen ? "button" : undefined}
          disabled={!onOpen}
          onPress={onOpen ? () => onOpen(r) : undefined}
          className="flex-row items-baseline gap-2"
        >
          <Text
            numberOfLines={2}
            className={`flex-1 text-sm leading-5 ${onOpen ? "text-primary" : "text-foreground"}`}
          >
            {planRuleLabel(r)}
          </Text>
          <Text className="text-muted-foreground text-sm">
            {t("plan.exceptions.ruleCount", { count: r.count })}
          </Text>
        </Pressable>
      ))}
    </View>
  );
}

export default function ReviewPlanScreen() {
  const { t, i18n } = useTranslation();
  const { ws, project, step } = useLocalSearchParams<{
    ws: string;
    project: string;
    step?: string;
  }>();
  const scroll = useRef<ComponentRef<typeof ScrollView>>(null);
  const placed = useRef<string | undefined>(undefined);
  const { client } = useAuth();
  const qc = useQueryClient();
  const scope = { workspace: ws, project };
  const plan = useQuery({
    queryKey: reviewPlanKey(ws, project),
    queryFn: () => client.reviewPlan(scope),
  });
  const refresh = () => void qc.invalidateQueries({ queryKey: reviewPlanKey(ws, project) });

  if (!plan.data) {
    return (
      <View className="bg-background flex-1">
        <Stack.Screen options={{ title: t("plan.title") }} />
        <QueryState query={plan} />
      </View>
    );
  }
  const p = plan.data;
  const lang = (l: string) => languageLabel(l, i18n.language);
  const failing = p.languages.reduce((n, l) => n + l.failing, 0);
  const pages = p.languages.reduce((n, l) => n + l.pages, 0);
  const pagesTotal = p.languages.reduce((n, l) => n + l.pagesTotal, 0);
  const settleable = p.languages.reduce((n, l) => n + l.settleable, 0);
  const openDeck = (locale: string | undefined, sample = false) =>
    locale
      ? router.push({
          pathname: "/[ws]/[project]/[locale]",
          params: sample ? { ws, project, locale, sample: "true" } : { ws, project, locale },
        })
      : undefined;
  const openRule = (locale: string, r: PlanRule) =>
    router.push({
      pathname: "/[ws]/[project]/[locale]",
      params: { ws, project, locale, rule: r.key, ruleName: planRuleLabel(r) },
    });
  const settleNow = p.policy.settle === "first_pass" || p.policy.settle === "policy";

  const fact = (s: PlanStep): string => {
    switch (s.level) {
      case "coordinates":
        if (s.state === "attention") {
          return t("plan.coordinates.changed", { count: p.coordinates.changed });
        }
        if (!p.coordinates.confirmed) return t("plan.coordinates.unconfirmed");
        return p.coordinates.confirmedByName
          ? t("plan.coordinates.confirmedBy", { name: p.coordinates.confirmedByName })
          : t("plan.coordinates.confirmed");
      case "context":
        if (s.state === "not_started") {
          return contextScanFact(s.scan, t, i18n.language, {
            progress: p.context.scan?.progress,
            quotaResetsAt: s.quotaResetsAt ?? p.context.quotaResetsAt,
          });
        }
        if (s.state === "open") {
          // Open for the team with nothing for this person: the terms are
          // missing in languages other reviewers decide.
          const others = (p.waiting ?? []).find((w) => w.reason === "other_languages");
          if (others && s.mine === 0) return waitFact(others, t, i18n.language);
          return t("plan.context.open_count", { count: s.count });
        }
        return t("plan.context.concepts", { count: p.context.concepts });
      case "exceptions":
        if (s.state === "not_started") return t("plan.exceptions.notStarted");
        if (failing > 0) return t("plan.exceptions.translations", { count: failing });
        if ((p.exceptions.sourceFindings ?? 0) > 0) {
          return t("plan.exceptions.source", { count: p.exceptions.sourceFindings ?? 0 });
        }
        return t("plan.exceptions.none");
      case "interfaces":
        if (pages === 0) return t("plan.interfaces.none");
        return p.involvement === "context"
          ? t("plan.interfaces.sampleShort", { count: pages, total: pagesTotal })
          : t("plan.interfaces.pages", { count: pages });
      case "policy":
        if (p.policy.settle === "none") return t("plan.policy.none");
        if (p.policy.settle === "held") return t("plan.policy.waits");
        return settleable > 0
          ? t("plan.policy.settleable", { count: settleable })
          : t("plan.policy.nothing");
    }
  };

  // The job proposing target terms: running, or what it waits for.
  const proposalsNote = (s: PlanStep): ReactNode => {
    const line = contextProposalsFact(
      s.proposals,
      t,
      i18n.language,
      s.quotaResetsAt ?? p.context.quotaResetsAt,
    );
    if (!line) return null;
    return (
      <View testID="plan-context-proposals" className="pl-10">
        <Text className="text-muted-foreground text-sm">{line}</Text>
      </View>
    );
  };

  // How the server settles after each run, and the last pass. Under every
  // nothing settles, so the step says nothing more.
  const policyNote = (): ReactNode => {
    if (p.policy.settle === "none") return null;
    const auto = p.policy.auto;
    const last = p.policy.last;
    const name = auto?.byName ?? auto?.by ?? "";
    let autoLine: string | null = null;
    if (auto?.skip === "no_one") autoLine = t("plan.policy.autoNoOne");
    else if (auto?.skip === "no_permission") autoLine = t("plan.policy.autoNoPermission", { name });
    else if (auto && settleNow) autoLine = t("plan.policy.auto", { name });
    if (!autoLine && !last) return null;
    return (
      <View testID="plan-policy-state" className="gap-0.5 pl-10">
        {autoLine ? (
          <Text testID="plan-policy-auto" className="text-muted-foreground text-sm">
            {autoLine}
          </Text>
        ) : null}
        {last ? (
          <Text testID="plan-policy-last" className="text-muted-foreground text-sm">
            {last.automatic
              ? t("plan.policy.lastAuto", {
                  count: last.approved,
                  when: ageLabel(last.at, t, i18n.language),
                })
              : t("plan.policy.lastBy", {
                  count: last.approved,
                  when: ageLabel(last.at, t, i18n.language),
                  name: last.byName ?? last.by ?? "",
                })}
          </Text>
        ) : null}
      </View>
    );
  };

  const action = (level: PlanLevel): ReactNode => {
    switch (level) {
      case "coordinates":
        return (
          <Action
            testID="plan-coordinates-action"
            label={t("plan.coordinates.open")}
            onPress={() =>
              router.push({
                pathname: "/[ws]/[project]/coordinates",
                params: { ws, project, languages: p.languages.map((l) => l.locale).join(",") },
              })
            }
          />
        );
      case "context":
        if (p.context.waiting > 0) {
          return (
            <Action
              testID="plan-context-action"
              label={t("plan.context.open")}
              onPress={() =>
                router.push({ pathname: "/[ws]/[project]/context", params: { ws, project } })
              }
            />
          );
        }
        return null;
      case "exceptions":
        return (
          <>
            <RuleList
              testID="plan-rules-source"
              title={t("plan.exceptions.inSource")}
              rules={p.exceptions.byRule.source}
            />
            {p.languages.map((l) => (
              <RuleList
                key={l.locale}
                testID={`plan-rules-${l.locale}`}
                title={t("plan.exceptions.inLanguage", { language: lang(l.locale) })}
                rules={p.exceptions.byRule.targets[l.locale] ?? []}
                onOpen={(r) => openRule(l.locale, r)}
              />
            ))}
            {failing > 0 ? (
              <Action
                testID="plan-exceptions-action"
                label={t("plan.exceptions.open")}
                onPress={() => openDeck(p.languages.find((l) => l.failing > 0)?.locale)}
              />
            ) : null}
          </>
        );
      case "interfaces":
        return pages > 0 ? (
          <Action
            testID="plan-interfaces-action"
            label={t("plan.interfaces.open")}
            onPress={() =>
              openDeck(
                p.languages.find((l) => l.pages > 0)?.locale ?? p.languages[0]?.locale,
                p.involvement === "context",
              )
            }
          />
        ) : null;
      case "policy":
        return null;
    }
  };

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen options={{ title: t("plan.title") }} />
      <ScrollView
        ref={scroll}
        testID="plan-list"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pt-3 pb-10"
        refreshControl={<RefreshControl refreshing={plan.isRefetching} onRefresh={refresh} />}
      >
        {PLAN_LEVELS.map((level, i) => {
          const s = stepOf(p, level);
          return (
            <Row
              key={level === step ? `selected-${level}` : level}
              onLayout={
                level === step
                  ? (event) => {
                      if (placed.current === step) return;
                      scroll.current?.scrollTo({ y: event.nativeEvent.layout.y, animated: false });
                      placed.current = step;
                    }
                  : undefined
              }
              n={i + 1}
              level={level}
              state={s.state}
              next={p.next === level}
              title={t(`planCard.steps.${level}`)}
              fact={fact(s)}
              note={
                level === "policy"
                  ? policyNote()
                  : level === "context"
                    ? proposalsNote(s)
                    : undefined
              }
            >
              {action(level)}
            </Row>
          );
        })}
        {p.held
          ? p.languages
              .filter((l) => l.held > 0)
              .map((l) => (
                <Text
                  key={l.locale}
                  testID={`plan-held-${l.locale}`}
                  className="text-muted-foreground px-5 pt-2 text-sm"
                >
                  {t("plan.held", { count: l.held, language: lang(l.locale) })}
                </Text>
              ))
          : null}
      </ScrollView>
    </View>
  );
}
