// Picks the project whose review languages to set: every project of every
// workspace the person belongs to. Reached from the Review tab's header chip
// and from the Account tab.
import { useQuery } from "@tanstack/react-query";
import { router, Stack } from "expo-router";
import { ScrollView, View } from "react-native";
import { useTranslation } from "react-i18next";

import { Cell, Section } from "../components/Grouped";
import { QueryState } from "../components/List";
import { EmptyState } from "../components/home/parts";
import { useAuth } from "../lib/auth";

export default function ReviewLanguagesPicker() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const all = useQuery({
    queryKey: ["projects-everywhere"],
    queryFn: async () => {
      const workspaces = await client.workspaces();
      return Promise.all(
        workspaces.map(async (w) => ({ workspace: w, projects: await client.projects(w.slug) })),
      );
    },
  });

  if (!all.data) {
    return (
      <View className="bg-background flex-1">
        <Stack.Screen options={{ title: t("follow.title") }} />
        <QueryState query={all} />
      </View>
    );
  }
  const groups = all.data.filter((g) => g.projects.length > 0);
  return (
    <ScrollView
      testID="follow-picker"
      className="bg-background flex-1"
      contentInsetAdjustmentBehavior="automatic"
      contentContainerClassName="gap-7 pt-4 pb-12"
    >
      <Stack.Screen options={{ title: t("follow.title") }} />
      {groups.length === 0 ? <EmptyState icon="folder-open" title={t("follow.pickEmpty")} /> : null}
      {groups.map((g, i) => (
        <Section
          key={g.workspace.slug}
          header={g.workspace.name}
          footer={i === groups.length - 1 ? t("follow.pickFooter") : undefined}
        >
          {g.projects.map((p) => (
            <Cell
              key={p.id}
              testID={`follow-pick-${p.id}`}
              title={p.name}
              chevron
              onPress={() =>
                router.push({
                  pathname: "/[ws]/[project]/languages",
                  params: { ws: g.workspace.slug, project: p.id },
                })
              }
            />
          ))}
        </Section>
      ))}
    </ScrollView>
  );
}
