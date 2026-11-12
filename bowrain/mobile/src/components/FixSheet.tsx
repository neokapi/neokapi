// The fix sheet: why the translation is not right, the ready-made fixes the
// server computed for that reason, an editor with the codes locked, and a
// rejection with the reason when no fix fits. A fix approves the corrected
// translation in one change set; a rejection asks for another draft.
import {
  FIX_REASONS,
  type DeckCard,
  type FixCandidate,
  type FixCandidates,
  type FixReason,
} from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { ActivityIndicator, Modal, Pressable, ScrollView, Text, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";
import { SafeAreaView } from "react-native-safe-area-context";

import { RunDiff, ServerDiff } from "./DiffView";
import { editable, fromSegments, RunEditor, toSegments, type Segment } from "./RunEditor";
import { RunText } from "./RunText";

export interface FixSheetProps {
  card: DeckCard | null;
  /** Read the fix candidates; null when the server offers none. */
  loadFixes: (card: DeckCard, reason: FixReason | undefined, ai: boolean) => Promise<FixCandidates | null>;
  onFix: (card: DeckCard, runs: Run[], reasons: FixReason[]) => void;
  onReject: (card: DeckCard, reasons: FixReason[], note: string) => void;
  onClose: () => void;
}

export function FixSheet(props: FixSheetProps) {
  return (
    <Modal
      visible={props.card !== null}
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={props.onClose}
    >
      {props.card ? <FixSheetBody {...props} card={props.card} key={props.card.key} /> : null}
    </Modal>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <View className="gap-2">
      <Text className="text-muted-foreground text-xs font-semibold uppercase">{title}</Text>
      {children}
    </View>
  );
}

export function FixSheetBody({ card, loadFixes, onFix, onReject, onClose }: FixSheetProps & { card: DeckCard }) {
  const { t } = useTranslation();
  const [reason, setReason] = useState<FixReason | undefined>(undefined);
  const [ai, setAi] = useState(false);
  const [segments, setSegments] = useState<Segment[] | null>(null);
  const [note, setNote] = useState("");
  const reasons = reason ? [reason] : [];

  const fixes = useQuery({
    queryKey: ["fixes", card.key, card.targetRevision, reason ?? "", ai],
    queryFn: () => loadFixes(card, reason, ai),
    // An offline reviewer still edits and rejects; fixes need the server.
    retry: false,
  });

  const edited = segments ? fromSegments(segments) : null;

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      <View className="border-border flex-row items-center justify-between border-b px-4 py-3">
        <Text className="text-foreground text-lg font-semibold">{t("fix.title")}</Text>
        <Pressable testID="fix-close" accessibilityRole="button" onPress={onClose} hitSlop={12}>
          <Text className="text-primary text-base">{t("common.cancel")}</Text>
        </Pressable>
      </View>
      <ScrollView contentContainerClassName="gap-6 p-4" keyboardShouldPersistTaps="handled">
        <View className="gap-1">
          <RunText runs={card.target} testID="fix-draft" />
        </View>

        <Section title={t("fix.why")}>
          <View className="flex-row flex-wrap gap-2">
            {FIX_REASONS.map((r) => {
              const on = reason === r;
              return (
                <Pressable
                  key={r}
                  testID={`reason-${r}`}
                  accessibilityRole="button"
                  accessibilityState={{ selected: on }}
                  onPress={() => setReason(on ? undefined : r)}
                  className={`rounded-full border px-4 py-2 ${on ? "bg-corrective border-corrective" : "border-border bg-card"}`}
                >
                  <Text className={on ? "text-corrective-foreground" : "text-foreground"}>
                    {t(`reasons.${r}`)}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </Section>

        <Section title={t("fix.readyMade")}>
          {fixes.isPending ? <ActivityIndicator /> : null}
          {fixes.isError ? <Text className="text-muted-foreground">{t("fix.fixesUnavailable")}</Text> : null}
          {fixes.data === null ? <Text className="text-muted-foreground">{t("fix.noFixRoute")}</Text> : null}
          {fixes.data && fixes.data.candidates.length === 0 ? (
            <Text className="text-muted-foreground">{t("fix.noFixes")}</Text>
          ) : null}
          {fixes.data?.candidates.map((c) => (
            <Candidate key={c.id} card={card} candidate={c} onUse={() => onFix(card, c.runs, reasons)} />
          ))}
          {fixes.data?.notes.map((n, i) => (
            <Text key={i} className="text-muted-foreground text-sm">
              {n}
            </Text>
          ))}
          {fixes.data && !ai ? (
            <Pressable testID="fix-ask-ai" accessibilityRole="button" onPress={() => setAi(true)}>
              <Text className="text-primary text-base">{t("fix.askAi")}</Text>
            </Pressable>
          ) : null}
        </Section>

        <Section title={t("fix.edit")}>
          {!editable(card.target) ? (
            <Text className="text-muted-foreground">{t("fix.notEditable")}</Text>
          ) : segments === null ? (
            <Pressable
              testID="fix-start-edit"
              accessibilityRole="button"
              onPress={() => setSegments(toSegments(card.target))}
              className="border-border bg-card items-center rounded-xl border py-3"
            >
              <Text className="text-foreground text-base">{t("fix.startEdit")}</Text>
            </Pressable>
          ) : (
            <View className="gap-3">
              <RunEditor segments={segments} onChange={setSegments} />
              {edited ? <RunDiff before={card.target} after={edited} testID="fix-edit-diff" /> : null}
              <Pressable
                testID="fix-approve-edit"
                accessibilityRole="button"
                disabled={!edited || edited.length === 0}
                onPress={() => edited && onFix(card, edited, reasons)}
                className="bg-primary items-center rounded-2xl py-4 active:opacity-80"
              >
                <Text className="text-primary-foreground text-base font-semibold">{t("fix.approveCorrection")}</Text>
              </Pressable>
            </View>
          )}
        </Section>

        <Section title={t("fix.reject")}>
          <TextInput
            testID="reject-note"
            className="bg-card border-border text-foreground rounded-xl border p-3 text-base"
            placeholder={t("fix.notePlaceholder")}
            value={note}
            onChangeText={setNote}
            multiline
          />
          <Pressable
            testID="fix-reject"
            accessibilityRole="button"
            accessibilityState={{ disabled: reasons.length === 0 }}
            disabled={reasons.length === 0}
            onPress={() => onReject(card, reasons, note.trim())}
            className={`border-corrective items-center rounded-2xl border py-4 ${reasons.length === 0 ? "opacity-40" : ""}`}
          >
            <Text className="text-corrective text-base font-semibold">{t("fix.rejectButton")}</Text>
          </Pressable>
          {reasons.length === 0 ? (
            <Text className="text-muted-foreground text-sm">{t("fix.rejectNeedsReason")}</Text>
          ) : null}
        </Section>
      </ScrollView>
    </SafeAreaView>
  );
}

function Candidate({ card, candidate, onUse }: { card: DeckCard; candidate: FixCandidate; onUse: () => void }) {
  const { t } = useTranslation();
  const serverDiff = candidate.diff && (candidate.diff.current.length > 0 || candidate.diff.prior.length > 0);
  return (
    <View testID={`candidate-${candidate.kind}`} className="bg-card border-border gap-3 rounded-xl border p-3">
      <Text className="text-muted-foreground text-sm">{candidate.label ?? t(`fix.kind.${candidate.kind}`)}</Text>
      {serverDiff && candidate.diff ? (
        <ServerDiff diff={candidate.diff} />
      ) : (
        <RunDiff before={card.target} after={candidate.runs} />
      )}
      <Pressable
        testID={`use-${candidate.id}`}
        accessibilityRole="button"
        onPress={onUse}
        className="bg-primary items-center rounded-xl py-3 active:opacity-80"
      >
        <Text className="text-primary-foreground font-semibold">{t("fix.useFix")}</Text>
      </Pressable>
    </View>
  );
}
