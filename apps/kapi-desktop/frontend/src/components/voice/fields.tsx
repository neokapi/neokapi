// The small inputs the voice editor is built from.
//
// A constrained field offers exactly the values validation applies, which the
// backend serves from the same slices `ValidateProfile` reads. Where a set is
// open — tone is described, not enumerated — the control accepts anything and
// offers the usual values as a starting point, because a register the list does
// not name is often what distinguishes one voice from another.

import {
  Input,
  Label,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  cn,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { FieldValueSet } from "../../types/voice";

/** A labelled row in the editor's form grid. */
export function Field({
  label,
  hint,
  children,
  className,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("space-y-1", className)}>
      <Label className="text-xs text-muted-foreground">{label}</Label>
      {children}
      {hint && <p className="text-[11px] text-muted-foreground">{hint}</p>}
    </div>
  );
}

const NONE = "__none__";

/**
 * A value from a constrained set.
 *
 * A closed set is a picker. An open one is a text field with the usual values
 * offered through a datalist, so an author can type a register the list does
 * not name without the control arguing.
 */
export function ValueField({
  label,
  value,
  onChange,
  set,
  id,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  set?: FieldValueSet;
  id: string;
  placeholder?: string;
}) {
  const values = set?.values ?? [];
  if (set && !set.open && values.length > 0) {
    return (
      <Field label={label}>
        <Select value={value || NONE} onValueChange={(v) => onChange(v === NONE ? "" : v)}>
          <SelectTrigger aria-label={label}>
            <SelectValue placeholder={t("unset")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE}>{t("unset")}</SelectItem>
            {values.map((v) => (
              <SelectItem key={v} value={v}>
                {v}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
    );
  }
  return (
    <Field
      label={label}
      hint={values.length ? t("Usually {values}", { values: values.join(", ") }) : undefined}
    >
      <Input
        id={id}
        list={values.length ? `${id}-values` : undefined}
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        aria-label={label}
      />
      {values.length > 0 && (
        <datalist id={`${id}-values`}>
          {values.map((v) => (
            <option key={v} value={v} />
          ))}
        </datalist>
      )}
    </Field>
  );
}

/** A number a profile carries, empty when unset. */
export function NumberField({
  label,
  value,
  onChange,
  min,
  max,
  hint,
}: {
  label: string;
  value?: number;
  onChange: (v: number | undefined) => void;
  min?: number;
  max?: number;
  hint?: string;
}) {
  return (
    <Field label={label} hint={hint}>
      <Input
        type="number"
        min={min}
        max={max}
        value={value === undefined ? "" : String(value)}
        onChange={(e) => {
          const raw = e.target.value;
          onChange(raw === "" ? undefined : Number(raw));
        }}
        aria-label={label}
      />
    </Field>
  );
}
