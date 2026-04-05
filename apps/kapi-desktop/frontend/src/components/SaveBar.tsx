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
    <div className="flex shrink-0 items-center justify-between border-t border-border bg-sidebar px-4 py-1.5">
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onUndo}
          disabled={!canUndo || saving}
          aria-label="Undo"
          title="Undo (⌘Z)"
          className="h-7 w-7"
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
          className="h-7 w-7"
        >
          <Redo2 size={14} />
        </Button>
      </div>
      {isDirty && (
        <Button size="sm" onClick={handleSave} disabled={saving} className="h-7 text-xs">
          {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
          Save Changes
        </Button>
      )}
    </div>
  );
}
