// Edit a translation's text while its placeholders and inline codes stay
// locked chips. The runs become a row of segments: an editable field for each
// stretch of text, and a chip for each code, so typing can never break or
// drop a code. Text marked not for translation is locked as well.
import {
  codeDisplays,
  projectRuns,
  type CodeDisplay,
  type ModelRunSpec,
  type Run,
} from "@neokapi/kapi-format";
import { useMemo } from "react";
import { TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";

import { CodeChip, LockedText } from "./CodeChip";

/**
 * One segment of the editor: a field, a locked code (drawn from its display,
 * never from its data), or locked text marked not for translation.
 */
export type Segment =
  | { kind: "text"; text: string }
  | { kind: "locked"; run: Run; display: CodeDisplay }
  | { kind: "fixed"; run: Run; text: string };

function segmentSpec(runs: readonly Run[]): ModelRunSpec<Segment> {
  const code = codeDisplays(runs);
  return {
    text: (r) =>
      r.noTranslate ? { kind: "fixed", run: r, text: r.text } : { kind: "text", text: r.text },
    ph: (r) => ({ kind: "locked", run: r, display: code.ph(r.ph) }),
    pcOpen: (r) => ({ kind: "locked", run: r, display: code.pcOpen(r.pcOpen) }),
    pcClose: (r) => ({ kind: "locked", run: r, display: code.pcClose(r.pcClose) }),
    sub: (r) => ({ kind: "locked", run: r, display: code.sub(r.sub) }),
    // A plural or select has forms of its own, which a single field cannot hold.
    // It stays as it is, and the editor reports that it cannot change it.
    plural: (r) => ({ kind: "locked", run: r, display: code.choice(r.plural.pivot) }),
    select: (r) => ({ kind: "locked", run: r, display: code.choice(r.select.pivot) }),
    fallback: () => ({ kind: "locked", run: { text: "" }, display: code.unknown() }),
  };
}

/**
 * The segments of a run sequence: adjacent text joined into one field, and an
 * empty field before, between and after locked runs so a person can type there.
 */
export function toSegments(runs: readonly Run[]): Segment[] {
  const out: Segment[] = [];
  for (const s of projectRuns(runs, segmentSpec(runs))) {
    const prev = out[out.length - 1];
    if (s.kind === "text" && prev?.kind === "text") {
      out[out.length - 1] = { kind: "text", text: prev.text + s.text };
    } else {
      if (s.kind !== "text" && prev?.kind !== "text") out.push({ kind: "text", text: "" });
      out.push(s);
    }
  }
  if (out[out.length - 1]?.kind !== "text") out.push({ kind: "text", text: "" });
  return out;
}

/** The runs a segment row stands for: empty fields drop out, locked runs stay as they were. */
export function fromSegments(segments: readonly Segment[]): Run[] {
  const runs: Run[] = [];
  for (const s of segments) {
    if (s.kind !== "text") runs.push(s.run);
    else if (s.text !== "") runs.push({ text: s.text });
  }
  return runs;
}

/** Whether a run is one the editor can keep or change as a segment. */
const EDITABLE: ModelRunSpec<boolean> = {
  text: () => true,
  ph: () => true,
  pcOpen: () => true,
  pcClose: () => true,
  sub: () => true,
  plural: () => false,
  select: () => false,
  fallback: () => false,
};

/** Whether the editor can change these runs: not when they hold a plural or select. */
export function editable(runs: readonly Run[]): boolean {
  return projectRuns(runs, EDITABLE).every(Boolean);
}

export interface RunEditorProps {
  segments: Segment[];
  onChange: (segments: Segment[]) => void;
}

export function RunEditor({ segments, onChange }: RunEditorProps) {
  const { t } = useTranslation();
  const fields = useMemo(() => segments.filter((s) => s.kind === "text").length, [segments]);
  let field = 0;
  return (
    <View className="bg-card border-border flex-row flex-wrap items-center gap-1 rounded-xl border p-3">
      {segments.map((s, i) => {
        if (s.kind === "locked")
          return <CodeChip key={`l${i}`} display={s.display} testID="editor-chip" />;
        if (s.kind === "fixed")
          return <LockedText key={`f${i}`} text={s.text} testID="editor-chip" />;
        const n = field++;
        return (
          <TextInput
            key={`t${i}`}
            testID={`editor-field-${n}`}
            accessibilityLabel={t("fix.editField", { n: n + 1, count: fields })}
            className="text-foreground min-w-6 text-base"
            value={s.text}
            multiline
            onChangeText={(text) => {
              const next = segments.slice();
              next[i] = { kind: "text", text };
              onChange(next);
            }}
          />
        );
      })}
    </View>
  );
}
