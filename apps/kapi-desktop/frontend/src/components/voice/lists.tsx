// The repeating parts of a voice profile: term rules, style patterns and
// examples.
//
// Each list is edited by the same three components wherever it appears, so a
// locale override's vocabulary is edited exactly as the profile's own is, and
// an override is not a second, thinner surface for the same decision.

import { memo } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Button, Input, Textarea, TagInput, Switch, Label, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { Field, NumberField, ValueField } from "./fields";
import type { FieldValueSet, Pattern, TermRule, VoiceExample } from "../../types/voice";

export type ValueSets = Record<string, FieldValueSet | undefined>;

/** A list of things, each editable, with add and remove. */
function ListFrame<T>({
  label,
  items,
  onChange,
  blank,
  addLabel,
  emptyHint,
  renderItem,
  testid,
}: {
  label: string;
  items: T[];
  onChange: (next: T[]) => void;
  blank: () => T;
  addLabel: string;
  emptyHint: string;
  renderItem: (item: T, update: (next: T) => void) => React.ReactNode;
  testid: string;
}) {
  const replace = (index: number, next: T) => {
    const copy = [...items];
    copy[index] = next;
    onChange(copy);
  };
  return (
    <div className="space-y-2" data-testid={testid}>
      <div className="flex items-center justify-between">
        <Label className="text-xs font-medium">{label}</Label>
        <Button variant="ghost" size="sm" onClick={() => onChange([...items, blank()])}>
          <Plus className="mr-1 size-3" />
          {addLabel}
        </Button>
      </div>
      {items.length === 0 ? (
        <p className="text-xs text-muted-foreground">{emptyHint}</p>
      ) : (
        <ul className="space-y-2">
          {items.map((item, i) => (
            <li key={i} className="rounded-lg border p-3">
              <div className="flex items-start gap-2">
                <div className="min-w-0 flex-1">{renderItem(item, (next) => replace(i, next))}</div>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("Remove")}
                  onClick={() => onChange(items.filter((_, j) => j !== i))}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** Term rules: one term, what to use instead, and how hard it bites. */
function TermRuleListEditorInner({
  label,
  rules,
  onChange,
  sets,
  emptyHint,
  testid = "term-rule-list",
}: {
  label: string;
  rules: TermRule[];
  onChange: (next: TermRule[]) => void;
  sets: ValueSets;
  emptyHint: string;
  testid?: string;
}) {
  return (
    <ListFrame
      label={label}
      items={rules}
      onChange={onChange}
      blank={(): TermRule => ({ term: "" })}
      addLabel={t("Add rule")}
      emptyHint={emptyHint}
      testid={testid}
      renderItem={(rule, update) => (
        <div className="space-y-2">
          <div className="grid gap-2 sm:grid-cols-3">
            <Field label={t("Term")}>
              <Input
                value={rule.term}
                onChange={(e) => update({ ...rule, term: e.target.value })}
                aria-label={t("Term")}
              />
            </Field>
            <Field label={t("Say this instead")} hint={t("Leave empty and tools skip the rule.")}>
              <Input
                value={rule.replacement ?? ""}
                onChange={(e) => update({ ...rule, replacement: e.target.value })}
                aria-label={t("Say this instead")}
              />
            </Field>
            <ValueField
              id={`severity-${rule.term}`}
              label={t("Severity")}
              value={rule.severity ?? ""}
              onChange={(v) => update({ ...rule, severity: v })}
              set={sets.severity}
            />
          </div>
          <Field label={t("Note")}>
            <Input
              value={rule.note ?? ""}
              onChange={(e) => update({ ...rule, note: e.target.value })}
              aria-label={t("Note")}
            />
          </Field>
          <div className="grid gap-2 sm:grid-cols-2">
            <Field label={t("Other forms")} hint={t("Spellings the matcher should also catch.")}>
              <TagInput
                value={rule.forms ?? []}
                onChange={(forms) => update({ ...rule, forms })}
                placeholder={t("Add a form...")}
              />
            </Field>
            <div className="space-y-2">
              <ValueField
                id={`scope-${rule.term}`}
                label={t("Where it applies")}
                value={rule.scope ?? ""}
                onChange={(v) => update({ ...rule, scope: v })}
                set={sets.scope}
              />
              <Field label={t("Concept")} hint={t("Ties the rule to the terms store.")}>
                <Input
                  value={rule.concept_id ?? ""}
                  onChange={(e) => update({ ...rule, concept_id: e.target.value })}
                  aria-label={t("Concept")}
                />
              </Field>
            </div>
          </div>
          <div className="flex flex-wrap gap-4">
            <label className="flex items-center gap-2 text-xs">
              <Switch
                checked={!!rule.do_not_translate}
                onCheckedChange={(v) => update({ ...rule, do_not_translate: v })}
                aria-label={t("Keep as written in every language")}
              />
              {t("Keep as written in every language")}
            </label>
            <label className="flex items-center gap-2 text-xs">
              <Switch
                checked={!!rule.case_sensitive}
                onCheckedChange={(v) => update({ ...rule, case_sensitive: v })}
                aria-label={t("Match case")}
              />
              {t("Match case")}
            </label>
          </div>
        </div>
      )}
    />
  );
}

/** Style patterns: a regular expression, why, and how often it may appear. */
function PatternListEditorInner({
  label,
  patterns,
  onChange,
  sets,
  emptyHint,
  testid = "pattern-list",
}: {
  label: string;
  patterns: Pattern[];
  onChange: (next: Pattern[]) => void;
  sets: ValueSets;
  emptyHint: string;
  testid?: string;
}) {
  return (
    <ListFrame
      label={label}
      items={patterns}
      onChange={onChange}
      blank={(): Pattern => ({ regex: "", description: "", severity: "" })}
      addLabel={t("Add pattern")}
      emptyHint={emptyHint}
      testid={testid}
      renderItem={(pattern, update) => (
        <div className="space-y-2">
          <div className="grid gap-2 sm:grid-cols-2">
            <Field label={t("Pattern")}>
              <Input
                className="font-mono text-xs"
                value={pattern.regex}
                onChange={(e) => update({ ...pattern, regex: e.target.value })}
                aria-label={t("Pattern")}
              />
            </Field>
            <Field label={t("Why")}>
              <Input
                value={pattern.description}
                onChange={(e) => update({ ...pattern, description: e.target.value })}
                aria-label={t("Why")}
              />
            </Field>
          </div>
          <div className="grid gap-2 sm:grid-cols-4">
            <ValueField
              id={`pattern-severity-${pattern.regex}`}
              label={t("Severity")}
              value={pattern.severity}
              onChange={(v) => update({ ...pattern, severity: v })}
              set={sets.severity}
            />
            <ValueField
              id={`pattern-scope-${pattern.regex}`}
              label={t("Where it applies")}
              value={pattern.scope ?? ""}
              onChange={(v) => update({ ...pattern, scope: v })}
              set={sets.scope}
            />
            <NumberField
              label={t("Allowed uses")}
              value={pattern.rate?.max}
              min={1}
              hint={t("Leave empty to forbid it outright.")}
              onChange={(max) =>
                update({
                  ...pattern,
                  rate: max === undefined ? undefined : { ...pattern.rate, max },
                })
              }
            />
            <NumberField
              label={t("Per words")}
              value={pattern.rate?.per_words}
              min={1}
              onChange={(per) =>
                update({
                  ...pattern,
                  rate: pattern.rate ? { ...pattern.rate, per_words: per } : undefined,
                })
              }
            />
          </div>
        </div>
      )}
    />
  );
}

/** Examples: the voice shown rather than described. */
function ExampleListEditorInner({
  label,
  examples,
  onChange,
  sets,
  emptyHint,
  testid = "example-list",
}: {
  label: string;
  examples: VoiceExample[];
  onChange: (next: VoiceExample[]) => void;
  sets: ValueSets;
  emptyHint: string;
  testid?: string;
}) {
  return (
    <ListFrame
      label={label}
      items={examples}
      onChange={onChange}
      blank={(): VoiceExample => ({ before: "", after: "" })}
      addLabel={t("Add example")}
      emptyHint={emptyHint}
      testid={testid}
      renderItem={(example, update) => (
        <div className="space-y-2">
          <div className="grid gap-2 sm:grid-cols-2">
            <Field label={t("Written as")}>
              <Textarea
                rows={2}
                value={example.before}
                onChange={(e) => update({ ...example, before: e.target.value })}
                aria-label={t("Written as")}
              />
            </Field>
            <Field label={t("Should read")}>
              <Textarea
                rows={2}
                value={example.after}
                onChange={(e) => update({ ...example, after: e.target.value })}
                aria-label={t("Should read")}
              />
            </Field>
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            <Field label={t("Why")}>
              <Input
                value={example.explanation ?? ""}
                onChange={(e) => update({ ...example, explanation: e.target.value })}
                aria-label={t("Why")}
              />
            </Field>
            <ValueField
              id={`example-category-${example.before}`}
              label={t("Category")}
              value={example.category ?? ""}
              onChange={(v) => update({ ...example, category: v })}
              set={sets["examples.category"]}
            />
          </div>
        </div>
      )}
    />
  );
}

/** Abbreviations: a short form and what it stands for. */
function AbbreviationsEditorInner({
  abbreviations,
  onChange,
}: {
  abbreviations: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
}) {
  const rows = Object.entries(abbreviations);
  const rename = (from: string, to: string) => {
    const next: Record<string, string> = {};
    for (const [k, v] of rows) next[k === from ? to : k] = v;
    onChange(next);
  };
  return (
    <div className="space-y-2" data-testid="abbreviation-list">
      <div className="flex items-center justify-between">
        <Label className="text-xs font-medium">{t("Abbreviations")}</Label>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onChange({ ...abbreviations, "": "" })}
          disabled={"" in abbreviations}
        >
          <Plus className="mr-1 size-3" />
          {t("Add abbreviation")}
        </Button>
      </div>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("This profile expands no abbreviations.")}
        </p>
      ) : (
        <ul className="space-y-1">
          {rows.map(([short, long], i) => (
            <li key={i} className={cn("flex items-center gap-2")}>
              <Input
                className="w-32"
                value={short}
                placeholder={t("API")}
                onChange={(e) => rename(short, e.target.value)}
                aria-label={t("Abbreviation")}
              />
              <Input
                value={long}
                placeholder={t("what it stands for")}
                onChange={(e) => onChange({ ...abbreviations, [short]: e.target.value })}
                aria-label={t("Expansion")}
              />
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t("Remove")}
                onClick={() => {
                  const next = { ...abbreviations };
                  delete next[short];
                  onChange(next);
                }}
              >
                <Trash2 className="size-3.5" />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// Memoized: a keystroke elsewhere in the profile must not re-render a list
// whose own slice did not move.
export const TermRuleListEditor = memo(TermRuleListEditorInner);
export const PatternListEditor = memo(PatternListEditorInner);
export const ExampleListEditor = memo(ExampleListEditorInner);
export const AbbreviationsEditor = memo(AbbreviationsEditorInner);
