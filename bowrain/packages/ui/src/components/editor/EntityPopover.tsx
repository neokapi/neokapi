import { useEffect, useRef, useState } from "react";
import type { EntityInfo } from "../../types/api";
import { entityLabel } from "./HighlightedSource";

const entityTypes = [
  "entity:person",
  "entity:organization",
  "entity:location",
  "entity:date",
  "entity:product",
  "entity:other",
];

interface EntityPopoverProps {
  entity: EntityInfo;
  onClose: () => void;
  onUpdate?: (entity: EntityInfo) => void;
  onDelete?: (entityKey: string) => void;
  onPromote?: (entityKey: string) => void;
}

export function EntityPopover({
  entity,
  onClose,
  onUpdate,
  onDelete,
  onPromote,
}: EntityPopoverProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [selectedType, setSelectedType] = useState(entity.type);
  const [dnt, setDnt] = useState(entity.dnt);

  // Close on click outside.
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [onClose]);

  // Close on Escape.
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [onClose]);

  const hasChanges = selectedType !== entity.type || dnt !== entity.dnt;

  function handleSave() {
    if (onUpdate && hasChanges) {
      onUpdate({ ...entity, type: selectedType, dnt });
    }
    onClose();
  }

  return (
    <div
      ref={ref}
      className="absolute z-50 mt-1 left-0 top-full w-56 rounded-md border border-border bg-popover text-popover-foreground shadow-lg p-3 text-xs"
    >
      {/* Header */}
      <div className="font-medium text-sm mb-2 truncate" title={entity.text}>
        {entity.text}
      </div>

      {/* Source badge */}
      {entity.source && (
        <div className="mb-2 text-muted-foreground">
          Source: <span className="font-medium">{entity.source}</span>
        </div>
      )}

      {/* Entity type selector */}
      <label className="block mb-1 text-muted-foreground">Type</label>
      <select
        className="w-full mb-2 rounded border border-input bg-background px-2 py-1 text-xs"
        value={selectedType}
        onChange={(e) => setSelectedType(e.target.value)}
      >
        {entityTypes.map((t) => (
          <option key={t} value={t}>
            {entityLabel(t)}
          </option>
        ))}
      </select>

      {/* DNT toggle */}
      <label className="flex items-center gap-2 mb-3 cursor-pointer">
        <input
          type="checkbox"
          className="rounded"
          checked={dnt}
          onChange={(e) => setDnt(e.target.checked)}
        />
        <span>Do Not Translate</span>
      </label>

      {/* Actions */}
      <div className="flex items-center gap-1">
        {hasChanges && (
          <button
            className="flex-1 rounded bg-primary px-2 py-1 text-primary-foreground text-xs font-medium hover:opacity-90"
            onClick={handleSave}
          >
            Save
          </button>
        )}
        {onPromote && (
          <button
            className="flex-1 rounded border border-border px-2 py-1 text-xs hover:bg-accent"
            onClick={() => {
              onPromote(entity.key);
              onClose();
            }}
            title="Promote to terminology candidate"
          >
            Promote
          </button>
        )}
        {onDelete && (
          <button
            className="rounded border border-border px-2 py-1 text-xs text-destructive hover:bg-destructive/10"
            onClick={() => {
              onDelete(entity.key);
              onClose();
            }}
            title="Remove entity annotation"
          >
            Delete
          </button>
        )}
      </div>
    </div>
  );
}
