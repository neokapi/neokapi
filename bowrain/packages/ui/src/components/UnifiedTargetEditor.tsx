/**
 * `<UnifiedTargetEditor>` — single editor surface for every target.
 *
 * Replaces three pre-existing editing paths in `TranslationEditor`:
 *
 *   1. The `TargetCellEditor` (Lexical chips) for blocks with inline codes.
 *   2. The plain `<textarea>` for blocks without them.
 *   3. The `PluralTargetCell` dialog opened from the toolbar.
 *
 * One editor surface, identical chip rendering everywhere, plural
 * authoring is a mode toggle inside it. The Lexical-based
 * `InlineCodeEditor` from `@neokapi/ui-primitives` is the body in
 * every case — flat, plural-form-zero, plural-form-other, no-codes
 * (degrades to plain-text editing).
 *
 * The editor's state is runs: the flat translation as one `Run[]`, a plural
 * as one `Run[]` per form. The editor body holds text with chips, and the two
 * meet at `editorText.ts`, the one place runs become editor text and an edit
 * becomes runs again. A save hands the parent runs, which it sends as a change
 * (`set_content` with runs, a plural as one plural run). See AD #408 / #409.
 */

import type { ReactElement } from "react";
import { useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";

import {
  Button,
  InlineCodeEditor,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
  type InlineCodeEditorHandle,
  type SpanInfo,
} from "@neokapi/ui-primitives";

import type { PluralForm, Run } from "@neokapi/kapi-format";
import { pluralPivotCandidates } from "@neokapi/kapi-format";

import { toKapiBlock } from "./blockAdapter";
import { pluralForms, pluralOf, soloPlural, sourceRuns, targetRuns } from "./editor/blockContent";
import { codeSpans, fromEditorText, toEditorText } from "./editor/editorText";
import type { BlockInfo } from "../types/api";

/** CLDR plural-form display order. */
const FORMS: readonly PluralForm[] = ["zero", "one", "two", "few", "many", "other"];

/**
 * What a save hands the parent: the translation as runs, each code kept. A
 * plural is one plural run carrying its forms; a translation built outside
 * the editor (a content-memory match a reviewer uses) is its runs as given.
 */
export interface UnifiedSaveResult {
  runs: Run[];
}

/**
 * Imperative surface of an open target editor. `insertText` inserts plain
 * text at the current Lexical cursor of the mounted editor body (the flat
 * editor, or the active plural form's editor) — it does NOT persist; the
 * insertion becomes part of the user's in-progress edit and lands on save.
 * Used by the Translate surface's term-insert affordance.
 */
export interface UnifiedTargetEditorHandle {
  insertText: (text: string) => void;
}

export interface UnifiedTargetEditorProps {
  /** The block being edited. */
  block: BlockInfo;
  /** Locale whose target the editor opens on. */
  locale: string;
  /**
   * Save handler. Wrapper figures out flat vs plural and calls back
   * with the translation's runs; the parent sends them as a change
   * (`set_content` with runs, a plural as one plural run).
   */
  onSave: (result: UnifiedSaveResult) => void | Promise<void>;
  /** Cancel handler — fired on Escape or the explicit Cancel button. */
  onCancel: () => void;
  /**
   * Compact mode hides the chip palette + preview; the embedded
   * InlineCodeEditor still renders chips for tags themselves. Use
   * for in-cell editing where vertical space matters.
   */
  compact?: boolean;
  /**
   * Optional ref receiving the editor's imperative handle
   * (`UnifiedTargetEditorHandle`) while it is mounted. React nulls the ref
   * on unmount, so `ref.current` doubles as an "is an editor open" probe.
   */
  handleRef?: React.Ref<UnifiedTargetEditorHandle>;
}

/** The runs of each plural form the editor holds. */
type Forms = Partial<Record<PluralForm, Run[]>>;

export function UnifiedTargetEditor({
  block,
  locale,
  onSave,
  onCancel,
  compact,
  handleRef,
}: UnifiedTargetEditorProps): ReactElement {
  // The source codes the editor offers as chips. Derived from the block
  // itself (a stable identity), so a parent re-render does not mint a new
  // array, recompute `initial` and wipe in-progress edits.
  const sourceSpans = useMemo(() => editorSourceSpans(block), [block]);
  const adaptedBlock = useMemo(() => toKapiBlock(block), [block]);
  const candidates = useMemo(() => pluralPivotCandidates(adaptedBlock), [adaptedBlock]);

  const initial = useMemo(
    () => seedInitialState(block, locale, candidates[0]?.name ?? ""),
    [block, locale, candidates],
  );

  const [mode, setMode] = useState<"flat" | "plural">(initial.mode);
  const [activeForm, setActiveForm] = useState<PluralForm>(initial.activeForm);
  const [pivot, setPivot] = useState<string>(initial.pivot);
  const [flatRuns, setFlatRuns] = useState<Run[]>(initial.flat);
  const [forms, setForms] = useState<Forms>(initial.forms);
  const [saving, setSaving] = useState(false);

  // Forward `insertText` to whichever InlineCodeEditor body is mounted
  // (the flat editor, or the active plural form — they share one slot,
  // re-keyed on mode/form, so a single inner ref always points at it).
  const innerEditorRef = useRef<InlineCodeEditorHandle | null>(null);
  useImperativeHandle(
    handleRef,
    () => ({
      insertText: (text: string) => innerEditorRef.current?.insertText(text),
    }),
    [],
  );

  // Reset everything when the (block, locale) tuple changes — eg the
  // translator clicks a different cell.
  useEffect(() => {
    setMode(initial.mode);
    setActiveForm(initial.activeForm);
    setPivot(initial.pivot);
    setFlatRuns(initial.flat);
    setForms(initial.forms);
  }, [initial]);

  const formsPresent = useMemo(() => FORMS.filter((f) => forms[f] !== undefined), [forms]);

  // Capture the live state of the currently-mounted InlineCodeEditor as runs,
  // so a form-tab switch (or mode toggle) doesn't lose unsaved edits.
  const handleEditorChange = useCallback(
    (codedText: string, spans: SpanInfo[]) => {
      const runs = fromEditorText(codedText, spans);
      if (mode === "flat") {
        setFlatRuns(runs);
      } else {
        setForms((prev) => ({ ...prev, [activeForm]: runs }));
      }
    },
    [mode, activeForm],
  );

  const handleSave = async () => {
    setSaving(true);
    try {
      if (mode === "flat") {
        await onSave({ runs: flatRuns });
        return;
      }
      await onSave({ runs: pluralRun(pivot, forms) });
    } finally {
      setSaving(false);
    }
  };

  const upgradeToPlural = () => {
    if (!pivot) return;
    // Seed `other` from the current flat state; leave the rest empty.
    setForms({ other: flatRuns });
    setActiveForm("other");
    setMode("plural");
  };

  const flattenBack = () => {
    // Take the `other` form as the new flat state (matches
    // `downgradePluralTarget` semantics in @neokapi/kapi-format).
    setFlatRuns(forms.other ?? Object.values(forms)[0] ?? []);
    setMode("flat");
  };

  const switchActiveForm = (form: PluralForm) => {
    if (form === activeForm) return;
    // The mounted editor's onChange has already persisted activeForm
    // into forms[activeForm]. Mounting a new form is just a matter of
    // switching active and remounting the editor body (we re-key it
    // on activeForm below).
    setActiveForm(form);
  };

  const editorKey = mode === "plural" ? `plural:${activeForm}` : "flat";
  const editorRuns = mode === "plural" ? (forms[activeForm] ?? []) : flatRuns;
  // The body reads its initial text once, on mount; it is re-keyed on every
  // mode or form switch, so this is the state the editor opens on.
  const editorText = useMemo(() => toEditorText(editorRuns), [editorRuns]);

  return (
    <div className="space-y-3" data-testid="unified-target-editor" data-mode={mode}>
      <ModeHeader
        mode={mode}
        pivot={pivot}
        candidates={candidates}
        canUpgrade={candidates.length > 0}
        onPivotChange={setPivot}
        onUpgrade={upgradeToPlural}
        onFlatten={flattenBack}
      />
      {mode === "plural" ? (
        <FormTabs
          forms={FORMS}
          present={new Set(formsPresent)}
          active={activeForm}
          onSelect={switchActiveForm}
        />
      ) : null}
      <InlineCodeEditor
        key={editorKey}
        initialCodedText={editorText.codedText}
        initialSpans={editorText.spans}
        sourceSpans={sourceSpans}
        onSave={() => void handleSave()}
        onCancel={onCancel}
        onChange={handleEditorChange}
        compact={compact}
        locale={locale}
        handleRef={innerEditorRef}
      />
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={saving} data-testid="unified-save">
          {saving ? "Saving…" : "Save"}
        </Button>
      </div>
    </div>
  );
}

// ─── Mode header ─────────────────────────────────────────────────

function ModeHeader({
  mode,
  pivot,
  candidates,
  canUpgrade,
  onPivotChange,
  onUpgrade,
  onFlatten,
}: {
  mode: "flat" | "plural";
  pivot: string;
  candidates: ReturnType<typeof pluralPivotCandidates>;
  canUpgrade: boolean;
  onPivotChange: (next: string) => void;
  onUpgrade: () => void;
  onFlatten: () => void;
}) {
  if (mode === "plural") {
    return (
      <div className="flex items-center gap-3 text-sm" data-testid="mode-header-plural">
        <span className="text-muted-foreground">
          Plural pivot: <span className="font-mono">{pivot}</span>
        </span>
        <div className="flex-1" />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onFlatten}
          aria-label="Collapse plural target to flat text"
        >
          Flatten back
        </Button>
      </div>
    );
  }
  if (!canUpgrade) {
    // No pivot candidates means the source has no numeric variable;
    // we silently hide the plural affordance rather than offering an
    // upgrade we can't pivot on.
    return null;
  }
  return (
    <div className="flex items-center gap-2 text-sm" data-testid="mode-header-flat">
      <span className="text-muted-foreground">Pivot:</span>
      <Select value={pivot} onValueChange={onPivotChange}>
        <SelectTrigger className="h-8 w-[160px]" aria-label="Plural pivot variable">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {candidates.map((c) => (
            <SelectItem key={c.name} value={c.name}>
              {c.label}
              {c.sourcePivot ? " (source pivot)" : ""}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onUpgrade}
        disabled={!pivot}
        data-testid="upgrade-to-plural"
      >
        Make plural…
      </Button>
    </div>
  );
}

// ─── Form tabs ────────────────────────────────────────────────────

function FormTabs({
  forms,
  present,
  active,
  onSelect,
}: {
  forms: readonly PluralForm[];
  present: Set<PluralForm>;
  active: PluralForm;
  onSelect: (form: PluralForm) => void;
}) {
  return (
    <div className="flex gap-1 rounded-md border border-border p-1" role="tablist">
      {forms.map((form) => {
        const filled = present.has(form);
        const isActive = form === active;
        return (
          <button
            key={form}
            type="button"
            role="tab"
            aria-selected={isActive}
            data-testid={`form-tab-${form}`}
            onClick={() => onSelect(form)}
            className={cn(
              "flex-1 rounded px-2 py-1 text-xs uppercase tracking-wide transition-colors",
              isActive
                ? "bg-primary text-primary-foreground"
                : filled
                  ? "text-foreground hover:bg-accent"
                  : "text-muted-foreground hover:bg-accent",
            )}
          >
            {form}
            {filled && !isActive ? <span className="ml-1 text-xs opacity-60">●</span> : null}
          </button>
        );
      })}
    </div>
  );
}

// ─── Initial state ────────────────────────────────────────────────

interface SeededState {
  mode: "flat" | "plural";
  activeForm: PluralForm;
  pivot: string;
  flat: Run[];
  forms: Forms;
}

/**
 * The state the editor opens on. A translation that is one plural (a plural
 * run, or ICU text spelling one) opens on its forms, each with its codes. A
 * message that is one plural, not yet translated, opens in plural mode on the
 * source's pivot: the forms are the target language's, which need not be the
 * source's, so only `other` opens and the person fills the rest. Anything
 * else opens flat, with `defaultPivot` offered for an upgrade.
 */
function seedInitialState(block: BlockInfo, locale: string, defaultPivot: string): SeededState {
  const target = targetRuns(block, locale);

  const targetPlural = pluralOf(target);
  if (targetPlural) {
    const forms = pluralForms(targetPlural);
    return {
      mode: "plural",
      activeForm: forms.other ? "other" : ((Object.keys(forms)[0] as PluralForm) ?? "other"),
      pivot: targetPlural.plural.pivot,
      flat: [],
      forms,
    };
  }

  const sourcePlural = soloPlural(sourceRuns(block));
  if (sourcePlural && target.length === 0) {
    return {
      mode: "plural",
      activeForm: "other",
      pivot: sourcePlural.plural.pivot,
      flat: [],
      forms: { other: [] },
    };
  }

  return { mode: "flat", activeForm: "other", pivot: defaultPivot, flat: target, forms: {} };
}

/**
 * The source codes the editor offers as chips: the source's, or, for a
 * message that is one plural, the codes of its `other` form (the variable a
 * form repeats, say), so each target form can place them.
 */
function editorSourceSpans(block: BlockInfo): SpanInfo[] {
  const runs = sourceRuns(block);
  const plural = soloPlural(runs);
  return codeSpans(plural ? (plural.plural.forms.other ?? []) : runs);
}

// ─── Save serialisation ───────────────────────────────────────────

/**
 * The plural run a save sends: each form's runs under its name, codes and
 * all. ICU plural rules require an `other` case, so an absent one is sent
 * empty rather than left out.
 */
function pluralRun(pivot: string, forms: Forms): Run[] {
  const formRuns: Forms = {};
  for (const form of FORMS) {
    const runs = forms[form];
    if (runs) formRuns[form] = runs;
  }
  if (!formRuns.other) formRuns.other = [];
  return [{ plural: { pivot, forms: formRuns } }];
}
