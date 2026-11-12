// The languages of a project the person may review, each with the number of
// translations waiting. The server says which languages the review
// permission covers; the app lists those and no others.
import type { DeckCounts, Project } from "@neokapi/bowrain-client";
import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { router, useLocalSearchParams } from "expo-router";
import { FlatList, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState, Row } from "../../../components/List";
import { useAuth } from "../../../lib/auth";

export default function Languages() {
  const { t } = useTranslation();
  const { ws, project } = useLocalSearchParams<{ ws: string; project: string }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const scope = { workspace: ws, project };

  const projects = useQuery({
    queryKey: ["projects", ws],
    queryFn: () => client.projects(ws),
    initialData: () => qc.getQueryData<Project[]>(["projects", ws]),
  });
  const info = projects.data?.find((p) => p.id === project);
  const perms = useQuery({
    queryKey: ["permissions", ws, project],
    queryFn: () => client.permissions(ws, project),
  });

  const mayReview = perms.data?.permissions.includes("review") ?? false;
  const scoped = perms.data?.languages ?? [];
  const languages = mayReview
    ? (info?.target_languages ?? []).filter((l) => scoped.length === 0 || scoped.includes(l))
    : [];

  const counts = useQueries({
    queries: languages.map((locale) => ({
      queryKey: ["counts", ws, project, locale],
      queryFn: async () => {
        const deck = await client.deck(scope, locale, { limit: 1 });
        // A server without the deck route counts the queue but does not group it.
        const counts: Partial<DeckCounts> & { total: number } = deck.counts ?? {
          total: deck.total,
        };
        return counts;
      },
    })),
  });

  return (
    <View className="bg-background flex-1">
      <FlatList
        data={languages}
        keyExtractor={(l) => l}
        refreshing={false}
        onRefresh={() => void qc.invalidateQueries({ queryKey: ["counts", ws, project] })}
        ListHeaderComponent={
          info ? (
            <View className="px-4 pt-4 pb-2">
              <Text className="text-muted-foreground text-sm">{info.name}</Text>
            </View>
          ) : undefined
        }
        renderItem={({ item, index }) => {
          const c = counts[index]?.data;
          return (
            <Row
              testID={`language-${item}`}
              title={item}
              detail={
                c?.fast !== undefined
                  ? t("languages.groups", { fast: c.fast, desk: c.desk ?? 0 })
                  : undefined
              }
              trailing={c ? String(c.total) : "…"}
              onPress={() =>
                router.push({
                  pathname: "/[ws]/[project]/[locale]",
                  params: { ws, project, locale: item },
                })
              }
            />
          );
        }}
        ListEmptyComponent={
          <QueryState
            query={perms}
            empty={
              perms.isSuccess && (mayReview ? t("languages.empty") : t("languages.noPermission"))
            }
          />
        }
      />
    </View>
  );
}
