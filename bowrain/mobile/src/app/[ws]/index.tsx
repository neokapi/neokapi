// The projects in a workspace.
import { useQuery } from "@tanstack/react-query";
import { router, useLocalSearchParams } from "expo-router";
import { FlatList, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState, Row } from "../../components/List";
import { useAuth } from "../../lib/auth";

export default function Projects() {
  const { t } = useTranslation();
  const { ws } = useLocalSearchParams<{ ws: string }>();
  const { client } = useAuth();
  const projects = useQuery({ queryKey: ["projects", ws], queryFn: () => client.projects(ws) });

  return (
    <View className="bg-background flex-1">
      <FlatList
        data={projects.data ?? []}
        keyExtractor={(p) => p.id}
        renderItem={({ item }) => (
          <Row
            testID={`project-${item.id}`}
            title={item.name}
            detail={t("projects.languages", { count: item.target_languages.length })}
            onPress={() =>
              router.push({ pathname: "/[ws]/[project]", params: { ws, project: item.id } })
            }
          />
        )}
        ListEmptyComponent={
          <QueryState query={projects} empty={projects.isSuccess && t("projects.empty")} />
        }
      />
    </View>
  );
}
