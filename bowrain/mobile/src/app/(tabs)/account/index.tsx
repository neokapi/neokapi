// The account screen, as a grouped list: who is signed in, the account page
// on the server, the server itself, the app's appearance, what build and
// update is running, and signing out.
import { useQueryClient } from "@tanstack/react-query";
import { router } from "expo-router";
import * as WebBrowser from "expo-web-browser";
import { useState } from "react";
import { Alert, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { Avatar } from "../../../components/feed/Avatar";
import { Cell, Section } from "../../../components/Grouped";
import { appInfo, type AppInfo } from "../../../lib/appInfo";
import { APPEARANCES, setAppearance, useAppearance } from "../../../lib/appearance";
import { useAuth } from "../../../lib/auth";
import { queuedDecisions } from "../../../lib/decisions";
import { useProfile } from "../../../lib/profile";
import { serverHost } from "../../../lib/serverChoice";

const APPEARANCE_ICONS = { system: "contrast", light: "sunny", dark: "moon" } as const;

function ProfileHeader() {
  const profile = useProfile();
  return (
    <View testID="account-profile" accessible className="items-center gap-1 px-6 pt-2 pb-2">
      <Avatar actor={{ id: profile.key, kind: "person" }} name={profile.name} size={84} />
      <Text className="text-foreground mt-3 text-center text-2xl font-semibold">
        {profile.name}
      </Text>
      {profile.email && profile.email !== profile.name ? (
        <Text className="text-muted-foreground text-center text-base">{profile.email}</Text>
      ) : null}
      <View className="from-prism-1 via-prism-3 to-prism-5 mt-3 h-1 w-14 rounded-full bg-linear-to-r" />
    </View>
  );
}

/** The update the app runs: its id, the build's own bundle, or none in development. */
function updateLabel(info: AppInfo, t: (key: string) => string): string {
  if (!info.updatesEnabled) return t("account.about.updateNone");
  return info.updateId ?? t("account.about.updateEmbedded");
}

export default function Account() {
  const { t } = useTranslation();
  const { session, signOut } = useAuth();
  const qc = useQueryClient();
  const appearance = useAppearance();
  const profile = useProfile();

  const [info] = useState(appInfo);
  const { version, build } = info;

  const doSignOut = () => void signOut().then(() => router.replace("/sign-in"));

  const confirmSignOut = () => {
    const queued = queuedDecisions(qc).length;
    if (queued > 0) {
      Alert.alert(t("signOut.queuedTitle"), t("signOut.queuedBody", { count: queued }), [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("signOut.confirm"), style: "destructive", onPress: doSignOut },
      ]);
      return;
    }
    Alert.alert(
      t("signOut.confirmTitle"),
      session?.email ? t("signOut.signedInAs", { email: session.email }) : undefined,
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("signOut.confirm"), style: "destructive", onPress: doSignOut },
      ],
    );
  };

  return (
    <ScrollView
      testID="account-screen"
      className="bg-background flex-1"
      contentInsetAdjustmentBehavior="automatic"
      contentContainerClassName="gap-7 pt-2 pb-12"
    >
      <ProfileHeader />

      <Section>
        <Cell
          testID="account-name"
          icon="create"
          tile="green"
          title={t("account.name.title")}
          value={profile.ownName ?? t("account.name.notSet")}
          chevron
          onPress={() => router.push("/account/name")}
        />
        <Cell
          testID="account"
          icon="person"
          tile="blue"
          title={t("account.manage")}
          external
          accessibilityHint={t("account.manageHint")}
          onPress={() =>
            session && void WebBrowser.openBrowserAsync(`${session.serverUrl}/settings`)
          }
        />
        <Cell
          testID="account-server"
          icon="server"
          tile="violet"
          title={t("account.server")}
          value={session ? serverHost(session.serverUrl) : undefined}
          chevron
          onPress={() => router.push("/server")}
        />
      </Section>

      <Section header={t("account.appearance.title")}>
        {APPEARANCES.map((a) => (
          <Cell
            key={a}
            testID={`appearance-${a}`}
            icon={APPEARANCE_ICONS[a]}
            tile={a === "dark" ? "grey" : a === "light" ? "amber" : "green"}
            title={t(`account.appearance.${a}`)}
            selected={appearance === a}
            onPress={() => void setAppearance(a)}
          />
        ))}
      </Section>

      <Section header={t("account.about.title")}>
        <Cell
          testID="about-version"
          title={t("account.about.version")}
          value={build ? t("account.about.versionBuild", { version, build }) : version}
        />
        <Cell
          testID="about-update"
          title={t("account.about.update")}
          value={updateLabel(info, t)}
        />
        {info.channel ? <Cell title={t("account.about.channel")} value={info.channel} /> : null}
      </Section>

      <Section>
        <Cell testID="sign-out" title={t("signOut.button")} destructive onPress={confirmSignOut} />
      </Section>
    </ScrollView>
  );
}
