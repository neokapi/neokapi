import { useState } from "react";
import { Save, Undo2, Redo2, Loader2 } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";

interface SaveBarProps {
  isDirty: boolean;
  canUndo: boolean;
  canRedo: boolean;
  onSave: () => Promise<void>;
  onUndo: () => void;
  onRedo: () => void;
}

export function SaveBar({ isDirty, canUndo, canRedo, onSave, onUndo, onRedo }: SaveBarProps) {
  const [saving, setSaving] = useState(false);

  if (!isDirty && !canUndo && !canRedo) return null;

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave();
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed bottom-4 right-4 z-40 flex items-center gap-1.5 rounded-lg border border-border bg-background/95 px-3 py-2 shadow-lg backdrop-blur">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onUndo}
        disabled={!canUndo || saving}
        aria-label="Undo"
        title="Undo (⌘Z)"
      >
        <Undo2 size={14} />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onRedo}
        disabled={!canRedo || saving}
        aria-label="Redo"
        title="Redo (⌘⇧Z)"
      >
        <Redo2 size={14} />
      </Button>
      {isDirty && (
        <Button size="sm" onClick={handleSave} disabled={saving} className="ml-1">
          {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
          Save
        </Button>
      )}
    </div>
  );
}
