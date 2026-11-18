// A block in its container, laid out the way a reader meets it: a React
// component's strings in order, with the heading as a heading and the button
// as a button; a document's section from its heading down. The block the card
// stood for is marked, the other translations waiting for review carry a dot,
// and any of them can be approved or fixed in place. The server picks the
// container and each unit's role (GET …/blocks/:ref/:bid/container); the app
// only draws what it is given.
import {
  cardOfUnit,
  type ContainerUnit,
  type DeckCard,
  type ProjectScope,
  type ReviewContainer,
} from "@neokapi/bowrain-client";
import { useQuery } from "@tanstack/react-query";
import { useRef, useState, type ComponentRef } from "react";
import { ActivityIndicator, Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { fileName } from "../lib/where";
import { RunText } from "./RunText";

/** The query key of one block's container in one language. */
export function reviewContainerKey(scope: ProjectScope, blockId: string, locale: string) {
  return ["review-container", scope.workspace, scope.project, blockId, locale] as const;
}

export type LoadContainer = (card: DeckCard) => Promise<ReviewContainer>;

/** Which side of each unit the page reads in. */
export type Reading = "translation" | "source" | "both";

export interface InContextViewProps {
  card: DeckCard;
  scope: ProjectScope;
  load: LoadContainer;
  /** Approve a unit's translation, through the deck's decision queue. */
  onApprove: (card: DeckCard) => void;
  /** Fix or reject a unit: the deck opens its fix sheet for it. */
  onFix: (card: DeckCard) => void;
  /** Blocks decided in this session, so they read as decided. */
  decided?: ReadonlySet<string>;
}

export function InContextView({
  card,
  scope,
  load,
  onApprove,
  onFix,
  decided,
}: InContextViewProps) {
  const { t } = useTranslation();
  const [reading, setReading] = useState<Reading>("translation");
  const [selected, setSelected] = useState<string>(card.blockId);
  const [approved, setApproved] = useState<ReadonlySet<string>>(new Set());
  const scroll = useRef<ComponentRef<typeof ScrollView>>(null);
  const placed = useRef(false);

  const ctx = useQuery({
    queryKey: reviewContainerKey(scope, card.blockId, card.locale),
    queryFn: () => load(card),
    staleTime: 60_000,
  });

  if (ctx.isPending) {
    return (
      <View className="flex-1 items-center justify-center">
        <ActivityIndicator />
      </View>
    );
  }
  if (ctx.isError || !ctx.data) {
    return (
      <View className="items-start gap-2 p-4">
        <Text className="text-muted-foreground text-sm">{t("common.loadFailed")}</Text>
        <Pressable accessibilityRole="button" onPress={() => void ctx.refetch()}>
          <Text className="text-primary text-sm">{t("common.retry")}</Text>
        </Pressable>
      </View>
    );
  }

  const c = ctx.data;
  const isDone = (u: ContainerUnit) =>
    approved.has(u.blockId) || (decided?.has(u.blockId) ?? false);
  const waiting = c.units.filter((u) => u.waiting && !isDone(u));
  const approve = (u: ContainerUnit) => {
    onApprove(cardOfUnit(u, card.locale, card.collectionId));
    setApproved((s) => new Set([...s, u.blockId]));
  };

  return (
    <View className="flex-1">
      <View className="border-border gap-2 border-b px-4 pt-2 pb-3">
        <Text
          testID="context-kind"
          className="text-muted-foreground text-xs font-semibold tracking-wide uppercase"
        >
          {t(`inContext.kind.${c.kind}`)}
        </Text>
        <Text
          testID="context-title"
          className="text-foreground text-lg font-semibold"
          numberOfLines={2}
        >
          {c.kind === "window" ? fileName(c.title) : c.title}
        </Text>
        <Text className="text-muted-foreground font-mono text-xs" numberOfLines={1}>
          {fileName(c.item)}
        </Text>
        <ReadingSwitch value={reading} onChange={setReading} />
      </View>

      <ScrollView ref={scroll} contentContainerClassName="gap-3 px-4 py-4 pb-24">
        {c.moreBefore ? <More /> : null}
        {c.units.map((u) => (
          <View
            key={u.blockId}
            onLayout={(e) => {
              // Open at the block the card stood for.
              if (u.blockId === c.focus && !placed.current) {
                placed.current = true;
                scroll.current?.scrollTo({
                  y: Math.max(0, e.nativeEvent.layout.y - 80),
                  animated: false,
                });
              }
            }}
          >
            <Unit
              unit={u}
              reading={reading}
              focus={u.blockId === c.focus}
              selected={u.blockId === selected}
              done={isDone(u)}
              onSelect={() => setSelected(u.blockId)}
              onApprove={() => approve(u)}
              onFix={() => onFix(cardOfUnit(u, card.locale, card.collectionId))}
            />
          </View>
        ))}
        {c.moreAfter ? <More /> : null}
        {waiting.length > 1 ? (
          <Pressable
            testID="context-approve-all"
            accessibilityRole="button"
            onPress={() => waiting.forEach(approve)}
            className="bg-approve/15 mt-2 items-center rounded-xl py-3"
          >
            <Text className="text-foreground text-base font-semibold">
              {t("inContext.approveAll", { count: waiting.length })}
            </Text>
          </Pressable>
        ) : null}
      </ScrollView>
    </View>
  );
}

function More() {
  const { t } = useTranslation();
  return <Text className="text-muted-foreground text-center text-xs">{t("inContext.more")}</Text>;
}

function ReadingSwitch({ value, onChange }: { value: Reading; onChange: (r: Reading) => void }) {
  const { t } = useTranslation();
  const options: Reading[] = ["translation", "source", "both"];
  return (
    <View accessibilityRole="tablist" className="bg-muted flex-row self-start rounded-lg p-0.5">
      {options.map((o) => (
        <Pressable
          key={o}
          testID={`context-reading-${o}`}
          accessibilityRole="tab"
          accessibilityState={{ selected: value === o }}
          onPress={() => onChange(o)}
          className={`rounded-md px-3 py-1 ${value === o ? "bg-background" : ""}`}
        >
          <Text
            className={`text-sm ${value === o ? "text-foreground font-semibold" : "text-muted-foreground"}`}
          >
            {t(`inContext.reading.${o}`)}
          </Text>
        </Pressable>
      ))}
    </View>
  );
}

/** The type size for a unit's role: a heading reads as a heading. */
function sizeOf(u: ContainerUnit): "base" | "lg" | "xl" {
  if (u.role !== "heading") return "base";
  return (u.level ?? 2) <= 2 ? "xl" : "lg";
}

function Unit({
  unit,
  reading,
  focus,
  selected,
  done,
  onSelect,
  onApprove,
  onFix,
}: {
  unit: ContainerUnit;
  reading: Reading;
  focus: boolean;
  selected: boolean;
  done: boolean;
  onSelect: () => void;
  onApprove: () => void;
  onFix: () => void;
}) {
  const { t } = useTranslation();
  const hasTarget = unit.target.length > 0;
  const main = reading === "source" || !hasTarget ? unit.source : unit.target;
  const size = sizeOf(unit);
  const waiting = unit.waiting && !done;

  const body = <RunText runs={main} size={size} muted={reading !== "source" && !hasTarget} />;
  let drawn = body;
  if (unit.role === "button") {
    drawn = (
      <View className="border-border self-start rounded-full border px-4 py-1.5">{body}</View>
    );
  } else if (unit.role === "list_item") {
    drawn = (
      <View className="flex-row gap-2">
        <Text className="text-muted-foreground text-base">•</Text>
        <View className="flex-1">{body}</View>
      </View>
    );
  } else if (unit.role === "code") {
    drawn = <View className="bg-muted rounded-lg px-3 py-2">{body}</View>;
  } else if (unit.role === "label" || unit.role === "caption") {
    drawn = <View className="opacity-80">{body}</View>;
  }

  return (
    <Pressable
      testID={`context-unit-${unit.blockId}`}
      accessibilityRole="button"
      accessibilityState={{ selected }}
      accessibilityHint={waiting ? t("inContext.waiting") : undefined}
      onPress={onSelect}
      className={`gap-2 rounded-xl px-3 py-2 ${focus ? "border-primary border-2" : selected ? "border-border border" : ""}`}
    >
      <View className="flex-row items-start gap-2">
        <View className="pt-2">
          <View
            testID={waiting ? "context-waiting-dot" : done ? "context-done-dot" : undefined}
            className={`h-2 w-2 rounded-full ${waiting ? "bg-corrective" : done ? "bg-approve" : "bg-transparent"}`}
          />
        </View>
        <View className="flex-1 gap-1">
          {drawn}
          {reading === "both" && hasTarget ? (
            <RunText runs={unit.source} size="base" muted />
          ) : null}
        </View>
      </View>
      {selected && waiting ? (
        <View className="flex-row gap-2 pl-4">
          <Pressable
            testID="context-approve"
            accessibilityRole="button"
            onPress={onApprove}
            className="bg-approve/15 rounded-lg px-4 py-2"
          >
            <Text className="text-foreground text-sm font-semibold">{t("inContext.approve")}</Text>
          </Pressable>
          <Pressable
            testID="context-fix"
            accessibilityRole="button"
            onPress={onFix}
            className="bg-muted rounded-lg px-4 py-2"
          >
            <Text className="text-foreground text-sm font-semibold">{t("inContext.fix")}</Text>
          </Pressable>
        </View>
      ) : null}
      {selected && done ? (
        <Text className="text-muted-foreground pl-4 text-xs">{t("inContext.decided")}</Text>
      ) : null}
    </Pressable>
  );
}
