// The voice profile governing content at a point, as one picker.
//
// Every surface binds a voice the same way: one row for "inherit what governs
// above", then the profiles the surface can offer. What a key means is the
// consumer's business: a recipe encodes a file, a starter pack or a store
// profile into it, a platform passes a profile id. The picker never decodes.

import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "../../lib/utils";
import { Label } from "../ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "../ui/select";

/** One profile a surface can bind. */
export interface VoiceBindingOption {
  /** The key handed back on change; opaque to the picker. */
  value: string;
  label: string;
  /** Options sharing a group are listed under that heading. */
  group?: string;
  /** A short muted qualifier after the label, such as "read-only". */
  hint?: string;
}

export interface VoiceBindingSelectProps {
  /** The bound key; empty or absent means nothing is bound here. */
  value?: string;
  /** Called with the chosen key, or undefined when the binding is cleared. */
  onChange: (next: string | undefined) => void;
  options: VoiceBindingOption[];
  /** What the unbound row says: "None bound", "Workspace default", "Inherit (project)". */
  inheritLabel: string;
  /** The field label; defaults to "Voice profile". */
  label?: string;
  /** A muted note beneath the picker. */
  help?: string;
  disabled?: boolean;
  /** Class for the trigger, typically a width. */
  className?: string;
  id?: string;
}

const NONE = "__none__";

export function VoiceBindingSelect({
  value,
  onChange,
  options,
  inheritLabel,
  label,
  help,
  disabled,
  className,
  id,
}: VoiceBindingSelectProps) {
  const fieldLabel = label ?? t("Voice profile");
  const current = value ? value : NONE;
  const known = options.some((o) => o.value === value);

  const ungrouped = options.filter((o) => !o.group);
  const groups: string[] = [];
  for (const o of options) {
    if (o.group && !groups.includes(o.group)) groups.push(o.group);
  }

  const item = (o: VoiceBindingOption) => (
    <SelectItem key={o.value} value={o.value}>
      {o.label}
      {o.hint && <span className="ml-1.5 text-xs text-muted-foreground">{o.hint}</span>}
    </SelectItem>
  );

  return (
    <div className="space-y-1" data-slot="voice-binding">
      <Label htmlFor={id} className="mb-1 block text-xs text-muted-foreground">
        {fieldLabel}
      </Label>
      <Select
        value={current}
        disabled={disabled}
        onValueChange={(v) => onChange(v === NONE ? undefined : v)}
      >
        <SelectTrigger id={id} className={cn("max-w-md", className)} aria-label={fieldLabel}>
          <SelectValue placeholder={inheritLabel} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE}>{inheritLabel}</SelectItem>
          {value && !known && (
            <SelectItem value={value} data-testid="voice-binding-missing">
              {value}
              <span className="ml-1.5 text-xs text-destructive">{t("not found")}</span>
            </SelectItem>
          )}
          {ungrouped.map(item)}
          {groups.map((g) => (
            <SelectGroup key={g}>
              <SelectLabel>{g}</SelectLabel>
              {options.filter((o) => o.group === g).map(item)}
            </SelectGroup>
          ))}
        </SelectContent>
      </Select>
      {help && <p className="text-[10px] text-muted-foreground">{help}</p>}
    </div>
  );
}
