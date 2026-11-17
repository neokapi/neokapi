// The details behind one card: where the text ships, the strings either side
// of it in its file, the voice it is held to, and the closest wording the
// content memory holds. Read from the server's review context for the block
// (GET …/blocks/:ref/:bid/review-context), the same context the web reviewer
// reads, and only when the sheet opens.
import type { DeckCard, ProjectScope, ReviewContext } from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { ActivityIndicator, Modal, Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { markBusy } from "../lib/busy";
import { fileName, humanize } from "../lib/where";
import { RunText } from "./RunText";
import { SafeAreaView } from "./SafeArea";

/** The query key of one block's review context in one language. */
export function reviewContextKey(scope: ProjectScope, blockId: string, locale: string) {
  return ["review-context", scope.workspace, scope.project, blockId, locale] as const;
}

type Load = (card: DeckCard) => Promise<ReviewContext>;

export function ContextSheet({
  card,
  scope,
  where,
  load,
  onClose,
}: {
  card: DeckCard | null;
  scope: ProjectScope;
  where: string[];
  load: Load;
  onClose: () => void;
}) {
  const open = card !== null;
  useEffect(() => {
    markBusy("context-sheet", open);
    return () => markBusy("context-sheet", false);
  }, [open]);
  return (
    <Modal
      visible={open}
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      {card ? (
        <ContextSheetBody card={card} scope={scope} where={where} load={load} onClose={onClose} />
      ) : null}
    </Modal>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <View className="gap-2">
      <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
        {title}
      </Text>
      {children}
    </View>
  );
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <View className="flex-row gap-3">
      <Text className="text-muted-foreground w-24 text-sm">{label}</Text>
      <Text
        className={`text-foreground min-w-0 flex-1 text-sm ${mono ? "font-mono" : ""}`}
        selectable
      >
        {value}
      </Text>
    </View>
  );
}

/**
 * The review context's runs as the app draws them. The contract and the
 * format package generate the same Run from one schema, and differ only in
 * whether a constraint flag may be absent.
 */
function drawable(runs: unknown): Run[] {
  return (runs ?? []) as Run[];
}

/** Two run sequences compare whole: codes and plurals count, not only the text. */
function sameRuns(a: readonly Run[] | undefined, b: readonly Run[] | undefined): boolean {
  return JSON.stringify(a ?? []) === JSON.stringify(b ?? []);
}

function Neighbour({ source, same, testID }: { source: Run[]; same: boolean; testID: string }) {
  const { t } = useTranslation();
  return (
    <View testID={testID} className="bg-muted gap-1 rounded-xl px-3 py-2">
      <RunText runs={source} size="base" muted />
      {same ? (
        <Text className="text-foreground text-xs font-semibold">{t("details.sameText")}</Text>
      ) : null}
    </View>
  );
}

export function ContextSheetBody({
  card,
  scope,
  where,
  load,
  onClose,
}: {
  card: DeckCard;
  scope: ProjectScope;
  where: string[];
  load: Load;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const ctx = useQuery({
    queryKey: reviewContextKey(scope, card.blockId, card.locale),
    queryFn: () => load(card),
    staleTime: 5 * 60_000,
  });
  const point = ctx.data?.point;
  const hood = ctx.data?.neighbourhood;
  const match = ctx.data?.history?.match;
  const before = hood?.before ?? [];
  const after = hood?.after ?? [];
  const extraAxes = Object.entries(point?.coordinates ?? {}).filter(
    ([axis]) => axis !== "product" && axis !== "channel",
  );

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      <View className="border-border flex-row items-center justify-between border-b px-4 py-3">
        <Text className="text-foreground text-lg font-semibold">{t("details.title")}</Text>
        <Pressable testID="details-close" accessibilityRole="button" onPress={onClose} hitSlop={12}>
          <Text className="text-primary text-base">{t("common.close")}</Text>
        </Pressable>
      </View>
      <ScrollView contentContainerClassName="gap-6 p-4 pb-12">
        <Section title={t("details.where")}>
          {where.length > 0 ? (
            <Text className="text-foreground text-base font-semibold">{where.join(" · ")}</Text>
          ) : null}
          {point?.collection ? (
            <Row label={t("details.collection")} value={humanize(point.collection)} />
          ) : null}
          {extraAxes.map(([axis, value]) => (
            <Row key={axis} label={humanize(axis)} value={value} />
          ))}
          <Row label={t("details.file")} value={card.item} mono />
          {card.name ? <Row label={t("details.key")} value={card.name} mono /> : null}
        </Section>

        {card.copies && card.copies.length > 0 ? (
          <Section title={t("details.copies")}>
            {card.copies.map((c) => (
              <View key={c.blockId} testID="details-copy" className="gap-0.5">
                <Text className="text-foreground text-sm font-medium">{fileName(c.item)}</Text>
                {c.name ? (
                  <Text className="text-muted-foreground font-mono text-xs" selectable>
                    {c.name}
                  </Text>
                ) : null}
              </View>
            ))}
          </Section>
        ) : null}

        {ctx.isPending ? (
          <ActivityIndicator />
        ) : ctx.isError ? (
          <View className="items-start gap-2">
            <Text className="text-muted-foreground text-sm">{t("common.loadFailed")}</Text>
            <Pressable accessibilityRole="button" onPress={() => void ctx.refetch()}>
              <Text className="text-primary text-sm">{t("common.retry")}</Text>
            </Pressable>
          </View>
        ) : (
          <>
            {before.length + after.length > 0 ? (
              <Section title={t("details.around")}>
                {before.map((n, i) => (
                  <Neighbour
                    key={n.key ?? `b${i}`}
                    testID={`details-before-${i}`}
                    source={drawable(n.source)}
                    same={sameRuns(drawable(n.source), card.source)}
                  />
                ))}
                <View
                  testID="details-this"
                  className="border-primary gap-1 rounded-xl border px-3 py-2"
                >
                  <Text className="text-primary text-xs font-semibold">
                    {t("details.thisText")}
                  </Text>
                  <RunText runs={card.source} size="base" />
                </View>
                {after.map((n, i) => (
                  <Neighbour
                    key={n.key ?? `a${i}`}
                    testID={`details-after-${i}`}
                    source={drawable(n.source)}
                    same={sameRuns(drawable(n.source), card.source)}
                  />
                ))}
              </Section>
            ) : null}

            {point?.voice ? (
              <Section title={t("details.voice")}>
                <Text className="text-foreground text-sm">{point.voice.name}</Text>
              </Section>
            ) : null}

            {match ? (
              <Section title={t("details.memory")}>
                <Text className="text-muted-foreground text-xs">
                  {t("details.memoryScore", { score: Math.round(match.score) })}
                </Text>
                {match.target_runs && match.target_runs.length > 0 ? (
                  <RunText runs={drawable(match.target_runs)} size="base" />
                ) : (
                  <Text className="text-foreground text-sm">{match.target}</Text>
                )}
              </Section>
            ) : null}
          </>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}
