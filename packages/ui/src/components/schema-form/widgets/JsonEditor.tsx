import { useState, useCallback } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { cn } from "../../../lib/utils";
import { CodeInput } from "../../ui/code-input";

export function JsonEditor({
  label,
  description,
  value,
  onChange,
}: {
  label: string;
  description?: string;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const [collapsed, setCollapsed] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [text, setText] = useState(() => JSON.stringify(value ?? {}, null, 2));

  const handleChange = useCallback(
    (raw: string) => {
      setText(raw);
      try {
        const parsed = JSON.parse(raw);
        onChange(parsed);
        setError(null);
      } catch (e) {
        setError((e as Error).message);
      }
    },
    [onChange],
  );

  return (
    <div className="space-y-1">
      <button
        type="button"
        className="flex items-center gap-1 text-xs font-medium"
        onClick={() => setCollapsed(!collapsed)}
      >
        {collapsed ? (
          <ChevronRight className="size-3 text-muted-foreground" />
        ) : (
          <ChevronDown className="size-3 text-muted-foreground" />
        )}
        {label}
      </button>
      {!collapsed && (
        <>
          {description && <p className="text-xs text-muted-foreground">{description}</p>}
          <CodeInput
            value={text}
            onChange={handleChange}
            language="json"
            minHeight={80}
            className={cn(error && "border-destructive")}
          />
          {error && <p className="text-xs text-destructive">{error}</p>}
        </>
      )}
    </div>
  );
}

export function JsonInlineEditor({
  value,
  onChange,
}: {
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const [text, setText] = useState(() =>
    typeof value === "string" ? value : JSON.stringify(value ?? "", null, 2),
  );

  const handleChange = useCallback(
    (raw: string) => {
      setText(raw);
      try {
        onChange(JSON.parse(raw));
      } catch {
        onChange(raw);
      }
    },
    [onChange],
  );

  return <CodeInput value={text} onChange={handleChange} language="json" minHeight={40} />;
}
