// The workspaces the person belongs to: the first screen of the Home tab,
// with the person's avatar at the top right.
import { useQuery } from "@tanstack/react-query";
import { router } from "expo-router";
import { FlatList, View } from "react-native";
import { useTranslation } from "react-i18next";

import { AccountButton } from "../../../components/AccountButton";
import { QueryState, Row } from "../../../components/List";
import { useAuth } from "../../../lib/auth";

export default function Workspaces() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => client.workspaces() });

  return (
    <View collapsable={false} className="bg-background flex-1">
      <AccountButton />
      <FlatList
        testID="workspace-list"
        contentInsetAdjustmentBehavior="automatic"
        data={workspaces.data ?? []}
        keyExtractor={(w) => w.id}
        refreshing={workspaces.isRefetching}
        onRefresh={() => void workspaces.refetch()}
        renderItem={({ item }) => (
          <Row
            testID={`workspace-${item.slug}`}
            title={item.name}
            detail={item.slug}
            onPress={() => router.push({ pathname: "/[ws]", params: { ws: item.slug } })}
          />
        )}
        ListEmptyComponent={
          <QueryState query={workspaces} empty={workspaces.isSuccess && t("workspaces.empty")} />
        }
      />
    </View>
  );
}
