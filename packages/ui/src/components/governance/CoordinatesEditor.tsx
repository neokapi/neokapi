// A point, or a region of one, as rows of axis and value.
//
// A recipe's default point, a collection's own point and a membership's region
// are the same shape: an open map from axis to value. This editor draws that
// map the same way on every surface and hands the map back; what may be
// declared where is the consumer's vocabulary, passed in as the axes.

import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";

/** One axis the consumer knows about. */
export interface CoordinateAxisOption {
  axis: string;
  /**
   * False for an axis the consumer derives rather than declares; `refusal` is
   * what it says when someone tries. Absent means declarable.
   */
  declarable?: boolean;
  refusal?: string;
  /** A closed set of values: the row offers them as a select. */
  values?: string[];
}

export interface CoordinatesEditorProps {
  value: Record<string, string>;
  /** The next map, or undefined once the last axis is removed. */
  onChange: (next: Record<string, string> | undefined) => void;
  /** The axes the consumer knows: what may be declared, what is refused, and which values are closed. */
  axes?: CoordinateAxisOption[];
  /**
   * Offer a free-text axis rather than only the declarable ones. A recipe may
   * mint an axis; a collection picks from what the recipe declared.
   */
  allowNewAxis?: boolean;
  /** Field label above the rows. */
  label?: string;
  /** Shown instead of rows when the map is empty. */
  emptyText?: string;
  /** A muted note beneath the editor. */
  note?: string;
  /**
   * Mark a row with no value. A region that names an axis without a value
   * constrains nothing on it, so a consumer saving one refuses instead; see
   * `incompleteAxes`.
   */
  requireValues?: boolean;
  disabled?: boolean;
  className?: string;
  testId?: string;
}

/** The axes of a map whose value is blank, for a consumer that refuses to save one. */
export function incompleteAxes(value: Record<string, string> | undefined): string[] {
  return Object.entries(value ?? {})
    .filter(([, v]) => v.trim() === "")
    .map(([axis]) => axis);
}

export function CoordinatesEditor({
  value,
  onChange,
  axes = [],
  allowNewAxis = false,
  label,
  emptyText,
  note,
  requireValues = false,
  disabled = false,
  className,
  testId = "coordinates-editor",
}: CoordinatesEditorProps) {
  const [axis, setAxis] = useState("");
  const rows = Object.entries(value);
  const typed = axis.trim();
  const refusal = axes.find((a) => a.axis === typed && a.declarable === false)?.refusal;
  const offered = axes.filter((a) => a.declarable !== false && !(a.axis in value));
  const canAdd = typed !== "" && !refusal && !(typed in value);

  const set = (next: Record<string, string>) =>
    onChange(Object.keys(next).length ? next : undefined);

  return (
    <div className={cn("space-y-2", className)} data-testid={testId}>
      {label && <Label className="block text-xs text-muted-foreground">{label}</Label>}
      {rows.length === 0 ? (
        emptyText && <p className="text-xs text-muted-foreground">{emptyText}</p>
      ) : (
        <ul className="space-y-1">
          {rows.map(([a, v]) => {
            const known = axes.find((x) => x.axis === a);
            const missing = requireValues && v.trim() === "";
            return (
              <li key={a} className="space-y-0.5" data-testid="coordinate-row">
                <div className="flex items-center gap-2">
                  <code className="w-28 shrink-0 font-mono text-xs">{a}</code>
                  {known?.values?.length ? (
                    <Select
                      value={v}
                      disabled={disabled}
                      onValueChange={(next) => set({ ...value, [a]: next })}
                    >
                      <SelectTrigger className="max-w-xs" aria-label={a}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {known.values.map((opt) => (
                          <SelectItem key={opt} value={opt}>
                            {opt}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <Input
                      className="max-w-xs"
                      value={v}
                      aria-label={a}
                      aria-invalid={missing || undefined}
                      disabled={disabled}
                      onChange={(e) => set({ ...value, [a]: e.target.value })}
                    />
                  )}
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t("Remove {axis}", { axis: a })}
                    disabled={disabled}
                    onClick={() => {
                      const next = { ...value };
                      delete next[a];
                      set(next);
                    }}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
                {missing && (
                  <p className="text-xs text-destructive" data-testid="coordinate-value-required">
                    {t("{axis} needs a value.", { axis: a })}
                  </p>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {(allowNewAxis || offered.length > 0) && (
        <div className="flex items-center gap-2">
          {allowNewAxis ? (
            <Input
              className="max-w-[12rem]"
              placeholder={t("axis, e.g. brand")}
              value={axis}
              aria-label={t("New axis")}
              disabled={disabled}
              onChange={(e) => setAxis(e.target.value)}
            />
          ) : (
            <Select value={axis} disabled={disabled} onValueChange={setAxis}>
              <SelectTrigger className="max-w-[12rem]" aria-label={t("New axis")}>
                <SelectValue placeholder={t("axis")} />
              </SelectTrigger>
              <SelectContent>
                {offered.map((a) => (
                  <SelectItem key={a.axis} value={a.axis}>
                    {a.axis}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled || !canAdd}
            onClick={() => {
              set({ ...value, [typed]: "" });
              setAxis("");
            }}
          >
            <Plus className="mr-1 size-3" />
            {t("Add axis")}
          </Button>
        </div>
      )}
      {refusal && (
        <p className="text-xs text-destructive" data-testid="axis-refusal">
          {refusal}
        </p>
      )}
      {note && <p className="text-[10px] text-muted-foreground">{note}</p>}
    </div>
  );
}
