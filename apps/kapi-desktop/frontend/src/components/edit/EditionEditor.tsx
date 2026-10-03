import { useMemo, useRef, useState } from "react";
import {
  InlineCodeEditor,
  PluralTargetEditor,
  codedToEditText,
  codedToRuns,
  editTextToCoded,
  runsToCoded,
} from "@neokapi/ui-primitives";
import type { CodeRead, PathStep, RunPath, StructureRead } from "@neokapi/contract-types";
import { isPlural, type PluralForm, type PluralRunWrapper, type Run } from "@neokapi/kapi-format";
import { t } from "@neokapi/i18n-react/runtime";

import type { EditionContent, EditionEdit } from "../../lib/changes";

/** The order a person reads plural forms in. */
const FORM_ORDER: readonly PluralForm[] = ["zero", "one", "two", "few", "many", "other"];

export interface EditionEditorProps {
  /** The edition as the change service read it. */
  content: EditionContent;
  /** The language the edition is written in, for the editor's direction. */
  locale?: string;
  /**
   * The content a translation answers to, whose codes the tag palette offers
   * and the validation bar holds the edit to. The edition's own content when
   * absent, as for the source.
   */
  reference?: { text: string; codes?: Readonly<Record<string, CodeRead>> };
  /**
   * Called with the edits against the content read: none when the editor
   * holds what it was given, the whole edition for flat content, and one edit
   * per changed branch of a plural or select.
   */
  onChange: (edits: EditionEdit[]) => void;
  /** Enter in a flat editor; the host saves. */
  onSubmit?: () => void;
  /** Escape in a flat editor; the host reverts. */
  onCancel?: () => void;
  /** Take focus on mount. A surface beside keyboard navigation passes false. */
  autoFocus?: boolean;
  /** Hide the tag palette and the inline preview. */
  compact?: boolean;
  "data-slot"?: string;
}

/**
 * The editor of one edition, over the change contract's read.
 *
 * Flat content (text with inline codes) is edited in the inline-code editor,
 * each code a chip that keeps its place, its type and its constraints; the
 * text goes back as the read's own placeholder form, so a code the edit kept
 * is the code the edition holds. A plural or select is edited a branch at a
 * time (PluralTargetEditor for a plural), each changed branch an edit of its
 * own addressed by the path the read lists, so the structure is never
 * rewritten as text.
 *
 * The editor is uncontrolled: a host that wants it to start over (a revert, a
 * new revision) gives it a new React key.
 */
export function EditionEditor({
  content,
  locale,
  reference,
  onChange,
  onSubmit,
  onCancel,
  autoFocus = false,
  compact,
  "data-slot": dataSlot,
}: EditionEditorProps) {
  const structures = content.structures ?? [];
  if (structures.length > 0) {
    return (
      <div className="space-y-3" data-slot={dataSlot} data-edition-editor="branches">
        <BranchEditors content={content} structures={structures} onChange={onChange} />
      </div>
    );
  }
  return (
    <FlatEditor
      content={content}
      locale={locale}
      reference={reference}
      onChange={onChange}
      onSubmit={onSubmit}
      onCancel={onCancel}
      autoFocus={autoFocus}
      compact={compact}
      dataSlot={dataSlot}
    />
  );
}

function FlatEditor({
  content,
  locale,
  reference,
  onChange,
  onSubmit,
  onCancel,
  autoFocus,
  compact,
  dataSlot,
}: Omit<EditionEditorProps, "data-slot"> & { dataSlot?: string }) {
  const initial = useMemo(
    () => editTextToCoded(content.text, content.codes ?? {}),
    [content.text, content.codes],
  );
  const referenceSpans = useMemo(
    () =>
      reference ? editTextToCoded(reference.text, reference.codes ?? {}).spans : initial.spans,
    [reference, initial.spans],
  );
  return (
    // The editor draws its field from the theme the platform defines; here it
    // takes the desktop's own surface.
    <div
      data-slot={dataSlot}
      data-edition-editor="flat"
      style={{ ["--bg-tertiary" as string]: "var(--background)" }}
      className="rounded-md text-sm"
    >
      <InlineCodeEditor
        initialCodedText={initial.codedText}
        initialSpans={initial.spans}
        sourceSpans={referenceSpans}
        onChange={(codedText, spans) => {
          const text = codedToEditText(codedText, spans);
          onChange(text === content.text ? [] : [{ text }]);
        }}
        onSave={() => onSubmit?.()}
        onCancel={() => onCancel?.()}
        compact={compact}
        locale={locale}
        autoFocus={autoFocus}
      />
    </div>
  );
}

/** A branch of a plural or select: the path to it, and its text as read. */
interface Branch {
  path: RunPath;
  text: string;
}

function pathTo(structure: StructureRead, branch: string): RunPath {
  const step: PathStep =
    structure.kind === "select" ? { select: branch } : { plural: branch as PluralForm };
  return [...structure.path, step];
}

/** Runs for one branch, through the coded-text bridge, so the plural editor reads its codes. */
function branchRuns(text: string, codes: Readonly<Record<string, CodeRead>>): Run[] {
  const { codedText, spans } = editTextToCoded(text, codes);
  return codedToRuns(codedText, spans);
}

function BranchEditors({
  content,
  structures,
  onChange,
}: {
  content: EditionContent;
  structures: StructureRead[];
  onChange: (edits: EditionEdit[]) => void;
}) {
  // What each branch says now, by path, against what the read said.
  const original = useMemo(() => {
    const out = new Map<string, Branch>();
    for (const s of structures) {
      for (const [name, text] of Object.entries(s.branches)) {
        const path = pathTo(s, name);
        out.set(JSON.stringify(path), { path, text });
      }
    }
    return out;
  }, [structures]);
  const current = useRef(new Map<string, string>());

  const emit = () => {
    const edits: EditionEdit[] = [];
    for (const [key, text] of current.current) {
      const was = original.get(key);
      if (was && was.text !== text) edits.push({ path: was.path, text });
    }
    onChange(edits);
  };
  const set = (path: RunPath, text: string) => {
    current.current.set(JSON.stringify(path), text);
    emit();
  };

  return (
    <>
      {structures.map((s, i) =>
        s.kind === "plural" ? (
          <PluralBranches
            key={i}
            structure={s}
            codes={content.codes ?? {}}
            onBranch={(form, text) => set(pathTo(s, form), text)}
          />
        ) : (
          <SelectBranches
            key={i}
            structure={s}
            codes={content.codes ?? {}}
            onBranch={(name, text) => set(pathTo(s, name), text)}
          />
        ),
      )}
    </>
  );
}

function PluralBranches({
  structure,
  codes,
  onBranch,
}: {
  structure: StructureRead;
  codes: Readonly<Record<string, CodeRead>>;
  onBranch: (form: PluralForm, text: string) => void;
}) {
  const forms = useMemo(
    () => FORM_ORDER.filter((f) => f in structure.branches),
    [structure.branches],
  );
  const [target, setTarget] = useState<Run[]>(() => {
    const byForm: Partial<Record<PluralForm, Run[]>> = {};
    for (const f of forms) byForm[f] = branchRuns(structure.branches[f], codes);
    return [{ plural: { pivot: structure.pivot ?? "", forms: byForm } }];
  });
  // Every code a branch holds, so a token typed in any form resolves to it.
  const block = useMemo(
    () => ({
      source: forms.flatMap((f) => branchRuns(structure.branches[f], codes)),
      placeholders: [],
    }),
    [forms, structure.branches, codes],
  );

  return (
    <div className="space-y-1" data-slot="edition-editor-plural">
      <p className="text-[11px] text-muted-foreground">
        {structure.pivot
          ? t("Plural forms of {pivot}", { pivot: structure.pivot })
          : t("Plural forms")}
      </p>
      <PluralTargetEditor
        block={block}
        target={target}
        forms={forms}
        fixedStructure
        onChange={(next) => {
          setTarget(next);
          // The structure is fixed, so the editor hands back the plural it was
          // given with its forms edited.
          if (!isPlural(next)) return;
          const plural = (next[0] as PluralRunWrapper).plural;
          for (const f of forms) {
            const runs = plural.forms[f] ?? [];
            const coded = runsToCoded(runs);
            onBranch(f, codedToEditText(coded.codedText, coded.spans));
          }
        }}
      />
    </div>
  );
}

function SelectBranches({
  structure,
  codes,
  onBranch,
}: {
  structure: StructureRead;
  codes: Readonly<Record<string, CodeRead>>;
  onBranch: (name: string, text: string) => void;
}) {
  const names = useMemo(() => Object.keys(structure.branches).sort(), [structure.branches]);
  return (
    <div className="space-y-2" data-slot="edition-editor-select">
      <p className="text-[11px] text-muted-foreground">
        {structure.pivot ? t("Cases of {pivot}", { pivot: structure.pivot }) : t("Cases")}
      </p>
      {names.map((name) => {
        const { codedText, spans } = editTextToCoded(structure.branches[name], codes);
        return (
          <label key={name} className="flex gap-3">
            <span
              className="mt-2 w-20 shrink-0 text-right text-xs text-muted-foreground"
              translate="no"
            >
              {name}
            </span>
            <div className="min-w-0 flex-1">
              <InlineCodeEditor
                initialCodedText={codedText}
                initialSpans={spans}
                sourceSpans={spans}
                onChange={(next, nextSpans) => onBranch(name, codedToEditText(next, nextSpans))}
                onSave={() => {}}
                onCancel={() => {}}
                compact
                autoFocus={false}
              />
            </div>
          </label>
        );
      })}
    </div>
  );
}
