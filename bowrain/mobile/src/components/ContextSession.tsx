// The frame of a context session: the "since the last session" card that
// opens it and the summary that closes it. Both show what the server
// counted; a figure the server leaves out is not drawn.
import type { ContextSessionEnd, ContextSessionSince } from "@neokapi/bowrain-client";
import { Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

/**
 * What changed since the team's last session: the terms scans found, the
 * terms whose use grew, and how many cards other people decided, which the
 * feed shows.
 */
export function SinceCard({ since, onStart }: { since: ContextSessionSince; onStart: () => void }) {
  const { t, i18n } = useTranslation();
  const date = new Date(since.at).toLocaleDateString(i18n.language);
  return (
    <View testID="context-since" className="bg-card border-border flex-1 rounded-[28px] border">
      <ScrollView contentContainerClassName="gap-5 p-5">
        <View className="gap-1">
          <Text accessibilityRole="header" className="text-foreground text-2xl font-semibold">
            {t("context.inSession.since.title")}
          </Text>
          <Text className="text-muted-foreground text-sm">
            {since.by
              ? t("context.inSession.since.endedBy", { name: since.by, date })
              : t("context.inSession.since.ended", { date })}
          </Text>
        </View>
        {since.added_total > 0 ? (
          <View className="gap-1" testID="context-since-added">
            <Text className="text-foreground text-base font-semibold">
              {t("context.inSession.since.added", { count: since.added_total })}
            </Text>
            {since.added.map((a) => (
              <Text key={a.item.id} className="text-foreground text-sm">
                {t("context.inSession.since.addedItem", { term: a.term, count: a.blocks })}
              </Text>
            ))}
          </View>
        ) : null}
        {since.grew_total > 0 ? (
          <View className="gap-1" testID="context-since-grew">
            <Text className="text-foreground text-base font-semibold">
              {t("context.inSession.since.grew", { count: since.grew_total })}
            </Text>
            {since.grew.map((g) => (
              <Text key={g.item.id} className="text-foreground text-sm">
                {t("context.inSession.since.grewItem", {
                  term: g.term,
                  before: g.before ?? 0,
                  count: g.blocks,
                })}
              </Text>
            ))}
          </View>
        ) : null}
        {since.decided_by_others > 0 ? (
          <Text testID="context-since-others" className="text-muted-foreground text-sm">
            {t("context.inSession.since.others", { count: since.decided_by_others })}
          </Text>
        ) : null}
      </ScrollView>
      <Pressable
        testID="context-since-start"
        accessibilityRole="button"
        onPress={onStart}
        className="bg-primary m-5 items-center rounded-full py-3 active:opacity-80"
      >
        <Text className="text-primary-foreground text-base font-semibold">
          {t("context.inSession.start")}
        </Text>
      </Pressable>
    </View>
  );
}

/**
 * What the session moved: the decisions by outcome, the blocks that now
 * follow the terms kept, the source findings cleared, and what is left.
 * A figure the server leaves out is not shown.
 */
export function Summary({
  end,
  onMore,
  onStop,
}: {
  end: ContextSessionEnd;
  onMore: () => void;
  onStop: () => void;
}) {
  const { t } = useTranslation();
  const s = end.summary;
  const outcomes = [
    s.kept > 0 ? t("context.inSession.summary.kept", { count: s.kept }) : null,
    s.changed > 0 ? t("context.inSession.summary.changed", { count: s.changed }) : null,
    s.rejected > 0 ? t("context.inSession.summary.rejected", { count: s.rejected }) : null,
  ].filter((x): x is string => !!x);
  return (
    <View testID="context-summary" className="flex-1 justify-between gap-6">
      <ScrollView contentContainerClassName="gap-5">
        <Text accessibilityRole="header" className="text-foreground text-2xl font-semibold">
          {t("context.inSession.summary.title")}
        </Text>
        <View className="gap-1">
          <Text testID="context-summary-decided" className="text-foreground text-lg">
            {s.decided > 0
              ? t("context.inSession.summary.decided", { count: s.decided })
              : t("context.inSession.summary.none")}
          </Text>
          {outcomes.length > 0 ? (
            <Text testID="context-summary-outcomes" className="text-muted-foreground text-sm">
              {outcomes.join(" · ")}
            </Text>
          ) : null}
        </View>
        {s.blocks !== undefined ? (
          <Text testID="context-summary-blocks" className="text-foreground text-base">
            {t("context.inSession.summary.blocks", { count: s.blocks })}
          </Text>
        ) : null}
        {s.findings_cleared !== undefined ? (
          <Text testID="context-summary-findings" className="text-foreground text-base">
            {t("context.inSession.summary.findings", { count: s.findings_cleared })}
          </Text>
        ) : null}
        <Text testID="context-summary-left" className="text-muted-foreground text-sm">
          {s.waiting > 0
            ? [
                t("context.inSession.summary.left", { count: s.waiting }),
                s.next ? t("context.inSession.summary.next", { term: s.next }) : null,
              ]
                .filter(Boolean)
                .join(" · ")
            : t("context.inSession.summary.leftNone")}
        </Text>
      </ScrollView>
      <View className="flex-row gap-3">
        <Pressable
          testID="context-summary-stop"
          accessibilityRole="button"
          onPress={onStop}
          className="bg-muted flex-1 items-center rounded-full py-3 active:opacity-80"
        >
          <Text className="text-foreground text-base font-semibold">
            {t("context.inSession.summary.stop")}
          </Text>
        </Pressable>
        {s.waiting > 0 ? (
          <Pressable
            testID="context-summary-more"
            accessibilityRole="button"
            onPress={onMore}
            className="bg-primary flex-1 items-center rounded-full py-3 active:opacity-80"
          >
            <Text className="text-primary-foreground text-base font-semibold">
              {t("context.inSession.summary.more")}
            </Text>
          </Pressable>
        ) : null}
      </View>
    </View>
  );
}
