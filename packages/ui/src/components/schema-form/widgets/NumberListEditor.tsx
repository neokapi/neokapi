import { useState, useEffect } from "react";
import { cn } from "../../../lib/utils";
import { Input } from "../../ui/input";

// Parses a comma/space separated list of numbers. Returns the parsed numbers
// and the list of raw tokens that failed to parse so the UI can warn.
function parseNumberList(raw: string): { numbers: number[]; invalid: string[] } {
  const tokens = raw
    .split(/[\s,]+/)
    .map((t) => t.trim())
    .filter(Boolean);
  const numbers: number[] = [];
  const invalid: string[] = [];
  for (const token of tokens) {
    const n = Number(token);
    if (token === "" || Number.isNaN(n)) {
      invalid.push(token);
    } else {
      numbers.push(n);
    }
  }
  return { numbers, invalid };
}

/**
 * Editor for a list of numbers entered as a comma/space separated string.
 *
 * The schema represents this value as a string (e.g. "1, 2, 3") — matching the
 * Okapi bridge `numberList` widget and the framework's string-typed parameters
 * — so this editor preserves the raw string value while validating that every
 * token parses as a number and surfacing an inline warning otherwise.
 */
export function NumberListEditor({
  value,
  placeholder,
  disabled,
  onChange,
}: {
  value: string;
  placeholder?: string;
  disabled?: boolean;
  onChange: (value: string | undefined) => void;
}) {
  // Local state lets the user type freely (incl. trailing separators) while
  // validation runs against the committed value.
  const [draft, setDraft] = useState(value);

  // Keep the draft in sync when the controlled value changes externally
  // (preset application, drill-down navigation, programmatic reset).
  useEffect(() => {
    setDraft(value);
  }, [value]);

  const { invalid } = parseNumberList(draft);
  const hasInvalid = invalid.length > 0;

  return (
    <div className="space-y-1">
      <Input
        inputMode="numeric"
        value={draft}
        placeholder={placeholder || "1, 2, 3, ..."}
        disabled={disabled}
        className={cn("text-xs h-8 font-mono", hasInvalid && "border-destructive")}
        aria-invalid={hasInvalid}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
          const next = e.target.value;
          setDraft(next);
          onChange(next.trim() === "" ? undefined : next);
        }}
      />
      {hasInvalid && (
        <p className="text-xs text-destructive">
          Not a number: {invalid.map((t) => `"${t}"`).join(", ")}
        </p>
      )}
    </div>
  );
}
