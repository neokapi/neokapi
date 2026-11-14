// The tab bar: the system's own (UITabBar on iOS, Material on Android). Each
// tab holds a stack of its own, so a tab keeps its place when another is
// opened. Home browses workspaces, projects and languages; Review lists the
// languages waiting for the person across projects; Activity is a project's
// feed; Account is the person, the server and the app.
import { NativeTabs } from "expo-router/native-tabs";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

export default function TabsLayout() {
  const { t } = useTranslation();
  const primary = useCSSVariable("--color-primary");
  return (
    <NativeTabs tintColor={String(primary)}>
      <NativeTabs.Trigger name="(home)">
        <NativeTabs.Trigger.Label>{t("tabs.home")}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: "house", selected: "house.fill" }} md="home" />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="review">
        <NativeTabs.Trigger.Label>{t("tabs.review")}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon
          sf={{ default: "rectangle.stack", selected: "rectangle.stack.fill" }}
          md="stacks"
        />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="activity">
        <NativeTabs.Trigger.Label>{t("tabs.activity")}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: "clock", selected: "clock.fill" }} md="history" />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="account">
        <NativeTabs.Trigger.Label>{t("tabs.account")}</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon
          sf={{ default: "person.crop.circle", selected: "person.crop.circle.fill" }}
          md="account_circle"
        />
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}
