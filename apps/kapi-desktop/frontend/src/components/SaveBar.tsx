import { useState } from "react";
import { Save, Loader2 } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";

interface SaveBarProps {
  isDirty: boolean;
  onSave: () => Promise<void>;
}

export function SaveBar({ isDirty, onSave }: SaveBarProps) {
  const [saving, setSaving] = useState(false);

  if (!isDirty) return null;

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave();
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex shrink-0 items-center justify-end border-t border-border bg-sidebar px-4 py-1.5">
      <Button size="sm" onClick={handleSave} disabled={saving} className="h-7 text-xs">
        {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
        Save Changes
      </Button>
    </div>
  );
}
