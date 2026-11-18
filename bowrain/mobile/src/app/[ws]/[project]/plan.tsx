// A project's review plan: what the project asks of its reviewers, ordered by
// the reach of each decision. The coordinates and the key language generate
// the text, so they come before any string; then the exceptions, then whole
// interfaces read in context, and last what the involvement leaves to the
// checks. The server counts every level (GET /:ws/:id/review/plan); each step
// opens the screen that decides it.
import {
  planProducts,
  type PlanLevel,
  type ReviewInvolvement,
  type ReviewPlan,
} from "@neokapi/bowrain-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { router, Stack, useLocalSearchParams } from "expo-router";
import { Alert, Pressable, RefreshControl, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { reviewPlanKey } from "../../../lib/plan";
import { languageLabel } from "../../../lib/feed";

const INVOLVEMENTS: readonly ReviewInvolvement[] = ["context", "interfaces", "every"];

/** A step's state: done, the next one to work, or waiting behind it. */
type StepState = "done" | "next" | "later";

function stateOf(plan: ReviewPlan, level: PlanLevel, waiting: number): StepState {
  if (plan.next === level) return "next";
  return waiting > 0 ? "later" : "done";
}

function Step({
  n,
  level,
  state,
  title,
  lines,
  action,
  onPress,
}: {
  n: number;
  level: PlanLevel;
  state: StepState;
  title: string;
  lines: string[];
  action?: string;
  onPress?: () => void;
}) {
  const { t } = useTranslation();
  const ring =
    state === "next"
      ? "border-primary bg-primary/10"
      : state === "done"
        ? "border-border bg-card opacity-80"
        : "border-border bg-card";
  return (
    <Pressable
      testID={`plan-${level}`}
      accessibilityRole={onPress ? "button" : undefined}
      disabled={!onPress}
      onPress={onPress}
      className={`mx-4 mb-3 gap-2 rounded-3xl border p-4 ${ring}`}
    >
      <View className="flex-row items-center gap-3">
        <View
          className={`h-7 w-7 items-center justify-center rounded-full ${state === "done" ? "bg-approve" : state === "next" ? "bg-primary" : "bg-muted"}`}
        >
          <Text
            className={`text-sm font-semibold ${state === "later" ? "text-muted-foreground" : "text-primary-foreground"}`}
          >
            {state === "done" ? "✓" : n}
          </Text>
        </View>
        <Text className="text-foreground flex-1 text-[17px] font-semibold">{title}</Text>
        {state === "next" ? (
          <Text
            testID={`plan-${level}-next`}
            className="text-primary text-xs font-semibold uppercase"
          >
            {t("plan.next")}
          </Text>
        ) : null}
      </View>
      {lines.map((line) => (
        <Text key={line} className="text-muted-foreground text-sm leading-5">
          {line}
        </Text>
      ))}
      {action && onPress ? (
        <Text className="text-primary text-sm font-semibold">{action} ›</Text>
      ) : null}
    </Pressable>
  );
}

export default function ReviewPlanScreen() {
  const { t, i18n } = useTranslation();
  const { ws, project } = useLocalSearchParams<{ ws: string; project: string }>();
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
  const mayManage = perms.data?.permissions.includes("manage_project") ?? false;
  const refresh = () => void qc.invalidateQueries({ queryKey: reviewPlanKey(ws, project) });

  const confirm = useMutation({
    mutationFn: () => client.confirmCoordinates(scope),
    onSuccess: refresh,
    onError: (e: Error) => Alert.alert(t("plan.failed"), e.message),
  });
  const involve = useMutation({
    mutationFn: (v: ReviewInvolvement) => client.setInvolvement(scope, v),
    onSuccess: refresh,
    onError: (e: Error) => Alert.alert(t("plan.failed"), e.message),
  });
  const settle = useMutation({
    mutationFn: (locales: string[]) => client.approvePassing(scope, locales),
    onSuccess: (r) => {
      Alert.alert(t("plan.policy.doneTitle"), t("plan.policy.done", { count: r.approved }));
      refresh();
    },
    onError: (e: Error) => Alert.alert(t("plan.failed"), e.message),
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
  const products = planProducts(p.coordinates.points);
  const untranslated = Object.entries(p.keyLanguage.untranslated).filter(([, n]) => n > 0);
  const failing = p.languages.reduce((n, l) => n + l.failing, 0);
  const pages = p.languages.reduce((n, l) => n + l.pages, 0);
  const settleable = p.languages.reduce((n, l) => n + l.settleable, 0);
  const firstLocale = p.languages[0]?.locale;
  const openDeck = (locale: string | undefined) =>
    locale
      ? router.push({ pathname: "/[ws]/[project]/[locale]", params: { ws, project, locale } })
      : undefined;

  const chooseInvolvement = () =>
    Alert.alert(t("plan.involvement.title"), t("plan.involvement.body"), [
      ...INVOLVEMENTS.map((v) => ({
        text: t(`plan.involvement.${v}`),
        onPress: () => involve.mutate(v),
      })),
      { text: t("common.cancel"), style: "cancel" as const },
    ]);

  const coordinateLines = [
    t("plan.coordinates.languages", {
      source: lang(p.coordinates.sourceLanguage),
      targets: p.coordinates.targetLanguages.map(lang).join(", ") || "—",
    }),
    ...products.map((pr) => `${pr.product}: ${pr.channels.join(", ")}`),
    p.coordinates.confirmed
      ? t("plan.coordinates.confirmed")
      : mayManage
        ? t("plan.coordinates.confirmHint")
        : t("plan.coordinates.unconfirmed"),
  ];

  const keyLines = [
    t("plan.key.waiting", { count: p.keyLanguage.waiting }),
    t("plan.key.concepts", { count: p.keyLanguage.concepts }),
    ...untranslated.map(([l, n]) => t("plan.key.untranslated", { count: n, language: lang(l) })),
  ];

  const exceptionLines = [
    p.exceptions.sourceFindings === null
      ? t("plan.exceptions.notChecked")
      : t("plan.exceptions.source", { count: p.exceptions.sourceFindings }),
    t("plan.exceptions.translations", { count: failing }),
  ];

  const interfaceLines = [
    t("plan.interfaces.pages", { count: pages }),
    p.involvement === "context" ? t("plan.interfaces.sample") : t("plan.interfaces.every"),
  ];

  const policyLines =
    p.involvement === "every"
      ? [t("plan.policy.none")]
      : [t("plan.policy.settleable", { count: settleable }), t("plan.policy.how")];

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen options={{ title: t("plan.title") }} />
      <ScrollView
        testID="plan-list"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pt-2 pb-10"
        refreshControl={<RefreshControl refreshing={plan.isRefetching} onRefresh={refresh} />}
      >
        <Text className="text-muted-foreground px-5 pb-4 text-sm leading-5">{t("plan.intro")}</Text>
        <Pressable
          testID="plan-involvement"
          accessibilityRole={mayManage ? "button" : undefined}
          disabled={!mayManage}
          onPress={chooseInvolvement}
          className="bg-muted mx-4 mb-4 rounded-2xl px-4 py-3"
        >
          <Text className="text-muted-foreground text-xs font-semibold uppercase">
            {t("plan.involvement.title")}
          </Text>
          <Text className="text-foreground text-base font-semibold">
            {t(`plan.involvement.${p.involvement}`)}
          </Text>
          <Text className="text-muted-foreground text-sm">
            {t(`plan.involvement.${p.involvement}Detail`)}
          </Text>
        </Pressable>

        <Step
          n={1}
          level="coordinates"
          state={p.coordinates.confirmed ? "done" : "next"}
          title={t("plan.coordinates.title")}
          lines={coordinateLines}
          action={!p.coordinates.confirmed && mayManage ? t("plan.coordinates.confirm") : undefined}
          onPress={
            !p.coordinates.confirmed && mayManage
              ? () =>
                  Alert.alert(t("plan.coordinates.confirm"), t("plan.coordinates.confirmBody"), [
                    { text: t("common.cancel"), style: "cancel" },
                    { text: t("plan.coordinates.confirm"), onPress: () => confirm.mutate() },
                  ])
              : undefined
          }
        />
        <Step
          n={2}
          level="key_language"
          state={stateOf(
            p,
            "key_language",
            p.keyLanguage.waiting + untranslated.reduce((n, [, c]) => n + c, 0),
          )}
          title={t("plan.key.title")}
          lines={keyLines}
          action={p.keyLanguage.waiting > 0 ? t("plan.key.open") : undefined}
          onPress={
            p.keyLanguage.waiting > 0
              ? () => router.push({ pathname: "/[ws]/[project]/context", params: { ws, project } })
              : undefined
          }
        />
        <Step
          n={3}
          level="exceptions"
          state={stateOf(p, "exceptions", (p.exceptions.sourceFindings ?? 0) + failing)}
          title={t("plan.exceptions.title")}
          lines={exceptionLines}
          action={failing > 0 ? t("plan.exceptions.open") : undefined}
          onPress={
            failing > 0 ? () => openDeck(p.languages.find((l) => l.failing > 0)?.locale) : undefined
          }
        />
        <Step
          n={4}
          level="interfaces"
          state={stateOf(p, "interfaces", pages)}
          title={t("plan.interfaces.title")}
          lines={interfaceLines}
          action={pages > 0 ? t("plan.interfaces.open") : undefined}
          onPress={pages > 0 ? () => openDeck(firstLocale) : undefined}
        />
        <Step
          n={5}
          level="policy"
          state={stateOf(p, "policy", settleable)}
          title={t("plan.policy.title")}
          lines={policyLines}
          action={settleable > 0 ? t("plan.policy.apply", { count: settleable }) : undefined}
          onPress={
            settleable > 0
              ? () =>
                  Alert.alert(
                    t("plan.policy.apply", { count: settleable }),
                    t("plan.policy.confirm"),
                    [
                      { text: t("common.cancel"), style: "cancel" },
                      {
                        text: t("plan.policy.applyShort"),
                        onPress: () => settle.mutate(p.languages.map((l) => l.locale)),
                      },
                    ],
                  )
              : undefined
          }
        />
      </ScrollView>
    </View>
  );
}
