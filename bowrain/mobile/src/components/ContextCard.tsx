// One context item as a card in the context deck: its kind, language and
// status as chips, the rule, term, proposed translation of a term, voice rule
// or note itself, how many blocks a proposed term reaches, where it applies,
// who wrote it, and the bulk operation that brought it in. The
// server decides every field; the card only lays them out.
import type { ContextCard as Card } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useState } from "react";
import { Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { originLine } from "../lib/contextOrigin";

import { Avatar } from "./feed/Avatar";
import { RegexText } from "./RegexText";
import { CARD_SHADOW } from "./ReviewCard";
import { RuleExamples } from "./RuleExamples";

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

/** The project a card belongs to, which a voice rule's examples are read from. */
export interface CardScope {
  ws: string;
  project: string;
}

/**
 * A voice rule: what it is for in words first, then the words it looks for and
 * where it matches the project's text. The regex itself waits behind a toggle.
 */
function PatternBody({ card, scope }: { card: Card; scope?: CardScope }) {
  const { t } = useTranslation();
  const [showPattern, setShowPattern] = useState(false);
  const muted = useCSSVariable("--color-muted-foreground");
  const pattern = card.pattern!;
  const words = pattern.words ?? [];
  const description = pattern.description?.trim();
  const [max, per] = (pattern.rate ?? "").split("/");
  return (
    <View className="gap-4">
      <View className="gap-1">
        <Label>{t(`context.list.${pattern.list}`)}</Label>
        {description ? (
          <Text
            testID="context-pattern-description"
            className="text-foreground text-xl font-semibold"
          >
            {description}
          </Text>
        ) : words.length > 0 ? null : (
          <RegexText testID="context-pattern" source={pattern.regex} />
        )}
      </View>
      {words.length > 0 ? (
        <View className="gap-2">
          <Label>{t("context.pattern.words")}</Label>
          <View testID="context-pattern-words" className="flex-row flex-wrap gap-2">
            {words.map((w) => (
              <View key={w} className="bg-muted rounded-lg px-2.5 py-1">
                <Text className="text-foreground text-base">{w}</Text>
              </View>
            ))}
          </View>
        </View>
      ) : null}
      {pattern.rate ? (
        <Text className="text-muted-foreground text-sm">{t("context.rate", { max, per })}</Text>
      ) : null}
      {scope ? <RuleExamples card={card} ws={scope.ws} project={scope.project} /> : null}
      {description || words.length > 0 ? (
        <View className="gap-2">
          <Pressable
            testID="context-pattern-toggle"
            accessibilityRole="button"
            accessibilityState={{ expanded: showPattern }}
            onPress={() => setShowPattern((v) => !v)}
            hitSlop={8}
            className="flex-row items-center gap-1 self-start active:opacity-70"
          >
            <Text className="text-primary text-sm font-medium">
              {showPattern ? t("context.pattern.hide") : t("context.pattern.show")}
            </Text>
            <Ionicons
              name={showPattern ? "chevron-up" : "chevron-down"}
              size={14}
              color={String(muted)}
            />
          </Pressable>
          {showPattern ? <RegexText testID="context-pattern" source={pattern.regex} /> : null}
        </View>
      ) : null}
    </View>
  );
}

/** The item itself, in the shape its kind takes. */
function Body({ card, scope }: { card: Card; scope?: CardScope }) {
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
  if (card.translation) {
    return (
      <View className="gap-3">
        <View className="gap-1">
          <Label>{t("context.translation.source")}</Label>
          <Text testID="context-translation-source" className="text-foreground text-lg">
            {card.translation.term}
          </Text>
        </View>
        <View className="gap-1">
          <Label>{t("context.translation.proposed")}</Label>
          <Text
            testID="context-translation-proposed"
            className="text-foreground text-2xl font-semibold"
          >
            {card.translation.proposed}
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
  if (card.pattern) return <PatternBody card={card} scope={scope} />;
  return (
    <Text testID="context-note" className="text-foreground text-xl">
      {card.text}
    </Text>
  );
}

/**
 * How many blocks use a candidate term, and in which collections, as the scan
 * that proposed it counted them.
 */
function Reach({ card }: { card: Card }) {
  const { t } = useTranslation();
  const reach = card.reach;
  return (
    <View className="gap-1" testID="context-reach">
      <Label>
        {reach
          ? t("context.candidate.reach", { count: reach.blocks })
          : t("context.candidate.reachNone")}
      </Label>
      {(reach?.collections ?? []).map((c) => (
        <Text key={c.name} className="text-foreground text-sm" testID="context-reach-collection">
          {t("context.candidate.collection", {
            name: c.name || t("context.candidate.noCollection"),
            count: c.blocks,
          })}
        </Text>
      ))}
    </View>
  );
}

/** Where the item came from, when a bulk operation brought it in. Tapping it opens the origin sheet. */
function OriginLine({ card, onOpen }: { card: Card; onOpen?: () => void }) {
  const { t } = useTranslation();
  const muted = useCSSVariable("--color-muted-foreground");
  if (!card.origin) return null;
  const line = originLine(t, card.origin);
  return (
    <Pressable
      testID="context-origin"
      accessibilityRole="button"
      accessibilityLabel={`${t("context.origin.openLabel")}: ${line}`}
      disabled={!onOpen}
      onPress={onOpen}
      hitSlop={6}
      className="bg-muted flex-row items-center gap-2 rounded-xl px-3 py-2 active:opacity-70"
    >
      <Ionicons name="git-branch-outline" size={14} color={String(muted)} />
      <Text className="text-foreground min-w-0 flex-1 text-sm" numberOfLines={1}>
        {line}
      </Text>
      {onOpen ? <Ionicons name="chevron-forward" size={14} color={String(muted)} /> : null}
    </Pressable>
  );
}

export function ContextCard({
  card,
  scope,
  onOpenOrigin,
}: {
  card: Card;
  /** The card's project. Given, a voice rule shows where it matches the project's text. */
  scope?: CardScope;
  onOpenOrigin?: () => void;
}) {
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
        <Chip
          label={card.proposed ? t("context.candidate.label") : t(`context.kind.${card.kind}`)}
          accent
          testID="context-kind"
        />
        {card.language ? <Chip label={card.language.toUpperCase()} /> : null}
        {card.status && card.status !== "established" ? (
          <Chip label={t(`context.status.${card.status}`)} testID="context-status" />
        ) : null}
        {(card.flags ?? []).map((f) => (
          <Chip key={f} label={t(`context.flag.${f}`, { defaultValue: f })} />
        ))}
      </View>

      <ScrollView contentContainerClassName="gap-5 p-5">
        <Body card={card} scope={scope} />

        {card.proposed || card.translation ? <Reach card={card} /> : null}

        <View className="gap-1">
          <Label>{t("context.appliesIn")}</Label>
          <Text testID="context-where" className="text-foreground text-sm">
            {where.join(" · ")}
          </Text>
        </View>

        <OriginLine card={card} onOpen={onOpenOrigin} />

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
