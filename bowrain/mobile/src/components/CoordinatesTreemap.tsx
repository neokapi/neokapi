// The coordinates treemap: the products as groups and their channels as
// tiles. A tile's area is its block count, its shade the share established in
// the languages shown, and its outline whether it changed since the last
// confirmation (green for a new point, amber for a changed one). Tapping a
// product zooms into it; tapping a channel there opens its collections.
// Plain views with absolute positions; the layout is src/lib/treemap.ts.
import type { CoordinateCell } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useMemo } from "react";
import { Modal, Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { inset, mixHex, shadeStep, squarify, type Rect } from "../lib/treemap";
import { SafeAreaView } from "./SafeArea";

/** How far from paper to accent each shade step sits; the last steps take the light ink. */
const SHADES = [0, 0.2, 0.4, 0.65, 0.9];
const INK_FROM_STEP = 3;
/** The smallest share of the map a tile gets, so a tiny channel stays tappable. */
const MIN_SHARE = 0.005;
const PRODUCT_HEADER = 22;

export interface MapColors {
  paper: string;
  accent: string;
  muted: string;
  ink: string;
  inkOnAccent: string;
  added: string;
  changed: string;
}

const FALLBACK: MapColors = {
  paper: "#ffffff",
  accent: "#0060c1",
  muted: "#ecf1f5",
  ink: "#192029",
  inkOnAccent: "#f8fafc",
  added: "#15803d",
  changed: "#92400e",
};

/** The theme's colours as hex, for the shades computed in JS. */
export function useMapColors(): MapColors {
  const v = useCSSVariable([
    "--color-card",
    "--color-primary",
    "--color-muted",
    "--color-foreground",
    "--color-primary-foreground",
    "--color-approve",
    "--color-warning-foreground",
  ]);
  const at = (i: number, fb: string) => (typeof v[i] === "string" ? (v[i] as string) : fb);
  return {
    paper: at(0, FALLBACK.paper),
    accent: at(1, FALLBACK.accent),
    muted: at(2, FALLBACK.muted),
    ink: at(3, FALLBACK.ink),
    inkOnAccent: at(4, FALLBACK.inkOnAccent),
    added: at(5, FALLBACK.added),
    changed: at(6, FALLBACK.changed),
  };
}

/**
 * The share of a cell established in the languages shown. It takes the lowest
 * of them rather than the average: a channel reads as done only once every
 * language shown is, so the language furthest behind decides the shade.
 */
export function cellCoverage(cell: { coverage: Record<string, number> }, languages: string[]) {
  if (languages.length === 0) return 0;
  return Math.min(...languages.map((l) => cell.coverage[l] ?? 0));
}

export function shadeOf(colors: MapColors, share: number): { fill: string; ink: string } {
  const step = shadeStep(share, SHADES.length);
  return {
    fill: mixHex(colors.paper, colors.accent, SHADES[step]),
    ink: step >= INK_FROM_STEP ? colors.inkOnAccent : colors.ink,
  };
}

/** The scale under the map: one swatch per shade step. */
export function shadeSwatches(colors: MapColors): string[] {
  return SHADES.map((s) => mixHex(colors.paper, colors.accent, s));
}

function outlineOf(cell: CoordinateCell, colors: MapColors): string | undefined {
  if (cell.changes.includes("point_added")) return colors.added;
  if (cell.changes.length > 0) return colors.changed;
  return undefined;
}

interface Group {
  product: string;
  cells: CoordinateCell[];
  blocks: number;
}

function groupsOf(cells: CoordinateCell[]): Group[] {
  const by = new Map<string, Group>();
  for (const c of cells) {
    const g = by.get(c.product) ?? { product: c.product, cells: [], blocks: 0 };
    g.cells.push(c);
    g.blocks += c.blocks;
    by.set(c.product, g);
  }
  return [...by.values()].sort((a, b) => a.product.localeCompare(b.product));
}

interface PlacedTile {
  cell: CoordinateCell;
  rect: Rect;
}
interface PlacedHeader {
  group: Group;
  rect: Rect;
}

/** The map's tiles and product headers for the area, zoomed into one product or not. */
export function layoutMap(
  cells: CoordinateCell[],
  area: Rect,
  zoomed: string | null,
): { tiles: PlacedTile[]; headers: PlacedHeader[] } {
  const groups = groupsOf(cells).filter((g) => zoomed === null || g.product === zoomed);
  const total = groups.reduce((s, g) => s + g.blocks, 0);
  const floor = Math.max(1, total * MIN_SHARE);
  const weight = (c: CoordinateCell) => Math.max(c.blocks, floor);
  const tiles: PlacedTile[] = [];
  const headers: PlacedHeader[] = [];
  const placeCells = (g: Group, r: Rect) => {
    for (const t of squarify(g.cells, weight, r)) tiles.push({ cell: t.item, rect: inset(t, 1.5) });
  };
  if (zoomed !== null) {
    if (groups[0]) placeCells(groups[0], area);
    return { tiles, headers };
  }
  const groupWeight = (g: Group) => g.cells.reduce((s, c) => s + weight(c), 0);
  for (const t of squarify(groups, groupWeight, area)) {
    const r = inset(t, 2);
    const header = r.h >= 3 * PRODUCT_HEADER && r.w >= 56;
    if (header) headers.push({ group: t.item, rect: { ...r, h: PRODUCT_HEADER } });
    placeCells(
      t.item,
      header ? { x: r.x, y: r.y + PRODUCT_HEADER, w: r.w, h: r.h - PRODUCT_HEADER } : r,
    );
  }
  return { tiles, headers };
}

export function CoordinatesTreemap({
  cells,
  languages,
  width,
  height,
  zoomed,
  onZoom,
  onOpen,
}: {
  cells: CoordinateCell[];
  languages: string[];
  width: number;
  height: number;
  /** The product zoomed into, or null for the whole project. */
  zoomed: string | null;
  onZoom: (product: string) => void;
  onOpen: (cell: CoordinateCell) => void;
}) {
  const { t, i18n } = useTranslation();
  const colors = useMapColors();
  const num = useMemo(() => new Intl.NumberFormat(i18n.language), [i18n.language]);
  const { tiles, headers } = useMemo(
    () => layoutMap(cells, { x: 0, y: 0, w: width, h: height }, zoomed),
    [cells, width, height, zoomed],
  );
  const blocks = (count: number) => t("plan.coordinates.blocks", { count });

  return (
    <View testID="coordinates-map" style={{ width, height }}>
      {headers.map(({ group, rect }) => {
        const name = group.product || t("plan.coordinates.noProduct");
        return (
          <Pressable
            key={`h-${group.product}`}
            testID={`coordinates-product-${group.product}`}
            accessibilityRole="button"
            accessibilityLabel={t("plan.coordinates.productLabel", {
              name,
              blocks: blocks(group.blocks),
            })}
            onPress={() => onZoom(group.product)}
            style={{
              position: "absolute",
              left: rect.x,
              top: rect.y,
              width: rect.w,
              height: rect.h,
            }}
            className="flex-row items-center gap-1 px-1"
          >
            <Text numberOfLines={1} className="text-foreground shrink text-xs font-semibold">
              {name}
            </Text>
            <Text numberOfLines={1} className="text-muted-foreground text-[11px]">
              {num.format(group.blocks)}
            </Text>
          </Pressable>
        );
      })}
      {tiles.map(({ cell, rect }) => {
        const name = cell.channel || t("plan.coordinates.noChannel");
        const share = cellCoverage(cell, languages);
        const shade = cell.sourceOnly
          ? { fill: colors.muted, ink: colors.ink }
          : shadeOf(colors, share);
        const outline = outlineOf(cell, colors);
        const label = [
          t("plan.coordinates.tileLabel", {
            name: zoomed === null && cell.product ? `${cell.product} ${name}` : name,
            blocks: blocks(cell.blocks),
            coverage: cell.sourceOnly
              ? t("plan.coordinates.sourceOnly")
              : t("plan.coordinates.established", { percent: Math.round(share * 100) }),
          }),
          outline === colors.added
            ? t("plan.coordinates.outlineNew")
            : outline
              ? t("plan.coordinates.outlineChanged")
              : null,
        ]
          .filter(Boolean)
          .join(", ");
        const full = rect.w >= 64 && rect.h >= 38;
        const nameOnly = !full && rect.w >= 36 && rect.h >= 18;
        return (
          <Pressable
            key={`${cell.product}\u0000${cell.channel}`}
            testID={`coordinates-tile-${cell.product}-${cell.channel}`}
            accessibilityRole="button"
            accessibilityLabel={label}
            onPress={() => (zoomed === null ? onZoom(cell.product) : onOpen(cell))}
            style={{
              position: "absolute",
              left: rect.x,
              top: rect.y,
              width: rect.w,
              height: rect.h,
              backgroundColor: shade.fill,
              borderColor: outline,
              borderWidth: outline ? 2 : 0,
              borderRadius: 4,
              padding: full || nameOnly ? 4 : 0,
              overflow: "hidden",
            }}
          >
            {full || nameOnly ? (
              <Text
                numberOfLines={1}
                style={{ color: shade.ink }}
                className="text-xs font-semibold"
              >
                {name}
              </Text>
            ) : null}
            {full ? (
              <Text
                numberOfLines={1}
                style={{ color: shade.ink }}
                className="text-[11px] opacity-80"
              >
                {num.format(cell.blocks)}
              </Text>
            ) : null}
          </Pressable>
        );
      })}
    </View>
  );
}

/** The sheet for one tile: its collections, voices, and coverage per language. */
export function CoordinateCellSheet({
  cell,
  languages,
  languageLabel,
  onClose,
}: {
  cell: CoordinateCell | null;
  languages: string[];
  languageLabel: (l: string) => string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Modal
      visible={!!cell}
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      {cell ? (
        <SafeAreaView edges={["bottom"]} className="bg-background flex-1">
          <View className="flex-row items-center justify-between px-5 pt-5 pb-3">
            <Text className="text-foreground flex-1 text-lg font-semibold">
              {t("plan.coordinates.cellTitle", {
                product: cell.product || t("plan.coordinates.noProduct"),
                channel: cell.channel || t("plan.coordinates.noChannel"),
              })}
            </Text>
            <Pressable
              testID="coordinates-cell-close"
              accessibilityRole="button"
              onPress={onClose}
              hitSlop={8}
            >
              <Text className="text-primary text-base font-semibold">{t("common.done")}</Text>
            </Pressable>
          </View>
          <ScrollView testID="coordinates-cell-detail" contentContainerClassName="gap-3 px-4 pb-8">
            {cell.collections.map((col) => (
              <View key={col.name} className="bg-card border-border gap-1.5 rounded-2xl border p-4">
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
                  : languages.map((l) => {
                      const pct = Math.round((col.coverage[l] ?? 0) * 100);
                      return (
                        <View key={l} className="flex-row items-center gap-2">
                          <Text className="text-muted-foreground w-28 text-xs" numberOfLines={1}>
                            {languageLabel(l)}
                          </Text>
                          <View className="bg-muted h-1 flex-1 overflow-hidden rounded-full">
                            <View className="bg-primary h-1" style={{ width: `${pct}%` }} />
                          </View>
                          <Text className="text-foreground w-10 text-right text-xs">{pct}%</Text>
                        </View>
                      );
                    })}
              </View>
            ))}
          </ScrollView>
        </SafeAreaView>
      ) : null}
    </Modal>
  );
}

export function ZoomOut({ product, onPress }: { product: string; onPress: () => void }) {
  const { t } = useTranslation();
  const [primary] = useCSSVariable(["--color-primary"]);
  return (
    <Pressable
      testID="coordinates-zoom-out"
      accessibilityRole="button"
      onPress={onPress}
      className="flex-row items-center gap-1 py-1"
    >
      <Ionicons
        name="chevron-back"
        size={16}
        color={typeof primary === "string" ? primary : undefined}
      />
      <Text className="text-primary text-sm font-semibold">
        {t("plan.coordinates.allProducts")}
      </Text>
      <Text className="text-muted-foreground text-sm">
        {" "}
        · {product || t("plan.coordinates.noProduct")}
      </Text>
    </Pressable>
  );
}
