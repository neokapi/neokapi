import { useCallback } from "react";
import type { VoiceExample } from "./types";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import { Label } from "@neokapi/ui-primitives/components/ui/label";
import { Plus, Trash2 } from "../components/icons";

interface ExamplesEditorProps {
  examples: VoiceExample[];
  onChange: (examples: VoiceExample[]) => void;
}

export function ExamplesEditor({ examples, onChange }: ExamplesEditorProps) {
  const addExample = useCallback(() => {
    onChange([...examples, { before: "", after: "", explanation: "" }]);
  }, [examples, onChange]);

  const updateExample = useCallback(
    (index: number, field: keyof VoiceExample, value: string) => {
      const updated = [...examples];
      updated[index] = { ...updated[index], [field]: value };
      onChange(updated);
    },
    [examples, onChange],
  );

  const removeExample = useCallback(
    (index: number) => {
      onChange(examples.filter((_, i) => i !== index));
    },
    [examples, onChange],
  );

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <Label>Voice Examples</Label>
          <p className="text-xs text-muted-foreground mt-0.5">
            Before/after pairs that illustrate your brand voice
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={addExample}>
          <Plus className="w-3.5 h-3.5 mr-1" /> Add Example
        </Button>
      </div>
      {examples.length === 0 && (
        <p className="text-xs text-muted-foreground text-center py-4">
          No examples yet. Add before/after pairs to illustrate your brand voice.
        </p>
      )}
      {examples.map((ex, i) => (
        <div key={i} className="space-y-2 rounded-md border p-3">
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              onClick={() => removeExample(i)}
            >
              <Trash2 className="w-3 h-3" />
            </Button>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label className="text-xs text-destructive/80">Before</Label>
              <Input
                value={ex.before}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateExample(i, "before", e.target.value)}
                placeholder="Original text"
              />
            </div>
            <div className="space-y-1">
              <Label className="text-xs text-emerald-500">After</Label>
              <Input
                value={ex.after}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateExample(i, "after", e.target.value)}
                placeholder="Brand-compliant text"
              />
            </div>
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Explanation</Label>
            <Input
              value={ex.explanation ?? ""}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateExample(i, "explanation", e.target.value)}
              placeholder="Why this change?"
            />
          </div>
        </div>
      ))}
    </div>
  );
}
