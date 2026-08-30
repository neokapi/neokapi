// Tone and style, and the override maps built from them.
//
// A channel override carries a tone and a style; a persona override carries
// those plus two term-rule lists; a locale override carries the scalars that
// change per language plus vocabulary and examples. Each is edited with the
// same components as the profile's own, so nothing is editable at one scope
// and read-only at another.

import { memo } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Button, Input, Textarea, TagInput, Switch, Label } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { Field, ValueField } from "./fields";
import { ExampleListEditor, PatternListEditor, TermRuleListEditor, type ValueSets } from "./lists";
import type {
  ChannelOverride,
  LocaleOverride,
  PersonaOverride,
  StyleRules,
  ToneProfile,
} from "../../types/voice";

function ToneEditorInner({
  tone,
  onChange,
  sets,
  idPrefix,
}: {
  tone: ToneProfile;
  onChange: (next: ToneProfile) => void;
  sets: ValueSets;
  idPrefix: string;
}) {
  return (
    <div className="space-y-3" data-testid={`tone-editor-${idPrefix}`}>
      <Field label={t("Personality")} hint={t("The handful of words that describe the voice.")}>
        <TagInput
          value={tone.personality ?? []}
          onChange={(personality) => onChange({ ...tone, personality })}
          placeholder={t("Add a trait...")}
        />
      </Field>
      <div className="grid gap-2 sm:grid-cols-3">
        <ValueField
          id={`${idPrefix}-formality`}
          label={t("Formality")}
          value={tone.formality ?? ""}
          onChange={(formality) => onChange({ ...tone, formality })}
          set={sets["tone.formality"]}
        />
        <ValueField
          id={`${idPrefix}-emotion`}
          label={t("Emotion")}
          value={tone.emotion ?? ""}
          onChange={(emotion) => onChange({ ...tone, emotion })}
          set={sets["tone.emotion"]}
        />
        <ValueField
          id={`${idPrefix}-humor`}
          label={t("Humor")}
          value={tone.humor ?? ""}
          onChange={(humor) => onChange({ ...tone, humor })}
          set={sets["tone.humor"]}
        />
      </div>
      <Field
        label={t("In your own words")}
        hint={t("Prose reaches the model as written, so say what the labels cannot.")}
      >
        <Textarea
          rows={3}
          value={tone.guidelines ?? ""}
          onChange={(e) => onChange({ ...tone, guidelines: e.target.value })}
          aria-label={t("In your own words")}
        />
      </Field>
    </div>
  );
}

function StyleEditorInner({
  style,
  onChange,
  sets,
  idPrefix,
  patterns = true,
}: {
  style: StyleRules;
  onChange: (next: StyleRules) => void;
  sets: ValueSets;
  idPrefix: string;
  /** Pattern lists belong to the profile's own style, not to every override. */
  patterns?: boolean;
}) {
  return (
    <div className="space-y-3" data-testid={`style-editor-${idPrefix}`}>
      <div className="grid gap-2 sm:grid-cols-3">
        <ValueField
          id={`${idPrefix}-sentence-length`}
          label={t("Sentences")}
          value={style.sentence_length ?? ""}
          onChange={(sentence_length) => onChange({ ...style, sentence_length })}
          set={sets["style.sentence_length"]}
        />
        <ValueField
          id={`${idPrefix}-person-pov`}
          label={t("Point of view")}
          value={style.person_pov ?? ""}
          onChange={(person_pov) => onChange({ ...style, person_pov })}
          set={sets["style.person_pov"]}
        />
        <ValueField
          id={`${idPrefix}-contractions`}
          label={t("Contractions")}
          value={style.contractions ?? ""}
          onChange={(contractions) => onChange({ ...style, contractions })}
          set={sets["style.contractions"]}
        />
      </div>
      <label className="flex items-center gap-2 text-xs">
        <Switch
          checked={!!style.active_voice}
          onCheckedChange={(active_voice) => onChange({ ...style, active_voice })}
          aria-label={t("Prefer the active voice")}
        />
        {t("Prefer the active voice")}
      </label>
      {patterns && (
        <>
          <PatternListEditor
            label={t("Never write")}
            patterns={style.prohibited_patterns ?? []}
            onChange={(prohibited_patterns) => onChange({ ...style, prohibited_patterns })}
            sets={sets}
            emptyHint={t("No pattern is forbidden.")}
            testid="prohibited-patterns"
          />
          <PatternListEditor
            label={t("Always write")}
            patterns={style.required_patterns ?? []}
            onChange={(required_patterns) => onChange({ ...style, required_patterns })}
            sets={sets}
            emptyHint={t("No pattern is required.")}
            testid="required-patterns"
          />
        </>
      )}
    </div>
  );
}

/** A keyed map of overrides, each edited by the same components as the base. */
function OverrideMap<T>({
  label,
  keyLabel,
  keyPlaceholder,
  entries,
  onChange,
  blank,
  emptyHint,
  renderBody,
  testid,
}: {
  label: string;
  keyLabel: string;
  keyPlaceholder: string;
  entries: Record<string, T>;
  onChange: (next: Record<string, T>) => void;
  blank: () => T;
  emptyHint: string;
  renderBody: (key: string, value: T, update: (next: T) => void) => React.ReactNode;
  testid: string;
}) {
  const rows = Object.entries(entries);
  const rename = (from: string, to: string) => {
    const next: Record<string, T> = {};
    for (const [k, v] of rows) next[k === from ? to : k] = v;
    onChange(next);
  };
  return (
    <div className="space-y-2" data-testid={testid}>
      <div className="flex items-center justify-between">
        <Label className="text-xs font-medium">{label}</Label>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onChange({ ...entries, "": blank() })}
          disabled={"" in entries}
        >
          <Plus className="mr-1 size-3" />
          {t("Add")}
        </Button>
      </div>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{emptyHint}</p>
      ) : (
        <ul className="space-y-2">
          {rows.map(([key, value], i) => (
            <li key={i} className="rounded-lg border p-3">
              <div className="mb-2 flex items-center gap-2">
                <Input
                  className="w-48"
                  value={key}
                  placeholder={keyPlaceholder}
                  onChange={(e) => rename(key, e.target.value)}
                  aria-label={keyLabel}
                />
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("Remove")}
                  onClick={() => {
                    const next = { ...entries };
                    delete next[key];
                    onChange(next);
                  }}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
              {renderBody(key, value, (next) => onChange({ ...entries, [key]: next }))}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function LocaleOverridesEditorInner({
  locales,
  onChange,
  sets,
}: {
  locales: Record<string, LocaleOverride>;
  onChange: (next: Record<string, LocaleOverride>) => void;
  sets: ValueSets;
}) {
  return (
    <OverrideMap
      label={t("By language")}
      keyLabel={t("Language")}
      keyPlaceholder="nb-NO"
      entries={locales}
      onChange={onChange}
      blank={(): LocaleOverride => ({})}
      emptyHint={t("The voice sounds the same in every language.")}
      testid="locale-overrides"
      renderBody={(key, value, update) => (
        <div className="space-y-3">
          <div className="grid gap-2 sm:grid-cols-3">
            <ValueField
              id={`locale-${key}-formality`}
              label={t("Formality")}
              value={value.formality ?? ""}
              onChange={(formality) => update({ ...value, formality })}
              set={sets["tone.formality"]}
            />
            <ValueField
              id={`locale-${key}-humor`}
              label={t("Humor")}
              value={value.humor ?? ""}
              onChange={(humor) => update({ ...value, humor })}
              set={sets["tone.humor"]}
            />
            <ValueField
              id={`locale-${key}-pov`}
              label={t("Point of view")}
              value={value.person_pov ?? ""}
              onChange={(person_pov) => update({ ...value, person_pov })}
              set={sets["style.person_pov"]}
            />
          </div>
          <Field label={t("What a reader here expects")}>
            <Textarea
              rows={2}
              value={value.cultural_notes ?? ""}
              onChange={(e) => update({ ...value, cultural_notes: e.target.value })}
              aria-label={t("What a reader here expects")}
            />
          </Field>
          <TermRuleListEditor
            label={t("Wording here")}
            rules={value.vocabulary_overrides ?? []}
            onChange={(vocabulary_overrides) => update({ ...value, vocabulary_overrides })}
            sets={sets}
            emptyHint={t("The profile's wording applies unchanged.")}
            testid={`locale-${key}-terms`}
          />
          <ExampleListEditor
            label={t("Examples here")}
            examples={value.example_overrides ?? []}
            onChange={(example_overrides) => update({ ...value, example_overrides })}
            sets={sets}
            emptyHint={t("The profile's examples apply unchanged.")}
            testid={`locale-${key}-examples`}
          />
        </div>
      )}
    />
  );
}

function ChannelOverridesEditorInner({
  channels,
  onChange,
  sets,
}: {
  channels: Record<string, ChannelOverride>;
  onChange: (next: Record<string, ChannelOverride>) => void;
  sets: ValueSets;
}) {
  return (
    <OverrideMap
      label={t("By channel")}
      keyLabel={t("Channel")}
      keyPlaceholder="docs"
      entries={channels}
      onChange={onChange}
      blank={(): ChannelOverride => ({})}
      emptyHint={t("The voice sounds the same on every channel.")}
      testid="channel-overrides"
      renderBody={(key, value, update) => (
        <div className="space-y-3">
          <ToneEditor
            tone={value.tone ?? {}}
            onChange={(tone) => update({ ...value, tone })}
            sets={sets}
            idPrefix={`channel-${key}`}
          />
          <StyleEditor
            style={value.style ?? {}}
            onChange={(style) => update({ ...value, style })}
            sets={sets}
            idPrefix={`channel-${key}`}
            patterns={false}
          />
        </div>
      )}
    />
  );
}

function PersonaOverridesEditorInner({
  personas,
  onChange,
  sets,
}: {
  personas: Record<string, PersonaOverride>;
  onChange: (next: Record<string, PersonaOverride>) => void;
  sets: ValueSets;
}) {
  return (
    <OverrideMap
      label={t("By persona")}
      keyLabel={t("Persona")}
      keyPlaceholder="support-agent"
      entries={personas}
      onChange={onChange}
      blank={(): PersonaOverride => ({})}
      emptyHint={t("Every author writes in the profile's own voice.")}
      testid="persona-overrides"
      renderBody={(key, value, update) => (
        <div className="space-y-3">
          <ToneEditor
            tone={value.tone ?? {}}
            onChange={(tone) => update({ ...value, tone })}
            sets={sets}
            idPrefix={`persona-${key}`}
          />
          <StyleEditor
            style={value.style ?? {}}
            onChange={(style) => update({ ...value, style })}
            sets={sets}
            idPrefix={`persona-${key}`}
            patterns={false}
          />
          <TermRuleListEditor
            label={t("Say this")}
            rules={value.preferred_terms ?? []}
            onChange={(preferred_terms) => update({ ...value, preferred_terms })}
            sets={sets}
            emptyHint={t("This persona prefers nothing extra.")}
            testid={`persona-${key}-preferred`}
          />
          <TermRuleListEditor
            label={t("Never say")}
            rules={value.avoided_terms ?? []}
            onChange={(avoided_terms) => update({ ...value, avoided_terms })}
            sets={sets}
            emptyHint={t("This persona avoids nothing extra.")}
            testid={`persona-${key}-avoided`}
          />
        </div>
      )}
    />
  );
}

// Memoized: a keystroke elsewhere in the profile must not re-render a list
// whose own slice did not move.
export const ToneEditor = memo(ToneEditorInner);
export const StyleEditor = memo(StyleEditorInner);
export const LocaleOverridesEditor = memo(LocaleOverridesEditorInner);
export const ChannelOverridesEditor = memo(ChannelOverridesEditorInner);
export const PersonaOverridesEditor = memo(PersonaOverridesEditorInner);
