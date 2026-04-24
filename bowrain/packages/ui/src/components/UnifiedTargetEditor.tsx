/**
 * `<UnifiedTargetEditor>` — single editor surface for every target.
 *
 * Replaces three pre-existing editing paths in `TranslationEditor`:
 *
 *   1. The `TargetCellEditor` (Lexical chips) for `has_spans` blocks.
 *   2. The plain `<textarea>` for non-`has_spans` blocks.
 *   3. The `PluralTargetCell` dialog opened from the toolbar.
 *
 * One editor surface, identical chip rendering everywhere, plural
 * authoring is a mode toggle inside it. The Lexical-based
 * `InlineCodeEditor` from `@neokapi/ui-primitives` is the body in
 * every case — flat, plural-form-zero, plural-form-other, no-spans
 * (degrades to plain-text editing).
 *
 * State + save semantics live in `useUnifiedTargetEditor`. This file
 * is presentation. See AD #408 / #409 for the design.
 */

import type { ReactElement } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";

import {
  Button,
  InlineCodeEditor,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
  codedToRuns,
  parsePluralFormForChips,
  runsToCoded,
  type SpanInfo as PrimitiveSpanInfo,
} from "@neokapi/ui-primitives";

import type { Block, PluralForm, Run } from "@neokapi/kapi-format";
import { flattenRuns, pluralPivotCandidates } from "@neokapi/kapi-format";

import { toKapiBlock } from "./blockAdapter";
import type { BlockInfo, SpanInfo } from "../types/api";

/** CLDR plural-form display order. */
const FORMS: readonly PluralForm[] = ["zero", "one", "two", "few", "many", "other"];

export type UnifiedSaveResult =
  | { kind: "flat"; codedText: string; spans: PrimitiveSpanInfo[] }
  | { kind: "plural"; text: string };

export interface UnifiedTargetEditorProps {
  /** The block being edited. */
  block: BlockInfo;
  /** Locale whose target the editor opens on. */
  locale: string;
  /**
   * Save handler. Wrapper figures out flat vs plural and calls back
   * with the right shape; the parent dispatches to the appropriate
   * API (typically `updateBlockTargetCoded` for flat, `updateBlockTarget`
   * + clearing the coded column for plural).
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
}

/**
 * Shape of a single form's editable state. Mirrors what
 * `InlineCodeEditor` consumes/emits.
 */
interface FormState {
  codedText: string;
  spans: SpanInfo[];
}

export function UnifiedTargetEditor({
  block,
  locale,
  onSave,
  onCancel,
  compact,
}: UnifiedTargetEditorProps): ReactElement {
  const sourceSpans = block.source_spans ?? [];
  const adaptedBlock = useMemo(() => toKapiBlock(block), [block]);
  const candidates = useMemo(() => pluralPivotCandidates(adaptedBlock as Block), [adaptedBlock]);

  const initial = useMemo(
    () => seedInitialState(block, locale, sourceSpans),
    [block, locale, sourceSpans],
  );

  const [mode, setMode] = useState<"flat" | "plural">(initial.mode);
  const [activeForm, setActiveForm] = useState<PluralForm>(initial.activeForm);
  const [pivot, setPivot] = useState<string>(initial.pivot);
  const [flatState, setFlatState] = useState<FormState>(initial.flat);
  const [pluralForms, setPluralForms] = useState<Partial<Record<PluralForm, FormState>>>(
    initial.forms,
  );
  const [saving, setSaving] = useState(false);

  // Reset everything when the (block, locale) tuple changes — eg the
  // translator clicks a different cell.
  useEffect(() => {
    setMode(initial.mode);
    setActiveForm(initial.activeForm);
    setPivot(initial.pivot);
    setFlatState(initial.flat);
    setPluralForms(initial.forms);
  }, [initial]);

  const formsPresent = useMemo(
    () => FORMS.filter((f) => pluralForms[f] !== undefined),
    [pluralForms],
  );

  // Capture the live state of the currently-mounted InlineCodeEditor
  // so a form-tab switch (or mode toggle) doesn't lose unsaved edits.
  const handleEditorChange = useCallback(
    (codedText: string, spans: PrimitiveSpanInfo[]) => {
      const cast = spans as SpanInfo[];
      if (mode === "flat") {
        setFlatState({ codedText, spans: cast });
      } else {
        setPluralForms((prev) => ({
          ...prev,
          [activeForm]: { codedText, spans: cast },
        }));
      }
    },
    [mode, activeForm],
  );

  const handleSave = async () => {
    setSaving(true);
    try {
      if (mode === "flat") {
        await onSave({ kind: "flat", codedText: flatState.codedText, spans: flatState.spans });
        return;
      }
      const text = serialiseFormsToICU(pivot, pluralForms);
      await onSave({ kind: "plural", text });
    } finally {
      setSaving(false);
    }
  };

  const upgradeToPlural = () => {
    if (!pivot) return;
    // Seed `other` from the current flat state; leave the rest empty.
    setPluralForms({
      ...emptyForms(),
      other: { codedText: flatState.codedText, spans: flatState.spans },
    });
    setActiveForm("other");
    setMode("plural");
  };

  const flattenBack = () => {
    // Take the `other` form as the new flat state (matches
    // `downgradePluralTarget` semantics in @neokapi/kapi-format).
    const otherState = pluralForms.other ?? Object.values(pluralForms)[0] ?? blankFormState();
    setFlatState(otherState);
    setMode("flat");
  };

  const switchActiveForm = (form: PluralForm) => {
    if (form === activeForm) return;
    // The mounted editor's onChange has already persisted activeForm
    // into pluralForms[activeForm]. Mounting a new form is just a
    // matter of switching active and remounting the editor body
    // (we re-key it on activeForm below).
    setActiveForm(form);
  };

  const editorKey = mode === "plural" ? `plural:${activeForm}` : "flat";
  const editorState = mode === "plural" ? (pluralForms[activeForm] ?? blankFormState()) : flatState;

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
        initialCodedText={editorState.codedText}
        initialSpans={editorState.spans as PrimitiveSpanInfo[]}
        sourceSpans={sourceSpans as PrimitiveSpanInfo[]}
        onSave={() => void handleSave()}
        onCancel={onCancel}
        onChange={handleEditorChange}
        compact={compact}
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
              {c.sourcePivot ? " — source pivot" : ""}
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
  flat: FormState;
  forms: Partial<Record<PluralForm, FormState>>;
}

function seedInitialState(block: BlockInfo, locale: string, sourceSpans: SpanInfo[]): SeededState {
  const rawTarget = block.targets[locale] ?? "";
  const codedTarget = block.targets_coded?.[locale] ?? "";

  // Plural targets always live in `targets[locale]` as ICU syntax —
  // if we recognise that shape, switch straight into per-form mode.
  const preview = parsePluralFormForChips(rawTarget, sourceSpans as PrimitiveSpanInfo[], "other");
  if (preview) {
    const forms = decodePluralForms(rawTarget, sourceSpans);
    const adapted = toKapiBlock(block);
    const candidates = pluralPivotCandidates(adapted as Block);
    return {
      mode: "plural",
      activeForm: forms.other ? "other" : ((Object.keys(forms)[0] as PluralForm) ?? "other"),
      pivot: preview.pivot || candidates[0]?.name || "",
      flat: blankFormState(),
      forms,
    };
  }

  // Flat path. Prefer the coded representation for chip rendering;
  // fall back to plain `targets[locale]` otherwise (and synthesise
  // empty spans).
  const flat: FormState = codedTarget
    ? { codedText: codedTarget, spans: sourceSpans }
    : { codedText: rawTarget, spans: [] };

  const adapted = toKapiBlock(block);
  const candidates = pluralPivotCandidates(adapted as Block);
  return {
    mode: "flat",
    activeForm: "other",
    pivot: candidates[0]?.name ?? "",
    flat,
    forms: {},
  };
}

function decodePluralForms(
  icuString: string,
  sourceSpans: SpanInfo[],
): Partial<Record<PluralForm, FormState>> {
  const out: Partial<Record<PluralForm, FormState>> = {};
  for (const form of FORMS) {
    const preview = parsePluralFormForChips(icuString, sourceSpans as PrimitiveSpanInfo[], form);
    if (preview && preview.shownForm === form) {
      out[form] = { codedText: preview.codedText, spans: preview.spans as SpanInfo[] };
    }
  }
  return out;
}

function blankFormState(): FormState {
  return { codedText: "", spans: [] };
}

function emptyForms(): Partial<Record<PluralForm, FormState>> {
  return {};
}

// ─── Save serialisation ───────────────────────────────────────────

function serialiseFormsToICU(pivot: string, forms: Partial<Record<PluralForm, FormState>>): string {
  // Build a plural Run with each form's coded state converted into
  // typed Runs. `flattenRuns` then emits the canonical ICU plural
  // string, identical to what the developer-authored `<Plural>`
  // path produces — same wire format end-to-end.
  const formRuns: Partial<Record<PluralForm, Run[]>> = {};
  for (const form of FORMS) {
    const state = forms[form];
    if (!state) continue;
    formRuns[form] = codedToRuns(state.codedText, state.spans as PrimitiveSpanInfo[]);
  }
  // Ensure at least an `other` form exists — ICU plural rules require
  // a fallback case, and `flattenRuns` would otherwise emit a string
  // the resolver couldn't match against.
  if (!formRuns.other) formRuns.other = [];

  const plural: Run[] = [{ plural: { pivot, forms: formRuns } }];
  return flattenRuns(plural);
}

// ─── Re-exports for type compatibility ────────────────────────────

// `runsToCoded` is consumed inside the file but also worth re-exporting
// so consumers seeding a UnifiedTargetEditor from typed Run[] can
// produce the matching FormState shape without a second import path.
export { runsToCoded };
