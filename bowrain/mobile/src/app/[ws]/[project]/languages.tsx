// The languages a person reviews in one project, and, for someone who manages
// the project, its languages and how involved its reviewers are. On a first
// visit it is the review setup; afterwards it is the same screen as a setting.
// The server says what the person may follow (GET /:ws/:id/review/languages)
// and files a project-language change as a recipe change for the next sync.
// A manager may also set the involvement for a product or channel in place of
// the project's; the server decides which setting applies where.
import {
  overridePoints,
  pointKey,
  pointLabel,
  projectLanguagesAfterSync,
  type InvolvementOverride,
  planLevelOf,
  type ReviewInvolvement,
  type ReviewPlan,
} from "@neokapi/bowrain-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { router, Stack, useLocalSearchParams } from "expo-router";
import { useState } from "react";
import { Alert, Pressable, ScrollView, Switch, Text, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";

import { Cell, Section } from "../../../components/Grouped";
import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { languageLabel } from "../../../lib/feed";
import { inboxKey } from "../../../lib/home";
import { reviewLanguagesKey } from "../../../lib/languages";
import { reviewPlanKey } from "../../../lib/plan";
import { planStepRoute } from "../../../lib/reviewNav";

const INVOLVEMENTS: readonly ReviewInvolvement[] = ["context", "interfaces", "every"];

/** A row with a switch, for a project language. */
function ToggleRow({
  testID,
  title,
  value,
  detail,
  onChange,
}: {
  testID: string;
  title: string;
  value: boolean;
  detail?: string;
  onChange: (on: boolean) => void;
}) {
  return (
    <View className="min-h-[48px] flex-row items-center gap-3 px-4 py-2.5">
      <View className="flex-1">
        <Text className="text-foreground text-base">{title}</Text>
        {detail ? <Text className="text-muted-foreground text-xs">{detail}</Text> : null}
      </View>
      <Switch testID={testID} accessibilityLabel={title} value={value} onValueChange={onChange} />
    </View>
  );
}

export default function ReviewLanguagesScreen() {
  const { t, i18n } = useTranslation();
  const params = useLocalSearchParams<{
    ws: string;
    project: string;
    setup?: string;
    next?: string;
  }>();
  const { ws, project } = params;
  const setup = params.setup === "1";
  const { client } = useAuth();
  const qc = useQueryClient();
  const lang = (l: string) => languageLabel(l, i18n.language);

  const languages = useQuery({
    queryKey: reviewLanguagesKey(ws, project),
    queryFn: ({ signal }) => client.with({ signal }).reviewLanguages(ws, project),
  });
  const canManage = languages.data?.canManage ?? false;
  const plan = useQuery({
    queryKey: reviewPlanKey(ws, project),
    queryFn: ({ signal }) => client.with({ signal }).reviewPlan({ workspace: ws, project }),
    enabled: canManage,
  });

  // The person's choice before it is saved; null until they change it.
  const [picked, setPicked] = useState<string[] | null>(null);
  const [adding, setAdding] = useState("");
  const followed = picked ?? languages.data?.followed ?? [];

  const failed = (e: Error) => Alert.alert(t("follow.failed"), e.message);
  const save = useMutation({
    mutationFn: () => client.setFollowedLanguages(ws, project, followed),
    onSuccess: (next) => {
      qc.setQueryData(reviewLanguagesKey(ws, project), next);
      void qc.invalidateQueries({ queryKey: inboxKey });
      void qc.invalidateQueries({ queryKey: reviewPlanKey(ws, project) });
      const step = planLevelOf(params.next);
      if (setup && step) {
        router.replace(planStepRoute(ws, project, step, next.followed));
      } else {
        router.back();
      }
    },
    onError: failed,
  });
  const toggleProject = useMutation({
    mutationFn: (v: { locale: string; enabled: boolean }) =>
      client.setProjectLanguage(ws, project, v.locale, v.enabled),
    onSuccess: (next) => {
      qc.setQueryData(reviewLanguagesKey(ws, project), next);
      setAdding("");
    },
    onError: failed,
  });
  // The tick moves on the tap. The plan is recomputed over the whole project,
  // so it is refreshed in the background and the earlier choice comes back
  // only when the save fails.
  const involve = useMutation({
    mutationFn: (v: ReviewInvolvement) => client.setInvolvement({ workspace: ws, project }, v),
    onMutate: async (v) => {
      const key = reviewPlanKey(ws, project);
      await qc.cancelQueries({ queryKey: key });
      const before = qc.getQueryData<ReviewPlan>(key);
      if (before) qc.setQueryData<ReviewPlan>(key, { ...before, involvement: v });
      return { before };
    },
    onError: (e, _v, ctx) => {
      if (ctx?.before) qc.setQueryData(reviewPlanKey(ws, project), ctx.before);
      failed(e);
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: reviewPlanKey(ws, project) }),
  });
  // The point being edited: an override's key, "new" while a point is
  // chosen for a new one, or null.
  const [editing, setEditing] = useState<string | null>(null);
  const [newPoint, setNewPoint] = useState<Record<string, string> | null>(null);
  // Overrides save as a whole list. The rows change on the tap, as the
  // involvement does, and come back as they were only when the save fails.
  const override = useMutation({
    mutationFn: (next: InvolvementOverride[]) =>
      client.setInvolvementOverrides(
        { workspace: ws, project },
        next.map((o) => ({ coordinates: o.coordinates, involvement: o.involvement })),
      ),
    onMutate: async (next) => {
      const key = reviewPlanKey(ws, project);
      await qc.cancelQueries({ queryKey: key });
      const before = qc.getQueryData<ReviewPlan>(key);
      if (before) qc.setQueryData<ReviewPlan>(key, { ...before, overrides: next });
      setEditing(null);
      setNewPoint(null);
      return { before };
    },
    onError: (e, _v, ctx) => {
      if (ctx?.before) qc.setQueryData(reviewPlanKey(ws, project), ctx.before);
      failed(e);
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: reviewPlanKey(ws, project) }),
  });

  const title = setup ? t("follow.setupTitle") : t("follow.title");
  const l = languages.data;
  if (!l) {
    return (
      <View className="bg-background flex-1">
        <Stack.Screen options={{ title }} />
        <QueryState query={languages} />
      </View>
    );
  }

  const after = projectLanguagesAfterSync(l);
  const projectRows = [...new Set([...l.project, ...after])];
  const confirmProject = (locale: string, enabled: boolean) =>
    Alert.alert(
      t(enabled ? "follow.project.turnOn" : "follow.project.turnOff", { language: lang(locale) }),
      t("follow.project.changeBody"),
      [
        { text: t("common.cancel"), style: "cancel" },
        {
          text: t(enabled ? "follow.project.on" : "follow.project.off"),
          onPress: () => toggleProject.mutate({ locale, enabled }),
        },
      ],
    );
  const togglePick = (locale: string) =>
    setPicked(
      followed.includes(locale) ? followed.filter((x) => x !== locale) : [...followed, locale],
    );
  const confirmed = plan.data?.coordinates.confirmed;
  const overrides = plan.data?.overrides ?? [];
  const setOverride = (coordinates: Record<string, string>, involvement: ReviewInvolvement) => {
    const key = pointKey(coordinates);
    const rest = overrides.filter((o) => pointKey(o.coordinates) !== key);
    override.mutate([...rest, { coordinates, involvement, collections: 0 }]);
  };
  const removeOverride = (key: string) =>
    override.mutate(overrides.filter((o) => pointKey(o.coordinates) !== key));
  const levels = (point: Record<string, string>, current?: ReviewInvolvement) =>
    INVOLVEMENTS.map((v) => (
      <Cell
        key={v}
        testID={`follow-override-level-${v}`}
        title={t(`plan.involvement.${v}`)}
        selected={current === v}
        onPress={() => setOverride(point, v)}
      />
    ));

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen options={{ title }} />
      <ScrollView
        testID="follow-screen"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="gap-7 pt-4 pb-12"
        keyboardShouldPersistTaps="handled"
      >
        {canManage ? (
          <Section header={t("follow.coordinates.header")}>
            <Cell
              testID="follow-coordinates"
              icon="grid"
              tile="violet"
              title={t("follow.coordinates.title")}
              value={
                confirmed === undefined
                  ? undefined
                  : t(confirmed ? "follow.coordinates.confirmed" : "follow.coordinates.unconfirmed")
              }
              chevron
              onPress={() =>
                router.push({
                  pathname: "/[ws]/[project]/coordinates",
                  params: { ws, project, languages: followed.join(",") },
                })
              }
            />
          </Section>
        ) : null}

        {canManage ? (
          <Section header={t("follow.project.header")} footer={t("follow.project.footer")}>
            {projectRows.map((locale) => {
              const on = after.includes(locale);
              const changing = on !== l.project.includes(locale);
              return (
                <ToggleRow
                  key={locale}
                  testID={`follow-project-${locale}`}
                  title={lang(locale)}
                  value={on}
                  detail={changing ? t("follow.project.pending") : undefined}
                  onChange={(v) => confirmProject(locale, v)}
                />
              );
            })}
            <View className="flex-row items-center gap-2 px-4 py-2">
              <TextInput
                testID="follow-project-add-input"
                accessibilityLabel={t("follow.project.addPlaceholder")}
                className="text-foreground flex-1 py-2 text-base"
                placeholder={t("follow.project.addPlaceholder")}
                value={adding}
                onChangeText={setAdding}
                autoCapitalize="none"
                autoCorrect={false}
              />
              <Pressable
                testID="follow-project-add"
                accessibilityRole="button"
                disabled={adding.trim() === ""}
                onPress={() => confirmProject(adding.trim(), true)}
                className="bg-chip rounded-full px-3 py-1.5 disabled:opacity-40"
              >
                <Text className="text-chip-foreground text-sm font-semibold">
                  {t("follow.project.add")}
                </Text>
              </Pressable>
            </View>
          </Section>
        ) : null}

        {canManage && plan.data ? (
          <Section header={t("follow.involvement.header")}>
            {INVOLVEMENTS.map((v) => (
              <Cell
                key={v}
                testID={`follow-involvement-${v}`}
                title={t(`plan.involvement.${v}`)}
                selected={plan.data.involvement === v}
                onPress={() => involve.mutate(v)}
              />
            ))}
          </Section>
        ) : null}

        {canManage && plan.data ? (
          <Section
            header={t("follow.involvement.overrides.header")}
            footer={t("follow.involvement.overrides.footer")}
          >
            {overrides.map((o) => {
              const key = pointKey(o.coordinates);
              const point = pointLabel(o.coordinates);
              const level = t(`plan.involvement.${o.involvement}`);
              return (
                <View key={key}>
                  <Cell
                    testID={`follow-override-${key}`}
                    title={point}
                    value={level}
                    accessibilityLabel={t("follow.involvement.overrides.row", { point, level })}
                    chevron
                    onPress={() => setEditing(editing === key ? null : key)}
                  />
                  {editing === key ? (
                    <>
                      {levels(o.coordinates, o.involvement)}
                      <Cell
                        testID="follow-override-remove"
                        title={t("follow.involvement.overrides.remove")}
                        destructive
                        onPress={() => removeOverride(key)}
                      />
                    </>
                  ) : null}
                </View>
              );
            })}
            {editing === "new" ? (
              newPoint ? (
                <>
                  <Cell
                    testID="follow-override-new-point"
                    title={pointLabel(newPoint)}
                    value={t("follow.involvement.overrides.choose")}
                  />
                  {levels(newPoint)}
                </>
              ) : (
                overridePoints(plan.data.coordinates.points, overrides).map((c) => (
                  <Cell
                    key={pointKey(c)}
                    testID={`follow-override-point-${pointKey(c)}`}
                    title={pointLabel(c)}
                    chevron
                    onPress={() => setNewPoint(c)}
                  />
                ))
              )
            ) : (
              <Cell
                testID="follow-override-add"
                icon="add"
                tile="green"
                title={t("follow.involvement.overrides.add")}
                onPress={() => {
                  setNewPoint(null);
                  setEditing("new");
                }}
              />
            )}
          </Section>
        ) : null}

        <Section header={t("follow.mine.header")} footer={t("follow.mine.footer")}>
          {l.allowed.length === 0 ? (
            <Cell testID="follow-mine-none" title={t("follow.mine.none")} />
          ) : (
            l.allowed.map((locale) => (
              <Cell
                key={locale}
                testID={`follow-mine-${locale}`}
                title={lang(locale)}
                selected={followed.includes(locale)}
                onPress={() => togglePick(locale)}
              />
            ))
          )}
        </Section>

        <View className="px-4">
          <Pressable
            testID="follow-save"
            accessibilityRole="button"
            disabled={save.isPending}
            onPress={() => save.mutate()}
            className="bg-primary items-center rounded-2xl py-4 active:opacity-80 disabled:opacity-50"
          >
            <Text className="text-primary-foreground text-base font-semibold">
              {t("follow.save")}
            </Text>
          </Pressable>
        </View>
      </ScrollView>
    </View>
  );
}
