// The server sheet, opened from the cog on the sign-in screen. A choice of
// rows: Bowrain Cloud, the local stack (development builds only), or a
// server the person types in. A fixed row applies at once and closes the
// sheet; the custom row reveals the address field and its confirm button.
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { useState } from "react";
import { Platform, Pressable, Text, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { cloudServerUrl, localServerUrl } from "../lib/config";
import {
  normalizeServerUrl,
  optionServer,
  serverHost,
  serverOption,
  setServerChoice,
  useServerChoice,
  type ServerOption,
} from "../lib/serverChoice";

function OptionRow({
  option,
  title,
  detail,
  selected,
  onPress,
  last,
}: {
  option: ServerOption;
  title: string;
  detail?: string;
  selected: boolean;
  onPress: () => void;
  last?: boolean;
}) {
  const primary = useCSSVariable("--color-primary");
  const muted = useCSSVariable("--color-muted-foreground");
  // iOS marks the chosen row with a checkmark; Android uses a radio button.
  const mark =
    Platform.OS === "ios" ? (
      selected ? (
        <Ionicons name="checkmark" size={22} color={String(primary)} />
      ) : null
    ) : (
      <Ionicons
        name={selected ? "radio-button-on" : "radio-button-off"}
        size={22}
        color={String(selected ? primary : muted)}
      />
    );
  return (
    <Pressable
      testID={`server-option-${option}`}
      accessibilityRole="radio"
      accessibilityState={{ selected }}
      onPress={onPress}
      className={`flex-row items-center gap-3 px-4 py-3.5 active:opacity-60 ${
        last ? "" : "border-border border-b"
      }`}
    >
      <View className="flex-1 gap-0.5">
        <Text className="text-foreground text-base">{title}</Text>
        {detail ? <Text className="text-muted-foreground text-sm">{detail}</Text> : null}
      </View>
      <View className="h-6 w-6 items-center justify-center">{mark}</View>
    </Pressable>
  );
}

export default function Server() {
  const { t } = useTranslation();
  const { server } = useServerChoice();
  const current = serverOption(server, __DEV__);
  const [selected, setSelected] = useState<ServerOption>(current);
  const [value, setValue] = useState(current === "custom" ? server : "");
  const [invalid, setInvalid] = useState(false);

  const choose = async (option: Exclude<ServerOption, "custom">) => {
    setSelected(option);
    await setServerChoice(optionServer(option));
    router.back();
  };

  const save = async () => {
    const url = normalizeServerUrl(value);
    if (!url) {
      setInvalid(true);
      return;
    }
    await setServerChoice(url);
    router.back();
  };

  return (
    <View className="bg-background flex-1 gap-4 px-4 pt-6">
      <Text accessibilityRole="header" className="text-foreground px-2 text-xl font-semibold">
        {t("server.title")}
      </Text>
      <View accessibilityRole="radiogroup" className="bg-card overflow-hidden rounded-xl">
        <OptionRow
          option="cloud"
          title={t("server.cloud")}
          detail={serverHost(cloudServerUrl)}
          selected={selected === "cloud"}
          onPress={() => void choose("cloud")}
        />
        {__DEV__ ? (
          <OptionRow
            option="local"
            title={t("server.local")}
            detail={serverHost(localServerUrl)}
            selected={selected === "local"}
            onPress={() => void choose("local")}
          />
        ) : null}
        <OptionRow
          option="custom"
          title={t("server.custom")}
          selected={selected === "custom"}
          onPress={() => setSelected("custom")}
          last
        />
      </View>
      {selected === "custom" ? (
        <View className="gap-3">
          <TextInput
            testID="server-url"
            accessibilityLabel={t("server.custom")}
            className="bg-card border-border text-foreground rounded-xl border p-4 text-base"
            value={value}
            onChangeText={(v) => {
              setValue(v);
              setInvalid(false);
            }}
            placeholder="https://bowrain.example.com"
            autoCapitalize="none"
            autoCorrect={false}
            autoFocus={current !== "custom"}
            keyboardType="url"
            inputMode="url"
            returnKeyType="done"
            onSubmitEditing={() => void save()}
          />
          {invalid ? (
            <Text testID="server-invalid" className="text-corrective px-2 text-sm">
              {t("server.invalid")}
            </Text>
          ) : null}
          <Pressable
            testID="server-save"
            accessibilityRole="button"
            disabled={value.trim() === ""}
            onPress={() => void save()}
            className="bg-primary items-center rounded-2xl py-4 active:opacity-80 disabled:opacity-50"
          >
            <Text className="text-primary-foreground text-base font-semibold">
              {t("server.save")}
            </Text>
          </Pressable>
        </View>
      ) : null}
    </View>
  );
}
