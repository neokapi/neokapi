// One context item as a card in the context deck: its kind, language and
// status as chips, the rule, term, voice rule or note itself, where it
// applies, and who wrote it. The server decides every field; the card only
// lays them out.
import type { ContextCard as Card } from "@neokapi/bowrain-client";
import { ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { Avatar } from "./feed/Avatar";
import { CARD_SHADOW } from "./ReviewCard";

function Chip({ label, accent, testID }: { label: string; accent?: boolean; testID?: string }) {
  return (
    <View testID={testID} className={`rounded-full px-3 py-1 ${accent ? "bg-chip" : "bg-muted"}`}>
      <Text
        className={`text-xs font-semibold ${accent ? "text-chip-foreground" : "text-muted-foreground"}`}
      >
        {label}
      </Text>
    </View>
  );
}

function Label({ children }: { children: string }) {
  return (
    <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
      {children}
    </Text>
  );
}

/** The item itself, in the shape its kind takes. */
function Body({ card }: { card: Card }) {
  const { t } = useTranslation();
  if (card.term) {
    const avoid = [card.term.term, ...(card.term.forms ?? [])];
    return (
      <View className="gap-3">
        {card.term.replacement ? (
          <View className="gap-1">
            <Label>{t("context.use")}</Label>
            <Text testID="context-use" className="text-foreground text-2xl font-semibold">
              {card.term.replacement}
            </Text>
          </View>
        ) : null}
        <View className="gap-1">
          <Label>{t("context.avoid")}</Label>
          <Text testID="context-avoid" className="text-foreground text-lg">
            {avoid.join(", ")}
          </Text>
        </View>
        {card.text ? <Text className="text-muted-foreground text-sm">{card.text}</Text> : null}
      </View>
    );
  }
  if (card.terms) {
    return (
      <View className="gap-2">
        {card.terms.map((term, i) => (
          <View
            key={`${term.locale}:${term.text}:${i}`}
            className="flex-row items-baseline gap-3"
            testID="context-term"
          >
            <Text className="text-muted-foreground w-10 font-mono text-xs uppercase">
              {term.locale}
            </Text>
            <Text
              className={`text-foreground flex-1 ${i === 0 ? "text-2xl font-semibold" : "text-lg"}`}
            >
              {term.text}
            </Text>
            {term.status ? (
              <Text className="text-muted-foreground text-xs">
                {t(`context.termStatus.${term.status}`, { defaultValue: term.status })}
              </Text>
            ) : null}
          </View>
        ))}
        {card.text ? <Text className="text-muted-foreground text-sm">{card.text}</Text> : null}
      </View>
    );
  }
  if (card.pattern) {
    return (
      <View className="gap-3">
        <View className="gap-1">
          <Label>{t(`context.list.${card.pattern.list}`)}</Label>
          <Text testID="context-pattern" className="text-foreground font-mono text-lg">
            {card.pattern.regex}
          </Text>
        </View>
        {card.pattern.description ? (
          <Text className="text-foreground text-base">{card.pattern.description}</Text>
        ) : null}
        {card.pattern.rate ? (
          <Text className="text-muted-foreground text-sm">
            {t("context.rate", {
              max: card.pattern.rate.split("/")[0],
              per: card.pattern.rate.split("/")[1],
            })}
          </Text>
        ) : null}
      </View>
    );
  }
  return (
    <Text testID="context-note" className="text-foreground text-xl">
      {card.text}
    </Text>
  );
}

export function ContextCard({ card }: { card: Card }) {
  const { t, i18n } = useTranslation();
  const where = [
    t(`context.where.${card.where.level}`),
    ...Object.entries(card.where.coordinates ?? {}).map(([axis, value]) => `${axis}: ${value}`),
  ];
  const date = (iso?: string) => (iso ? new Date(iso).toLocaleDateString(i18n.language) : "");
  if (card.where.valid_from)
    where.push(t("context.validFrom", { date: date(card.where.valid_from) }));
  if (card.where.valid_to) where.push(t("context.validTo", { date: date(card.where.valid_to) }));

  const author = card.author;
  const name = author.name || t("context.unknownAuthor");
  const byline = [
    author.agent_authored ? t("context.agent") : null,
    author.session ? t("context.session", { session: author.session.slice(0, 8) }) : null,
    author.host ? t("context.host", { host: author.host }) : null,
    card.at ? date(card.at) : null,
  ].filter((x): x is string => !!x);

  return (
    <View
      testID="context-card"
      className="bg-card border-border flex-1 rounded-[28px] border"
      style={{ boxShadow: CARD_SHADOW }}
    >
      <View className="flex-row flex-wrap items-center gap-2 px-5 pt-5">
        <Chip label={t(`context.kind.${card.kind}`)} accent testID="context-kind" />
        {card.language ? <Chip label={card.language.toUpperCase()} /> : null}
        {card.status && card.status !== "established" ? (
          <Chip label={t(`context.status.${card.status}`)} testID="context-status" />
        ) : null}
        {(card.flags ?? []).map((f) => (
          <Chip key={f} label={t(`context.flag.${f}`, { defaultValue: f })} />
        ))}
      </View>

      <ScrollView contentContainerClassName="gap-5 p-5">
        <Body card={card} />

        <View className="gap-1">
          <Label>{t("context.appliesIn")}</Label>
          <Text testID="context-where" className="text-foreground text-sm">
            {where.join(" · ")}
          </Text>
        </View>

        {card.review ? (
          <View className="bg-muted rounded-xl px-3 py-2">
            <Text className="text-foreground text-sm">
              {t("context.changedSinceReview", { name: card.review.by.name || name })}
            </Text>
          </View>
        ) : null}
        {card.own_work && !card.actions.includes("keep") ? (
          <View className="bg-warning rounded-xl px-3 py-2">
            <Text className="text-warning-foreground text-sm">{t("context.ownWork")}</Text>
          </View>
        ) : null}
      </ScrollView>

      <View
        testID="context-author"
        className="border-border flex-row items-center gap-3 border-t px-5 py-3"
      >
        <Avatar
          actor={{ id: name, kind: author.agent_authored ? "agent" : author.kind, name }}
          name={name}
          size={28}
        />
        <View className="min-w-0 flex-1">
          <Text className="text-foreground text-sm font-medium" numberOfLines={1}>
            {name}
          </Text>
          {byline.length > 0 ? (
            <Text className="text-muted-foreground text-xs" numberOfLines={1}>
              {byline.join(" · ")}
            </Text>
          ) : null}
        </View>
      </View>
    </View>
  );
}
