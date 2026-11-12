// The workspaces the person belongs to.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { router } from "expo-router";
import * as WebBrowser from "expo-web-browser";
import { Alert, FlatList, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState, Row } from "../components/List";
import { useAuth } from "../lib/auth";
import { queuedDecisions } from "../lib/decisions";

export default function Workspaces() {
  const { t } = useTranslation();
  const { client, session, signOut } = useAuth();
  const qc = useQueryClient();
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => client.workspaces() });

  const confirmSignOut = () => {
    const queued = queuedDecisions(qc).length;
    if (queued === 0) {
      void signOut().then(() => router.replace("/sign-in"));
      return;
    }
    Alert.alert(t("signOut.queuedTitle"), t("signOut.queuedBody", { count: queued }), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("signOut.confirm"),
        style: "destructive",
        onPress: () => void signOut().then(() => router.replace("/sign-in")),
      },
    ]);
  };

  return (
    <View className="bg-background flex-1">
      <FlatList
        data={workspaces.data ?? []}
        keyExtractor={(w) => w.id}
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
        ListFooterComponent={
          <View className="items-center gap-2 p-6">
            {session?.email ? (
              <Text className="text-muted-foreground text-sm">
                {t("signOut.signedInAs", { email: session.email })}
              </Text>
            ) : null}
            {session ? (
              <Pressable
                testID="account"
                accessibilityRole="link"
                onPress={() => void WebBrowser.openBrowserAsync(`${session.serverUrl}/settings`)}
              >
                <Text className="text-primary text-base">{t("signOut.account")}</Text>
              </Pressable>
            ) : null}
            <Pressable testID="sign-out" accessibilityRole="button" onPress={confirmSignOut}>
              <Text className="text-primary text-base">{t("signOut.button")}</Text>
            </Pressable>
          </View>
        }
      />
    </View>
  );
}
