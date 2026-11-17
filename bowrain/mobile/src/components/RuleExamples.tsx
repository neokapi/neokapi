// Where a voice rule matches the project's own text: up to three excerpts
// with the matching words marked, so a reviewer judges the rule by what it
// catches rather than by its regex.
import type { ContextCard as Card } from "@neokapi/bowrain-client";
import { useQuery } from "@tanstack/react-query";
import { Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { useAuth } from "../lib/auth";

export function RuleExamples({ card, ws, project }: { card: Card; ws: string; project: string }) {
  const { t } = useTranslation();
  const { client } = useAuth();
  const profile = card.item.profile ?? "";
  const required = card.pattern?.list === "required";
  const q = useQuery({
    queryKey: ["ruleExamples", ws, project, profile, card.item.id],
    queryFn: () => client.contextRuleExamples(ws, project, { profile, id: card.item.id }),
    enabled: profile !== "",
    staleTime: 5 * 60 * 1000,
  });
  if (!profile || q.isError) return null;

  let body;
  if (q.isPending) {
    body = <Text className="text-muted-foreground text-sm">{t("context.pattern.looking")}</Text>;
  } else if (q.data.examples.length === 0) {
    body = (
      <Text testID="rule-examples-none" className="text-muted-foreground text-sm">
        {q.data.complete
          ? t("context.pattern.noMatches")
          : t("context.pattern.noMatchesYet", { count: q.data.scanned })}
      </Text>
    );
  } else {
    body = q.data.examples.map((ex, i) => (
      <View key={i} testID="rule-example" className="bg-muted gap-1 rounded-xl px-3 py-2">
        <Text className="text-foreground text-base">
          {ex.before}
          <Text className={`${required ? "bg-added" : "bg-removed"} font-semibold`}>
            {ex.match}
          </Text>
          {ex.after}
        </Text>
        {ex.item ? (
          <Text className="text-muted-foreground text-xs" numberOfLines={1}>
            {ex.item}
          </Text>
        ) : null}
      </View>
    ));
  }
  return (
    <View className="gap-2">
      <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
        {t("context.pattern.inYourContent")}
      </Text>
      {body}
    </View>
  );
}
