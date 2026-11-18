// A term met in a string, opened: what the term means, the form to use in
// the language under review, whether the translation uses it, and the
// concept's terms in every language. Previous and next step through the
// other terms of the same component or section, so a reviewer can check
// them in turn without going back to the strings. Whether the translation
// uses the term is the term check's verdict, read from the server; the sheet
// decides nothing itself.
import type { Concept, ConceptTerm, UnitTerm } from "@neokapi/bowrain-client";
import { useQuery } from "@tanstack/react-query";
import { ActivityIndicator, Modal, Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { SafeAreaView } from "./SafeArea";

/** A term in the container, in the unit it was met in. */
export interface TermStop {
  term: UnitTerm;
  blockId: string;
  /** The unit has a translation the check could read. */
  hasTarget: boolean;
}

/** Terms with no forbidden or deprecated status first, then the rest. */
const STATUS_ORDER = ["preferred", "approved", "admitted", "proposed", "deprecated", "forbidden"];

function byStatus(a: ConceptTerm, b: ConceptTerm): number {
  const ai = STATUS_ORDER.indexOf(a.status);
  const bi = STATUS_ORDER.indexOf(b.status);
  return (ai < 0 ? 9 : ai) - (bi < 0 ? 9 : bi);
}

export interface TermSheetProps {
  stops: TermStop[];
  /** The index of the open term in stops; null when closed. */
  at: number | null;
  locale: string;
  workspace: string;
  loadConcept?: (conceptId: string) => Promise<Concept>;
  onMove: (at: number) => void;
  onClose: () => void;
}

export function TermSheet({
  stops,
  at,
  locale,
  workspace,
  loadConcept,
  onMove,
  onClose,
}: TermSheetProps) {
  return (
    <Modal
      visible={at !== null}
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      {at !== null && stops[at] ? (
        <TermBody
          stop={stops[at]}
          at={at}
          count={stops.length}
          locale={locale}
          workspace={workspace}
          loadConcept={loadConcept}
          onMove={onMove}
          onClose={onClose}
        />
      ) : null}
    </Modal>
  );
}

function TermBody({
  stop,
  at,
  count,
  locale,
  workspace,
  loadConcept,
  onMove,
  onClose,
}: {
  stop: TermStop;
  at: number;
  count: number;
  locale: string;
  workspace: string;
  loadConcept?: (conceptId: string) => Promise<Concept>;
  onMove: (at: number) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { term, hasTarget } = stop;
  const conceptId = term.conceptId ?? "";
  const concept = useQuery({
    queryKey: ["concept", workspace, conceptId],
    queryFn: () => loadConcept!(conceptId),
    enabled: !!loadConcept && !!conceptId,
    staleTime: 5 * 60_000,
  });
  // The forms the concept holds in the language under review, preferred
  // first; the rule's rendering when the term comes from the recipe alone.
  const lang = locale.toLowerCase().split("-")[0];
  const conceptForms = (concept.data?.terms ?? [])
    .filter((ct) => ct.locale.toLowerCase().split("-")[0] === lang)
    .filter((ct) => ct.status !== "forbidden" && ct.status !== "deprecated")
    .sort(byStatus)
    .map((ct) => ct.text);
  const forms = conceptForms.length ? conceptForms : term.replacement ? [term.replacement] : [];
  const keep = term.doNotTranslate || concept.data?.doNotTranslate;
  const languages = new Map<string, ConceptTerm[]>();
  for (const ct of [...(concept.data?.terms ?? [])].sort(byStatus)) {
    languages.set(ct.locale, [...(languages.get(ct.locale) ?? []), ct]);
  }

  return (
    <SafeAreaView className="bg-background flex-1">
      <View className="border-border flex-row items-center border-b px-2 py-2">
        <Pressable
          testID="term-prev"
          accessibilityRole="button"
          accessibilityLabel={t("term.prev")}
          disabled={at === 0}
          onPress={() => onMove(at - 1)}
          className="h-11 w-11 items-center justify-center"
        >
          <Text
            className={`text-2xl ${at === 0 ? "text-muted-foreground opacity-40" : "text-foreground"}`}
          >
            ‹
          </Text>
        </Pressable>
        <Text className="text-muted-foreground flex-1 text-center text-sm">
          {t("term.of", { n: at + 1, count })}
        </Text>
        <Pressable
          testID="term-next"
          accessibilityRole="button"
          accessibilityLabel={t("term.next")}
          disabled={at + 1 >= count}
          onPress={() => onMove(at + 1)}
          className="h-11 w-11 items-center justify-center"
        >
          <Text
            className={`text-2xl ${at + 1 >= count ? "text-muted-foreground opacity-40" : "text-foreground"}`}
          >
            ›
          </Text>
        </Pressable>
        <Pressable
          testID="term-close"
          accessibilityRole="button"
          onPress={onClose}
          className="px-3 py-2"
        >
          <Text className="text-primary text-base font-semibold">{t("common.done")}</Text>
        </Pressable>
      </View>

      <ScrollView contentContainerClassName="gap-5 px-4 py-4 pb-16">
        <View className="gap-1">
          <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
            {t("term.kind")}
          </Text>
          <Text testID="term-source" className="text-foreground text-2xl font-semibold">
            {term.term}
          </Text>
          {concept.data?.domain ? (
            <Text className="text-muted-foreground text-sm">{concept.data.domain}</Text>
          ) : null}
          {concept.data?.definition ? (
            <Text className="text-foreground pt-1 text-base">{concept.data.definition}</Text>
          ) : null}
        </View>

        <View className="bg-muted gap-2 rounded-xl p-3">
          <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
            {t("term.use", { locale })}
          </Text>
          {keep ? (
            <Text className="text-foreground text-base">{t("term.keep", { term: term.term })}</Text>
          ) : forms.length ? (
            <Text testID="term-forms" className="text-foreground text-lg font-semibold">
              {forms.join(", ")}
            </Text>
          ) : (
            <Text className="text-muted-foreground text-base">{t("term.noForm")}</Text>
          )}
          {hasTarget ? (
            <Text
              testID={term.satisfied ? "term-used" : "term-unused"}
              className={`text-sm ${term.satisfied ? "text-approve" : "text-corrective"}`}
            >
              {term.satisfied
                ? t("term.used")
                : keep
                  ? t("term.notKept", { term: term.term })
                  : t("term.unused", { form: term.replacement ?? forms[0] ?? term.term })}
            </Text>
          ) : null}
        </View>

        {loadConcept && conceptId ? (
          <View className="gap-3">
            <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
              {t("term.languages")}
            </Text>
            {concept.isPending ? <ActivityIndicator /> : null}
            {concept.isError ? (
              <Text className="text-muted-foreground text-sm">{t("common.loadFailed")}</Text>
            ) : null}
            {[...languages.entries()].map(([lang, list]) => (
              <View key={lang} testID={`term-lang-${lang}`} className="gap-1">
                <Text className="text-muted-foreground text-xs font-semibold">{lang}</Text>
                {list.map((ct) => (
                  <View key={`${ct.text}-${ct.status}`} className="flex-row items-baseline gap-2">
                    <Text
                      className={`text-base ${ct.status === "forbidden" || ct.status === "deprecated" ? "text-muted-foreground line-through" : "text-foreground"}`}
                    >
                      {ct.text}
                    </Text>
                    <Text className="text-muted-foreground text-xs">
                      {t(`term.status.${ct.status}`, { defaultValue: ct.status })}
                    </Text>
                  </View>
                ))}
                {list.find((ct) => ct.note)?.note ? (
                  <Text className="text-muted-foreground text-sm">
                    {list.find((ct) => ct.note)?.note}
                  </Text>
                ) : null}
              </View>
            ))}
          </View>
        ) : null}
      </ScrollView>
    </SafeAreaView>
  );
}
