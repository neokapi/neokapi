// The coordinates map: the products as rows and the channels as columns, each
// cell with its blocks, its voice and a coverage bar per target language. What
// changed since the team last confirmed comes first, and the changed cells are
// outlined. One confirmation records the map for the whole team
// (GET /:ws/:id/review/coordinates, POST /:ws/:id/review/plan/coordinates).
import {
  coordinateGrid,
  coordinatesOpen,
  type CoordinateCell,
  type CoordinateChange,
} from "@neokapi/bowrain-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Stack, useLocalSearchParams } from "expo-router";
import { useState } from "react";
import { Alert, Pressable, RefreshControl, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { languageLabel } from "../../../lib/feed";
import { reviewCoordinatesKey, reviewPlanKey } from "../../../lib/plan";

const CELL_WIDTH = 136;
const LABEL_WIDTH = 96;

/** A point's place as a person reads it: product / channel. */
function placeOf(coords: Record<string, string> | undefined): string {
  if (!coords) return "";
  return [coords.product, coords.channel].filter(Boolean).join(" / ");
}

function ChangeLine({ change, lang }: { change: CoordinateChange; lang: (l: string) => string }) {
  const { t } = useTranslation();
  const where = placeOf(change.coordinates);
  const collection = change.collection ?? "";
  let text: string;
  switch (change.kind) {
    case "voice_changed":
      text = change.voice
        ? t("plan.coordinates.change.voice_changed", { collection, voice: change.voice })
        : t("plan.coordinates.change.voice_removed", { collection });
      break;
    case "source_only_changed":
      text = change.sourceOnly
        ? t("plan.coordinates.change.source_only_changed", { collection })
        : t("plan.coordinates.change.source_only_cleared", { collection });
      break;
    case "point_moved":
      text = t("plan.coordinates.change.point_moved", {
        collection,
        from: placeOf(change.previousCoordinates),
        where,
      });
      break;
    case "language_added":
    case "language_removed":
      text = t(`plan.coordinates.change.${change.kind}`, { language: lang(change.locale ?? "") });
      break;
    default:
      text = t(`plan.coordinates.change.${change.kind}`, { collection, where });
  }
  const tone =
    change.kind === "point_removed" || change.kind === "language_removed"
      ? "bg-destructive"
      : "bg-primary";
  return (
    <View className="flex-row items-start gap-2">
      <View className={`mt-1.5 h-2 w-2 rounded-full ${tone}`} />
      <Text className="text-foreground flex-1 text-sm leading-5">{text}</Text>
    </View>
  );
}

function CoverageBar({ share }: { share: number }) {
  const pct = Math.max(0, Math.min(1, share)) * 100;
  return (
    <View className="bg-muted h-1 overflow-hidden rounded-full">
      <View className="bg-approve h-1 rounded-full" style={{ width: `${pct}%` }} />
    </View>
  );
}

function Cell({
  cell,
  languages,
  height,
  selected,
  onPress,
}: {
  cell: CoordinateCell;
  languages: string[];
  height: number;
  selected: boolean;
  onPress: () => void;
}) {
  const { t } = useTranslation();
  const changed = cell.changes.length > 0;
  const border = selected
    ? "border-foreground border-2"
    : changed
      ? "border-primary border-2"
      : "border-border border";
  return (
    <Pressable
      testID={`coordinates-cell-${cell.product}-${cell.channel}`}
      accessibilityRole="button"
      accessibilityState={{ selected }}
      onPress={onPress}
      style={{ width: CELL_WIDTH - 8, height }}
      className={`bg-card m-1 gap-1 rounded-2xl p-2 ${border}`}
    >
      <Text className="text-foreground text-[15px] font-semibold">
        {t("plan.coordinates.blocks", { count: cell.blocks })}
      </Text>
      <Text numberOfLines={1} className="text-muted-foreground text-xs">
        {cell.voices.length > 0 ? cell.voices.join(", ") : t("plan.coordinates.noVoice")}
      </Text>
      {cell.sourceOnly ? (
        <Text className="text-muted-foreground text-xs">{t("plan.coordinates.sourceOnly")}</Text>
      ) : (
        <View className="gap-1 pt-1">
          {languages.map((l) => (
            <CoverageBar key={l} share={cell.coverage[l] ?? 0} />
          ))}
        </View>
      )}
    </Pressable>
  );
}

export default function CoordinatesScreen() {
  const { t, i18n } = useTranslation();
  // languages narrows the coverage bars to the named target languages,
  // comma-separated; every target language when unset.
  const { ws, project, languages } = useLocalSearchParams<{
    ws: string;
    project: string;
    languages?: string;
  }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const scope = { workspace: ws, project };
  const [selected, setSelected] = useState<string | null>(null);
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
  const grid = coordinateGrid(cells);
  const rowHeight = 72 + shown.length * 8;
  const open = coordinatesOpen(c);
  const picked = selected
    ? cells.find((cell) => `${cell.product}\u0000${cell.channel}` === selected)
    : undefined;
  const name = c.confirmedByName || c.confirmedBy;

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

        <View testID="coordinates-changes" className="bg-muted mx-4 mb-4 gap-2 rounded-2xl p-4">
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
                c.changes.map((ch, i) => (
                  <ChangeLine
                    key={`${ch.kind}-${ch.collection ?? ch.locale}-${i}`}
                    change={ch}
                    lang={lang}
                  />
                ))
              )}
            </>
          )}
        </View>

        {cells.length === 0 ? (
          <Text className="text-muted-foreground px-5 text-sm">{t("plan.coordinates.empty")}</Text>
        ) : (
          <View className="flex-row px-3">
            <View style={{ width: LABEL_WIDTH }}>
              <View style={{ height: 28 }} />
              {grid.products.map((p) => (
                <View key={p} style={{ height: rowHeight + 8 }} className="justify-center pr-2">
                  <Text numberOfLines={2} className="text-foreground text-sm font-semibold">
                    {p || t("plan.coordinates.noProduct")}
                  </Text>
                </View>
              ))}
            </View>
            <ScrollView testID="coordinates-grid" horizontal showsHorizontalScrollIndicator>
              <View>
                <View className="flex-row" style={{ height: 28 }}>
                  {grid.channels.map((ch) => (
                    <Text
                      key={ch}
                      numberOfLines={1}
                      style={{ width: CELL_WIDTH }}
                      className="text-muted-foreground px-2 text-xs font-semibold uppercase"
                    >
                      {ch || t("plan.coordinates.noChannel")}
                    </Text>
                  ))}
                </View>
                {grid.products.map((p) => (
                  <View key={p} className="flex-row">
                    {grid.channels.map((ch) => {
                      const cell = grid.at(p, ch);
                      const key = `${p}\u0000${ch}`;
                      return cell ? (
                        <Cell
                          key={ch}
                          cell={cell}
                          languages={shown}
                          height={rowHeight}
                          selected={selected === key}
                          onPress={() => setSelected(selected === key ? null : key)}
                        />
                      ) : (
                        <View
                          key={ch}
                          style={{ width: CELL_WIDTH - 8, height: rowHeight }}
                          className="border-border m-1 rounded-2xl border border-dashed opacity-40"
                        />
                      );
                    })}
                  </View>
                ))}
              </View>
            </ScrollView>
          </View>
        )}
        {cells.length > 0 ? (
          <Text className="text-muted-foreground px-5 pt-2 text-xs leading-4">
            {shown.length > 1
              ? `${t("plan.coordinates.legend")} ${t("plan.coordinates.barOrder", { languages: shown.map(lang).join(", ") })}`
              : t("plan.coordinates.legend")}
          </Text>
        ) : null}

        {picked ? (
          <View
            testID="coordinates-cell-detail"
            className="bg-card border-border mx-4 mt-4 gap-3 rounded-2xl border p-4"
          >
            <Text className="text-foreground text-base font-semibold">
              {t("plan.coordinates.cellTitle", {
                product: picked.product || t("plan.coordinates.noProduct"),
                channel: picked.channel || t("plan.coordinates.noChannel"),
              })}
            </Text>
            {picked.collections.map((col) => (
              <View key={col.name} className="gap-1">
                <Text className="text-foreground text-sm font-semibold">{col.name}</Text>
                <Text className="text-muted-foreground text-xs">
                  {[
                    t("plan.coordinates.blocks", { count: col.blocks }),
                    col.voice ?? t("plan.coordinates.noVoice"),
                    col.sourceOnly ? t("plan.coordinates.sourceOnly") : null,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </Text>
                {col.sourceOnly
                  ? null
                  : shown.map((l) => (
                      <View key={l} className="flex-row items-center gap-2">
                        <Text className="text-muted-foreground w-28 text-xs" numberOfLines={1}>
                          {lang(l)}
                        </Text>
                        <View className="flex-1">
                          <CoverageBar share={col.coverage[l] ?? 0} />
                        </View>
                        <Text className="text-muted-foreground w-10 text-right text-xs">
                          {Math.round((col.coverage[l] ?? 0) * 100)}%
                        </Text>
                      </View>
                    ))}
              </View>
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
            className="bg-primary mx-4 mt-6 items-center rounded-full px-4 py-3"
          >
            <Text className="text-primary-foreground text-base font-semibold">
              {t("plan.coordinates.confirmChanges")}
            </Text>
          </Pressable>
        ) : null}
      </ScrollView>
    </View>
  );
}
