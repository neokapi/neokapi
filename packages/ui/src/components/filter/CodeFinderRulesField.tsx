import { useState, useCallback } from "react";

// UI components from the ui directory
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";

// ─── Code Finder Rules Field ────────────────────────────────

interface CodeFinderRulesFieldProps {
  name: string;
  description?: string;
  value: Record<string, unknown> | undefined;
  presets?: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
}

export function CodeFinderRulesField({
  name,
  description,
  value,
  presets,
  onChange,
}: CodeFinderRulesFieldProps) {
  const [showPresets, setShowPresets] = useState(false);

  const rules = (value?.rules as Array<{ pattern: string }>) ?? [];
  const sample = (value?.sample as string) ?? "";

  const handleAddRule = useCallback(() => {
    onChange({
      ...value,
      rules: [...rules, { pattern: "" }],
    });
  }, [value, rules, onChange]);

  const handleRemoveRule = useCallback(
    (index: number) => {
      const newRules = [...rules];
      newRules.splice(index, 1);
      onChange({ ...value, rules: newRules });
    },
    [value, rules, onChange],
  );

  const handleRuleChange = useCallback(
    (index: number, pattern: string) => {
      const newRules = [...rules];
      newRules[index] = { pattern };
      onChange({ ...value, rules: newRules });
    },
    [value, rules, onChange],
  );

  const handleSampleChange = useCallback(
    (newSample: string) => {
      onChange({ ...value, sample: newSample });
    },
    [value, onChange],
  );

  const handleApplyPreset = useCallback(
    (presetName: string) => {
      const preset = presets?.[presetName] as Record<string, unknown>;
      if (preset) {
        onChange(preset);
      }
      setShowPresets(false);
    },
    [presets, onChange],
  );

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label className="text-sm">{name}</Label>
        {presets && Object.keys(presets).length > 0 && (
          <div className="relative">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setShowPresets(!showPresets)}
            >
              Presets
            </Button>
            {showPresets && (
              <div className="absolute right-0 mt-1 bg-popover border border-border rounded shadow-lg z-10">
                {Object.keys(presets).map((presetName) => (
                  <button
                    key={presetName}
                    type="button"
                    className="block w-full px-3 py-1.5 text-left text-sm hover:bg-accent"
                    onClick={() => handleApplyPreset(presetName)}
                  >
                    {presetName}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      {description && <p className="text-xs text-muted-foreground">{description}</p>}

      <div className="space-y-2 ml-2">
        {rules.map((rule, index) => (
          <div key={index} className="flex items-center gap-2">
            <Input
              value={rule.pattern}
              placeholder="Regex pattern"
              className="flex-1 font-mono text-xs"
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                handleRuleChange(index, e.target.value)
              }
            />
            <Button type="button" variant="ghost" size="sm" onClick={() => handleRemoveRule(index)}>
              ✕
            </Button>
          </div>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={handleAddRule}>
          + Add Rule
        </Button>
      </div>

      <div className="mt-2">
        <Label className="text-xs text-muted-foreground">Sample Text</Label>
        <Input
          value={sample}
          placeholder="Sample text to test patterns"
          className="mt-1"
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => handleSampleChange(e.target.value)}
        />
      </div>
    </div>
  );
}
