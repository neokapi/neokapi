import { useState, useCallback } from "react";
import { cn } from "../../lib/utils";

// UI components from the ui directory
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Switch } from "../ui/switch";

// ─── Individual Field Components ────────────────────────────

interface BooleanFieldProps {
  name: string;
  description?: string;
  value: boolean | undefined;
  defaultValue?: boolean;
  onChange: (value: boolean) => void;
}

export function BooleanField({
  name,
  description,
  value,
  defaultValue,
  onChange,
}: BooleanFieldProps) {
  const checked = value ?? defaultValue ?? false;
  return (
    <div className="flex items-center justify-between">
      <div>
        <Label htmlFor={name} className="text-sm">
          {name}
        </Label>
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
      </div>
      <Switch id={name} checked={checked} onCheckedChange={onChange} />
    </div>
  );
}

interface TextFieldProps {
  name: string;
  description?: string;
  placeholder?: string;
  value: string | number | undefined;
  defaultValue?: string | number;
  type?: "text" | "number";
  mono?: boolean;
  onChange: (value: string | number) => void;
}

export function TextField({
  name,
  description,
  placeholder,
  value,
  defaultValue,
  type = "text",
  mono,
  onChange,
}: TextFieldProps) {
  const displayValue = value ?? defaultValue ?? "";
  return (
    <div className="space-y-1">
      <Label htmlFor={name} className="text-sm">
        {name}
      </Label>
      <Input
        id={name}
        type={type}
        value={displayValue}
        placeholder={placeholder}
        className={mono ? "font-mono text-xs" : undefined}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
          const v = type === "number" ? Number(e.target.value) : e.target.value;
          onChange(v);
        }}
      />
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
    </div>
  );
}

// ─── JSON Field (fallback) ──────────────────────────────────

interface JsonFieldProps {
  name: string;
  description?: string;
  value: Record<string, unknown> | unknown[] | undefined;
  onChange: (value: Record<string, unknown> | unknown[]) => void;
}

export function JsonField({ name, description, value, onChange }: JsonFieldProps) {
  const [text, setText] = useState(() => JSON.stringify(value ?? {}, null, 2));
  const [error, setError] = useState<string | null>(null);

  const handleBlur = useCallback(() => {
    try {
      const parsed = JSON.parse(text);
      setError(null);
      onChange(parsed);
    } catch {
      setError("Invalid JSON");
    }
  }, [text, onChange]);

  return (
    <div className="space-y-1">
      <Label htmlFor={name} className="text-sm">
        {name}
      </Label>
      <textarea
        id={name}
        className={cn(
          "w-full min-h-[80px] p-2 text-xs font-mono rounded border",
          "bg-background border-input focus:border-ring focus:outline-none resize-y",
          error && "border-destructive",
        )}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onBlur={handleBlur}
      />
      {error && <p className="text-xs text-destructive">{error}</p>}
      {!error && description && <p className="text-xs text-muted-foreground">{description}</p>}
    </div>
  );
}
