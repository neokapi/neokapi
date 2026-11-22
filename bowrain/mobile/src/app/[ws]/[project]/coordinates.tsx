// The coordinates map: a treemap of the products and their channels, each
// tile sized by its blocks, shaded by the share established in the languages
// shown, and outlined when it changed since the team last confirmed. What
// changed comes first as chips, the removed points sit under the map, and one
// confirmation records the map for the whole team
// (GET /:ws/:id/review/coordinates, POST /:ws/:id/review/plan/coordinates).
import {
  coordinatesOpen,
  type CoordinateCell,
  type CoordinateChange,
} from "@neokapi/bowrain-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Stack, useLocalSearchParams } from "expo-router";
import { useState } from "react";
import {
  Alert,
  Pressable,
  RefreshControl,
  ScrollView,
  Text,
  useWindowDimensions,
  View,
} from "react-native";
import { useTranslation } from "react-i18next";

import {
  CoordinateCellSheet,
  CoordinatesTreemap,
  shadeSwatches,
  useMapColors,
  ZoomOut,
} from "../../../components/CoordinatesTreemap";
import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { languageLabel } from "../../../lib/feed";
import { reviewCoordinatesKey, reviewPlanKey } from "../../../lib/plan";

/** A point's place as a person reads it: product / channel. */
function placeOf(coords: Record<string, string> | undefined): string {
  if (!coords) return "";
  return [coords.product, coords.channel].filter(Boolean).join(" / ");
}

function ChangeChip({ change, lang }: { change: CoordinateChange; lang: (l: string) => string }) {
  const { t } = useTranslation();
  const where = placeOf(change.coordinates) || change.collection || "";
  let key: string = change.kind;
  if (change.kind === "voice_changed" && !change.voice) key = "voice_removed";
  if (change.kind === "source_only_changed" && !change.sourceOnly) key = "source_only_cleared";
  const text = t(`plan.coordinates.chip.${key}`, { where, language: lang(change.locale ?? "") });
  const tone =
    change.kind === "point_removed" || change.kind === "language_removed"
      ? "bg-removed"
      : change.kind === "point_added" || change.kind === "language_added"
        ? "bg-added"
        : "bg-warning";
  return (
    <View className={`rounded-full px-2.5 py-1 ${tone}`}>
      <Text numberOfLines={1} className="text-foreground text-xs">
        {text}
      </Text>
    </View>
  );
}

function Legend({ shown, lang }: { shown: string[]; lang: (l: string) => string }) {
  const { t } = useTranslation();
  const colors = useMapColors();
  return (
    <View testID="coordinates-legend" className="gap-2 px-5 pt-3">
      <View className="flex-row flex-wrap items-center gap-x-4 gap-y-2">
        <View className="flex-row items-center gap-1">
          <Text className="text-muted-foreground text-[11px]">0%</Text>
          {shadeSwatches(colors).map((fill, i) => (
            <View
              key={`${fill}-${i}`}
              className="border-border h-3 w-4 rounded-sm border"
              style={{ backgroundColor: fill }}
            />
          ))}
          <Text className="text-muted-foreground text-[11px]">100%</Text>
        </View>
        <View className="flex-row items-center gap-1">
          <View className="h-3 w-4 rounded-sm border-2" style={{ borderColor: colors.added }} />
          <Text className="text-muted-foreground text-[11px]">
            {t("plan.coordinates.outlineNew")}
          </Text>
        </View>
        <View className="flex-row items-center gap-1">
          <View className="h-3 w-4 rounded-sm border-2" style={{ borderColor: colors.changed }} />
          <Text className="text-muted-foreground text-[11px]">
            {t("plan.coordinates.outlineChanged")}
          </Text>
        </View>
      </View>
      {shown.length > 0 ? (
        <Text className="text-muted-foreground text-xs leading-4">
          {t(shown.length > 1 ? "plan.coordinates.shadeLowest" : "plan.coordinates.shade", {
            languages: shown.map(lang).join(", "),
          })}
        </Text>
      ) : null}
    </View>
  );
}

export default function CoordinatesScreen() {
  const { t, i18n } = useTranslation();
  // languages narrows the shading to the named target languages,
  // comma-separated; every target language when unset.
  const { ws, project, languages } = useLocalSearchParams<{
    ws: string;
    project: string;
    languages?: string;
  }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const scope = { workspace: ws, project };
  const [zoom, setZoom] = useState<string | null>(null);
  const [picked, setPicked] = useState<CoordinateCell | null>(null);
  const window = useWindowDimensions();
  const map = useQuery({
    queryKey: reviewCoordinatesKey(ws, project),
    queryFn: () => client.reviewCoordinates(scope),
  });
  const perms = useQuery({
    queryKey: ["permissions", ws, project],
    queryFn: () => client.permissions(ws, project),
  });
  const mayManage = perms.data?.permissions.includes("manage_project") ?? false;
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: reviewCoordinatesKey(ws, project) });
    void qc.invalidateQueries({ queryKey: reviewPlanKey(ws, project) });
  };
  const confirm = useMutation({
    mutationFn: () => client.confirmCoordinates(scope),
    onSuccess: refresh,
    onError: (e: Error) => Alert.alert(t("plan.failed"), e.message),
  });

  if (!map.data) {
    return (
      <View className="bg-background flex-1">
        <Stack.Screen options={{ title: t("plan.coordinates.mapTitle") }} />
        <QueryState query={map} />
      </View>
    );
  }
  const { coordinates: c, cells } = map.data;
  const lang = (l: string) => languageLabel(l, i18n.language);
  const wanted = languages ? languages.split(",").filter(Boolean) : [];
  const shown =
    wanted.length > 0 ? c.targetLanguages.filter((l) => wanted.includes(l)) : c.targetLanguages;
  const open = coordinatesOpen(c);
  const name = c.confirmedByName || c.confirmedBy;
  const products = [...new Set(cells.map((cell) => cell.product))];
  // One product needs no zooming: its channels fill the map from the start.
  const only = products.length === 1 ? products[0] : null;
  const zoomed = only ?? (zoom !== null && products.includes(zoom) ? zoom : null);
  const mapWidth = Math.max(0, window.width - 32);
  const mapHeight = Math.min(560, Math.round(mapWidth * 1.15));
  const removed = c.changes.filter((ch) => ch.kind === "point_removed");

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen options={{ title: t("plan.coordinates.mapTitle") }} />
      <ScrollView
        testID="coordinates-screen"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pt-2 pb-10"
        refreshControl={<RefreshControl refreshing={map.isRefetching} onRefresh={refresh} />}
      >
        <Text className="text-muted-foreground px-5 pb-3 text-sm leading-5">
          {t("plan.coordinates.languages", {
            source: lang(c.sourceLanguage),
            targets: c.targetLanguages.map(lang).join(", ") || "-",
          })}
        </Text>

        <View testID="coordinates-changes" className="mx-4 mb-3 gap-2">
          {!c.confirmed ? (
            <Text className="text-foreground text-sm leading-5">
              {t("plan.coordinates.neverConfirmed")}
            </Text>
          ) : (
            <>
              <Text className="text-muted-foreground text-xs font-semibold uppercase">
                {name ? t("plan.coordinates.since", { name }) : t("plan.coordinates.sinceSomeone")}
              </Text>
              {c.changes.length === 0 ? (
                <Text className="text-foreground text-sm">{t("plan.coordinates.noChanges")}</Text>
              ) : (
                <View className="flex-row flex-wrap gap-1.5">
                  {c.changes.map((ch, i) => (
                    <ChangeChip
                      key={`${ch.kind}-${ch.collection ?? ch.locale}-${i}`}
                      change={ch}
                      lang={lang}
                    />
                  ))}
                </View>
              )}
            </>
          )}
        </View>

        {cells.length === 0 ? (
          <Text className="text-muted-foreground px-5 text-sm">{t("plan.coordinates.empty")}</Text>
        ) : (
          <>
            <View className="min-h-8 justify-center px-4 pb-1">
              {zoomed !== null && only === null ? (
                <ZoomOut product={zoomed} onPress={() => setZoom(null)} />
              ) : (
                <Text className="text-muted-foreground text-xs">
                  {zoomed === null
                    ? t("plan.coordinates.zoomHint")
                    : t("plan.coordinates.tileHint")}
                </Text>
              )}
            </View>
            <View className="px-4">
              <CoordinatesTreemap
                cells={cells}
                languages={shown}
                width={mapWidth}
                height={mapHeight}
                zoomed={zoomed}
                onZoom={setZoom}
                onOpen={setPicked}
              />
            </View>
            <Legend shown={shown} lang={lang} />
          </>
        )}

        {removed.length > 0 ? (
          <View testID="coordinates-removed" className="gap-1 px-5 pt-4">
            <Text className="text-muted-foreground text-xs font-semibold uppercase">
              {t("plan.coordinates.removed")}
            </Text>
            {removed.map((ch, i) => (
              <Text key={`${ch.collection}-${i}`} className="text-foreground text-sm">
                {[placeOf(ch.coordinates), ch.collection].filter(Boolean).join(" · ")}
              </Text>
            ))}
          </View>
        ) : null}

        {open && mayManage ? (
          <Pressable
            testID="coordinates-confirm"
            accessibilityRole="button"
            disabled={confirm.isPending}
            onPress={() =>
              Alert.alert(
                t("plan.coordinates.confirmChanges"),
                t("plan.coordinates.confirmChangesBody"),
                [
                  { text: t("common.cancel"), style: "cancel" },
                  { text: t("plan.coordinates.confirm"), onPress: () => confirm.mutate() },
                ],
              )
            }
            className="bg-primary mx-4 mt-5 items-center rounded-full px-4 py-3"
          >
            <Text className="text-primary-foreground text-base font-semibold">
              {t("plan.coordinates.confirmChanges")}
            </Text>
          </Pressable>
        ) : null}
      </ScrollView>
      <CoordinateCellSheet
        cell={picked}
        languages={shown}
        languageLabel={lang}
        onClose={() => setPicked(null)}
      />
    </View>
  );
}
