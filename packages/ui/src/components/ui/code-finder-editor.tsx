import { useState, useCallback, useMemo } from "react";
import { X, Plus, ChevronDown, AlertCircle } from "lucide-react";
import { cn } from "../../lib/utils";
import { Button } from "./button";
import { Input } from "./input";
import { Label } from "./label";
import { CodeInput } from "./code-input";

export interface CodeFinderRulesValue {
  rules: Array<{ pattern: string }>;
  sample?: string;
}

export interface CodeFinderEditorProps {
  value?: CodeFinderRulesValue;
  onChange: (value: CodeFinderRulesValue) => void;
  presets?: Record<string, unknown>;
  label?: string;
  description?: string;
  className?: string;
  disabled?: boolean;
}

// OKLCH colors for up to 8 rules — distinct hues, readable on both light/dark
const RULE_COLORS = [
  "oklch(0.75 0.15 145)",  // green
  "oklch(0.75 0.15 250)",  // blue
  "oklch(0.75 0.15 320)",  // pink
  "oklch(0.78 0.14 75)",   // amber
  "oklch(0.75 0.15 200)",  // cyan
  "oklch(0.75 0.12 30)",   // red-orange
  "oklch(0.78 0.12 180)",  // teal
  "oklch(0.75 0.15 280)",  // violet
];

function ruleColor(index: number): string {
  return RULE_COLORS[index % RULE_COLORS.length];
}

function validateRegex(pattern: string): string | null {
  if (!pattern) return null;
  try {
    new RegExp(pattern);
    return null;
  } catch (e) {
    return (e as Error).message.replace(/^Invalid regular expression: /, "");
  }
}

function highlightMatches(
  text: string,
  patterns: Array<{ pattern: string; index: number }>,
): React.ReactNode[] {
  if (!text || patterns.length === 0) return [text];

  // Build matches with rule index tracking
  const allMatches: Array<{ start: number; end: number; text: string; ruleIndex: number }> = [];

  for (const { pattern, index } of patterns) {
    if (!pattern || validateRegex(pattern)) continue;
    try {
      const regex = new RegExp(pattern, "g");
      let m: RegExpExecArray | null;
      while ((m = regex.exec(text)) !== null) {
        if (m[0].length === 0) { regex.lastIndex++; continue; }
        allMatches.push({ start: m.index, end: regex.lastIndex, text: m[0], ruleIndex: index });
      }
    } catch { /* skip invalid */ }
  }

  if (allMatches.length === 0) return [text];

  // Sort by position, longest match first for ties
  allMatches.sort((a, b) => a.start - b.start || b.end - a.end);

  // Build non-overlapping output
  const parts: React.ReactNode[] = [];
  let cursor = 0;

  for (const m of allMatches) {
    if (m.start < cursor) continue;
    if (m.start > cursor) {
      parts.push(text.slice(cursor, m.start));
    }
    parts.push(
      <mark
        key={`${m.start}-${m.ruleIndex}`}
        className="rounded-sm px-0.5"
        style={{
          backgroundColor: `color-mix(in oklch, ${ruleColor(m.ruleIndex)} 25%, transparent)`,
          color: ruleColor(m.ruleIndex),
        }}
        title={`Rule ${m.ruleIndex + 1}`}
      >
        {m.text}
      </mark>,
    );
    cursor = m.end;
  }

  if (cursor < text.length) {
    parts.push(text.slice(cursor));
  }

  return parts;
}

export function CodeFinderEditor({
  value,
  onChange,
  presets,
  label,
  description,
  className,
  disabled,
}: CodeFinderEditorProps) {
  const [showPresets, setShowPresets] = useState(false);
  const [errors, setErrors] = useState<Map<number, string>>(new Map());

  const rules = value?.rules ?? [];
  const sample = value?.sample ?? "";
  const update = useCallback(
    (patch: Partial<CodeFinderRulesValue>) => {
      onChange({ rules, sample, ...value, ...patch });
    },
    [value, rules, sample, onChange],
  );

  const handleAddRule = useCallback(() => {
    update({ rules: [...rules, { pattern: "" }] });
  }, [rules, update]);

  const handleRemoveRule = useCallback(
    (index: number) => {
      const next = [...rules];
      next.splice(index, 1);
      setErrors((prev) => {
        const updated = new Map(prev);
        updated.delete(index);
        return updated;
      });
      update({ rules: next });
    },
    [rules, update],
  );

  const handleRuleChange = useCallback(
    (index: number, pattern: string) => {
      const next = [...rules];
      next[index] = { pattern };
      const err = validateRegex(pattern);
      setErrors((prev) => {
        const updated = new Map(prev);
        if (err) updated.set(index, err);
        else updated.delete(index);
        return updated;
      });
      update({ rules: next });
    },
    [rules, update],
  );

  const handleApplyPreset = useCallback(
    (presetName: string) => {
      const preset = presets?.[presetName] as CodeFinderRulesValue | undefined;
      if (preset) {
        onChange(preset);
        setErrors(new Map());
      }
      setShowPresets(false);
    },
    [presets, onChange],
  );

  const highlightedSample = useMemo(() => {
    const indexed = rules
      .map((r, i) => ({ pattern: r.pattern, index: i }))
      .filter((r) => r.pattern);
    return highlightMatches(sample, indexed);
  }, [rules, sample]);

  const hasPresets = presets && Object.keys(presets).length > 0;

  return (
    <div data-slot="code-finder-editor" className={cn("space-y-3", disabled && "opacity-50 pointer-events-none", className)}>
      {/* Header: label + presets */}
      <div className="flex items-center justify-between gap-2">
        <div>
          {label && <Label className="text-sm font-medium">{label}</Label>}
          {description && (
            <p className="text-xs text-muted-foreground mt-0.5">
              {description}
            </p>
          )}
        </div>
        {hasPresets && (
          <div className="relative">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled}
              onClick={() => setShowPresets(!showPresets)}
              className="gap-1"
            >
              Presets
              <ChevronDown className="size-3" />
            </Button>
            {showPresets && (
              <div className="absolute right-0 mt-1 min-w-[140px] bg-popover border border-border rounded-md shadow-lg z-10 py-1">
                {Object.keys(presets!).map((name) => (
                  <button
                    key={name}
                    type="button"
                    className="block w-full px-3 py-1.5 text-left text-sm hover:bg-accent transition-colors"
                    onClick={() => handleApplyPreset(name)}
                  >
                    {name}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Rules list */}
      <div className="space-y-1.5">
        {rules.map((rule, index) => (
          <div key={index}>
            <div className="flex items-center gap-1.5">
              <div
                className={cn(
                  "flex items-center flex-1 rounded-md border border-input overflow-hidden",
                  "focus-within:border-ring focus-within:ring-ring/50 focus-within:ring-[3px]",
                  errors.has(index) && "border-destructive focus-within:ring-destructive/30",
                )}
              >
                <span
                  className="shrink-0 w-1.5 self-stretch"
                  style={{ backgroundColor: ruleColor(index) }}
                />
                <CodeInput
                  value={rule.pattern}
                  onChange={(v) => handleRuleChange(index, v)}
                  language="regex"
                  placeholder="Regex pattern"
                  disabled={disabled}
                  singleLine
                  className="flex-1 text-xs border-0 ring-0 focus-within:ring-0 focus-within:border-0 rounded-none"
                />
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={disabled}
                className="size-7 p-0 shrink-0 text-muted-foreground hover:text-destructive"
                onClick={() => handleRemoveRule(index)}
              >
                <X className="size-3.5" />
              </Button>
            </div>
            {errors.has(index) && (
              <div className="flex items-center gap-1 ml-5 mt-1 text-xs text-destructive">
                <AlertCircle className="size-3 shrink-0" />
                <span className="truncate">{errors.get(index)}</span>
              </div>
            )}
          </div>
        ))}

        <div className="ml-5">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            className="h-7 text-xs gap-1"
            onClick={handleAddRule}
          >
            <Plus className="size-3" />
            Add rule
          </Button>
        </div>
      </div>

      {/* Test area */}
      <div className="rounded-lg border border-input bg-card p-3 space-y-2">
        <Label className="text-xs font-medium">
          Test Sample
        </Label>

        <Input
          value={sample}
          placeholder="Enter sample text to test patterns against..."
          disabled={disabled}
          className="text-sm"
          onChange={(e) => update({ sample: e.target.value })}
        />

        {/* Match preview */}
        {sample && rules.some((r) => r.pattern) && (
          <div className="rounded-md bg-muted/40 px-3 py-2 text-sm font-mono break-all">
            {highlightedSample}
          </div>
        )}
      </div>
    </div>
  );
}
