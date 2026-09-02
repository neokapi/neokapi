import {
  Button,
  Card,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
} from "@neokapi/ui-primitives";
import { useCallback, useState } from "react";
import type { PersonaOverride, ToneProfile, StyleRules, TermRule } from "./types";
import { Plus, Trash2 } from "../components/icons";

interface PersonasEditorProps {
  personas: Record<string, PersonaOverride>;
  onChange: (personas: Record<string, PersonaOverride>) => void;
  /** Brand tone/style used to seed a new tone/style override so it starts from
   * the voice rather than a blank slate. */
  seedTone: ToneProfile;
  seedStyle: StyleRules;
}

type TermField = "preferred_terms" | "avoided_terms";

const termLabels: Record<TermField, string> = {
  preferred_terms: "Preferred terms",
  avoided_terms: "Avoided terms",
};

const termDescriptions: Record<TermField, string> = {
  preferred_terms:
    "Words this author leans on (added on top of the brand's; a brand-forbidden term is never re-allowed)",
  avoided_terms:
    "Words this author personally steers clear of (added to the brand's forbidden set)",
};

/**
 * PersonasEditor edits the map of author personas layered on a voice profile.
 * Each persona optionally overrides tone and style (replacing the brand's) and
 * carries additive preferred/avoided vocabulary. The brand's guardrails always
 * win: a persona can tighten vocabulary but never loosen it.
 */
export function PersonasEditor({ personas, onChange, seedTone, seedStyle }: PersonasEditorProps) {
  const [newName, setNewName] = useState("");

  const keys = Object.keys(personas);

  const addPersona = useCallback(() => {
    const key = newName.trim();
    if (!key || personas[key]) return;
    onChange({ ...personas, [key]: {} });
    setNewName("");
  }, [newName, personas, onChange]);

  const removePersona = useCallback(
    (key: string) => {
      const next = { ...personas };
      delete next[key];
      onChange(next);
    },
    [personas, onChange],
  );

  const updatePersona = useCallback(
    (key: string, patch: PersonaOverride) => {
      onChange({ ...personas, [key]: { ...personas[key], ...patch } });
    },
    [personas, onChange],
  );

  return (
    <div className="space-y-4">
      <div>
        <Label>Personas</Label>
        <p className="text-xs text-muted-foreground mt-0.5">
          Give individual authors a personal voice inside the brand's guardrails. Select a persona
          when checking or rewriting to apply it. Attribution is explicit, never automatic.
        </p>
      </div>

      <div className="flex gap-2">
        <Input
          value={newName}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewName(e.target.value)}
          placeholder="Persona name (e.g. jordan, editorial-lead)"
          onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) =>
            e.key === "Enter" && (e.preventDefault(), addPersona())
          }
        />
        <Button variant="outline" size="sm" onClick={addPersona} disabled={!newName.trim()}>
          <Plus className="w-3.5 h-3.5 mr-1" /> Add persona
        </Button>
      </div>

      {keys.length === 0 && <p className="text-xs text-muted-foreground">No personas defined.</p>}

      {keys.map((key) => (
        <PersonaCard
          key={key}
          name={key}
          persona={personas[key]}
          seedTone={seedTone}
          seedStyle={seedStyle}
          onRemove={() => removePersona(key)}
          onUpdate={(patch) => updatePersona(key, patch)}
        />
      ))}
    </div>
  );
}

interface PersonaCardProps {
  name: string;
  persona: PersonaOverride;
  seedTone: ToneProfile;
  seedStyle: StyleRules;
  onRemove: () => void;
  onUpdate: (patch: PersonaOverride) => void;
}

function PersonaCard({ name, persona, seedTone, seedStyle, onRemove, onUpdate }: PersonaCardProps) {
  const toggleTone = useCallback(
    (on: boolean) => onUpdate({ tone: on ? { ...seedTone } : undefined }),
    [onUpdate, seedTone],
  );
  const toggleStyle = useCallback(
    (on: boolean) => onUpdate({ style: on ? { ...seedStyle } : undefined }),
    [onUpdate, seedStyle],
  );

  const setTone = useCallback(
    (patch: Partial<ToneProfile>) => {
      if (!persona.tone) return;
      onUpdate({ tone: { ...persona.tone, ...patch } });
    },
    [onUpdate, persona.tone],
  );
  const setStyle = useCallback(
    (patch: Partial<StyleRules>) => {
      if (!persona.style) return;
      onUpdate({ style: { ...persona.style, ...patch } });
    },
    [onUpdate, persona.style],
  );

  return (
    <Card className="p-4 space-y-4">
      <div className="flex items-center justify-between">
        <div className="text-sm font-semibold">{name}</div>
        <Button variant="ghost" size="icon" onClick={onRemove} aria-label={`Remove ${name}`}>
          <Trash2 className="w-3.5 h-3.5" />
        </Button>
      </div>

      {/* Tone override */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <div>
            <Label className="text-xs">Override tone</Label>
            <p className="text-[11px] text-muted-foreground">
              Replaces the brand tone for this author
            </p>
          </div>
          <Switch checked={persona.tone != null} onCheckedChange={toggleTone} />
        </div>
        {persona.tone && (
          <div className="grid grid-cols-3 gap-3">
            <ToneSelect
              label="Formality"
              value={persona.tone.formality}
              options={["casual", "neutral", "formal", "technical"]}
              onChange={(v) => setTone({ formality: v as ToneProfile["formality"] })}
            />
            <ToneSelect
              label="Emotion"
              value={persona.tone.emotion}
              options={["warm", "neutral", "authoritative"]}
              onChange={(v) => setTone({ emotion: v as ToneProfile["emotion"] })}
            />
            <ToneSelect
              label="Humor"
              value={persona.tone.humor}
              options={["none", "light", "frequent"]}
              onChange={(v) => setTone({ humor: v as ToneProfile["humor"] })}
            />
          </div>
        )}
      </div>

      {/* Style override */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <div>
            <Label className="text-xs">Override style</Label>
            <p className="text-[11px] text-muted-foreground">
              Replaces the brand style for this author
            </p>
          </div>
          <Switch checked={persona.style != null} onCheckedChange={toggleStyle} />
        </div>
        {persona.style && (
          <div className="grid grid-cols-2 gap-3">
            <ToneSelect
              label="Point of view"
              value={persona.style.person_pov}
              options={["first_plural", "second", "third"]}
              onChange={(v) => setStyle({ person_pov: v as StyleRules["person_pov"] })}
            />
            <ToneSelect
              label="Contractions"
              value={persona.style.contractions}
              options={["always", "sometimes", "never"]}
              onChange={(v) => setStyle({ contractions: v as StyleRules["contractions"] })}
            />
          </div>
        )}
      </div>

      <PersonaTermList
        field="preferred_terms"
        terms={persona.preferred_terms ?? []}
        onChange={(preferred_terms) => onUpdate({ preferred_terms })}
      />
      <PersonaTermList
        field="avoided_terms"
        terms={persona.avoided_terms ?? []}
        onChange={(avoided_terms) => onUpdate({ avoided_terms })}
      />
    </Card>
  );
}

interface ToneSelectProps {
  label: string;
  value: string;
  options: string[];
  onChange: (value: string) => void;
}

function ToneSelect({ label, value, options, onChange }: ToneSelectProps) {
  return (
    <div className="space-y-1">
      <Label className="text-xs">{label}</Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((opt) => (
            <SelectItem key={opt} value={opt}>
              {opt}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

interface PersonaTermListProps {
  field: TermField;
  terms: TermRule[];
  onChange: (terms: TermRule[]) => void;
}

function PersonaTermList({ field, terms, onChange }: PersonaTermListProps) {
  const add = useCallback(() => {
    onChange([...terms, { term: "", replacement: "", note: "" }]);
  }, [terms, onChange]);

  const update = useCallback(
    (index: number, key: keyof TermRule, value: string) => {
      const list = [...terms];
      list[index] = { ...list[index], [key]: value };
      onChange(list);
    },
    [terms, onChange],
  );

  const remove = useCallback(
    (index: number) => {
      const list = [...terms];
      list.splice(index, 1);
      onChange(list);
    },
    [terms, onChange],
  );

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div>
          <Label className="text-xs">{termLabels[field]}</Label>
          <p className="text-[11px] text-muted-foreground">{termDescriptions[field]}</p>
        </div>
        <Button variant="outline" size="sm" onClick={add}>
          <Plus className="w-3.5 h-3.5 mr-1" /> Add
        </Button>
      </div>
      {terms.length === 0 && <p className="text-xs text-muted-foreground">No terms defined.</p>}
      {terms.map((rule, i) => (
        <div key={i} className="flex gap-2 items-start">
          <Input
            placeholder="Term"
            value={rule.term}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => update(i, "term", e.target.value)}
          />
          <Input
            placeholder="Replacement"
            value={rule.replacement ?? ""}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
              update(i, "replacement", e.target.value)
            }
          />
          <Input
            placeholder="Note"
            value={rule.note ?? ""}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => update(i, "note", e.target.value)}
          />
          <Button variant="ghost" size="icon" onClick={() => remove(i)} aria-label="Remove term">
            <Trash2 className="w-3.5 h-3.5" />
          </Button>
        </div>
      ))}
    </div>
  );
}
