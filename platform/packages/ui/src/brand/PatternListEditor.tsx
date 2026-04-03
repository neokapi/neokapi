import { useCallback } from "react";
import type { Pattern } from "./types";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import { Label } from "@neokapi/ui-primitives/components/ui/label";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives/components/ui/select";
import { Plus, Trash2 } from "../components/icons";

interface PatternListEditorProps {
  label: string;
  patterns: Pattern[];
  onChange: (patterns: Pattern[]) => void;
}

export function PatternListEditor({ label, patterns, onChange }: PatternListEditorProps) {
  const addPattern = useCallback(() => {
    onChange([...patterns, { regex: "", description: "", severity: "minor" }]);
  }, [patterns, onChange]);

  const updatePattern = useCallback(
    (index: number, field: keyof Pattern, value: string) => {
      const updated = [...patterns];
      updated[index] = { ...updated[index], [field]: value };
      onChange(updated);
    },
    [patterns, onChange],
  );

  const removePattern = useCallback(
    (index: number) => {
      onChange(patterns.filter((_, i) => i !== index));
    },
    [patterns, onChange],
  );

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label>{label}</Label>
        <Button variant="outline" size="sm" onClick={addPattern}>
          <Plus className="w-3.5 h-3.5 mr-1" /> Add
        </Button>
      </div>
      {patterns.length === 0 && (
        <p className="text-xs text-muted-foreground">No patterns defined.</p>
      )}
      {patterns.map((pat, i) => (
        <div key={i} className="flex gap-2 items-start">
          <Input
            placeholder="Regex"
            value={pat.regex}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => updatePattern(i, "regex", e.target.value)}
            className="font-mono text-xs"
          />
          <Input
            placeholder="Description"
            value={pat.description}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => updatePattern(i, "description", e.target.value)}
          />
          <Select
            value={pat.severity}
            onValueChange={(v: string) => updatePattern(i, "severity", v)}
          >
            <SelectTrigger className="w-28">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="minor">Minor</SelectItem>
              <SelectItem value="major">Major</SelectItem>
              <SelectItem value="critical">Critical</SelectItem>
            </SelectContent>
          </Select>
          <Button variant="ghost" size="icon" onClick={() => removePattern(i)}>
            <Trash2 className="w-3.5 h-3.5" />
          </Button>
        </div>
      ))}
    </div>
  );
}
