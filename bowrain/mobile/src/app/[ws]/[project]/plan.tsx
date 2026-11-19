// A project's review plan: five steps, one row each, ordered by the reach of
// each decision. Each row shows its state, its title and one fact; only the
// next step carries an action. The server decides every state and count
// (GET /:ws/:id/review/plan), including which translations the language decks
// hold until the key language is decided. The involvement setting lives on
// the project's review languages screen.
import {
  ApiError,
  planRuleLabel,
  type PlanLevel,
  type PlanRule,
  type PlanStep,
  type PlanStepState,
  type ReviewPlan,
} from "@neokapi/bowrain-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { router, Stack, useLocalSearchParams } from "expo-router";
import { useState, type ReactNode } from "react";
import { Alert, Pressable, RefreshControl, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { reviewPlanKey } from "../../../lib/plan";
import { languageLabel } from "../../../lib/feed";

const LEVELS: readonly PlanLevel[] = [
  "coordinates",
  "key_language",
  "exceptions",
  "interfaces",
  "policy",
];

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
  children,
}: {
  n: number;
  level: PlanLevel;
  state: PlanStepState;
  next: boolean;
  title: string;
  fact: string;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <View
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
  const { ws, project } = useLocalSearchParams<{ ws: string; project: string; step?: string }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const scope = { workspace: ws, project };
  const plan = useQuery({
    queryKey: reviewPlanKey(ws, project),
    queryFn: () => client.reviewPlan(scope),
  });
  const perms = useQuery({
    queryKey: ["permissions", ws, project],
    queryFn: () => client.permissions(ws, project),
  });
  const mayScan = perms.data?.permissions.includes("manage_voice") ?? false;
  const refresh = () => void qc.invalidateQueries({ queryKey: reviewPlanKey(ws, project) });

  // A scan the server refused for want of AI credits, shown in the step.
  const [noCredits, setNoCredits] = useState(false);
  const settle = useMutation({
    mutationFn: (locales: string[]) => client.settlePlan(scope, locales),
    onSuccess: (r) => {
      Alert.alert(
        t("plan.policy.doneTitle"),
        r.firstPass
          ? t("plan.policy.doneFirstPass", { count: r.approved })
          : t("plan.policy.done", { count: r.approved }),
      );
      refresh();
    },
    onError: (e: Error) => Alert.alert(t("plan.failed"), e.message),
  });
  const scan = useMutation({
    mutationFn: () => client.startTermScan(scope),
    onSuccess: () => {
      setNoCredits(false);
      refresh();
    },
    onError: (e: Error) => {
      if (e instanceof ApiError && e.status === 402) setNoCredits(true);
      else Alert.alert(t("plan.failed"), e.message);
    },
  });

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
  const scanState = p.keyLanguage.scan?.status;
  const scanning = scanState === "queued" || scanState === "processing";
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
      case "key_language":
        if (s.state === "not_started") {
          if (scanning)
            return t("plan.key.scanning", { progress: p.keyLanguage.scan?.progress ?? 0 });
          if (noCredits) return t("plan.key.noCredits");
          if (scanState === "failed") return t("plan.key.scanFailed");
          return t("plan.key.notStarted");
        }
        if (s.state === "open") return t("plan.key.open_count", { count: s.count });
        return t("plan.key.concepts", { count: p.keyLanguage.concepts });
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
      case "key_language":
        if (p.keyLanguage.waiting > 0) {
          return (
            <Action
              testID="plan-key_language-action"
              label={t("plan.key.open")}
              onPress={() =>
                router.push({ pathname: "/[ws]/[project]/context", params: { ws, project } })
              }
            />
          );
        }
        if (stepOf(p, "key_language").state === "not_started" && mayScan && !scanning) {
          return (
            <Action
              testID="plan-scan"
              label={t("plan.key.scan")}
              onPress={() =>
                Alert.alert(t("plan.key.scan"), t("plan.key.scanBody"), [
                  { text: t("common.cancel"), style: "cancel" },
                  { text: t("plan.key.scanStart"), onPress: () => scan.mutate() },
                ])
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
        return settleNow && settleable > 0 ? (
          <Action
            testID="plan-policy-action"
            label={t("plan.policy.apply", { count: settleable })}
            onPress={() =>
              Alert.alert(
                t("plan.policy.apply", { count: settleable }),
                p.policy.settle === "first_pass"
                  ? t("plan.policy.firstPass")
                  : t("plan.policy.confirm"),
                [
                  { text: t("common.cancel"), style: "cancel" },
                  {
                    text: t("plan.policy.applyShort"),
                    onPress: () => settle.mutate(p.languages.map((l) => l.locale)),
                  },
                ],
              )
            }
          />
        ) : null;
    }
  };

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen options={{ title: t("plan.title") }} />
      <ScrollView
        testID="plan-list"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pt-3 pb-10"
        refreshControl={<RefreshControl refreshing={plan.isRefetching} onRefresh={refresh} />}
      >
        {LEVELS.map((level, i) => {
          const s = stepOf(p, level);
          return (
            <Row
              key={level}
              n={i + 1}
              level={level}
              state={s.state}
              next={p.next === level}
              title={t(`planCard.steps.${level}`)}
              fact={fact(s)}
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
