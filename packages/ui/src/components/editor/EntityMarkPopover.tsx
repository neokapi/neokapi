import { useState, useEffect, useRef } from "react";
import { entityLabel } from "./HighlightedSource";

const entityTypes = [
  "entity:person",
  "entity:organization",
  "entity:location",
  "entity:date",
  "entity:product",
  "entity:other",
];

interface EntityMarkPopoverProps {
  /** Selected text to mark as entity. */
  text: string;
  /** Character offset of selection start in the source string. */
  start: number;
  /** Character offset of selection end in the source string. */
  end: number;
  /** Position to render the popover at (from selection rect). */
  position: { x: number; y: number };
  onConfirm: (type: string, dnt: boolean) => void;
  onCancel: () => void;
}

/**
 * Popover shown when the user selects text and presses Cmd+E to mark an entity.
 */
export function EntityMarkPopover({ text, position, onConfirm, onCancel }: EntityMarkPopoverProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [selectedType, setSelectedType] = useState("entity:other");
  const [dnt, setDnt] = useState(false);

  // Close on Escape.
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onCancel();
      if (e.key === "Enter") onConfirm(selectedType, dnt);
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [onCancel, onConfirm, selectedType, dnt]);

  // Close on outside click.
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onCancel();
      }
    }
    // Delay to avoid capturing the triggering click.
    const timer = setTimeout(() => document.addEventListener("mousedown", handleClick), 50);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("mousedown", handleClick);
    };
  }, [onCancel]);

  return (
    <div
      ref={ref}
      className="fixed z-50 w-52 rounded-md border border-border bg-popover text-popover-foreground shadow-lg p-3 text-xs"
      style={{ left: position.x, top: position.y + 4 }}
    >
      <div className="font-medium text-sm mb-2 truncate" title={text}>
        Mark: &ldquo;{text}&rdquo;
      </div>

      <label className="block mb-1 text-muted-foreground">Entity Type</label>
      <select
        className="w-full mb-2 rounded border border-input bg-background px-2 py-1 text-xs"
        value={selectedType}
        onChange={(e) => setSelectedType(e.target.value)}
        autoFocus
      >
        {entityTypes.map((t) => (
          <option key={t} value={t}>
            {entityLabel(t)}
          </option>
        ))}
      </select>

      <label className="flex items-center gap-2 mb-3 cursor-pointer">
        <input
          type="checkbox"
          className="rounded"
          checked={dnt}
          onChange={(e) => setDnt(e.target.checked)}
        />
        <span>Do Not Translate</span>
      </label>

      <div className="flex gap-1">
        <button
          className="flex-1 rounded bg-primary px-2 py-1 text-primary-foreground text-xs font-medium hover:opacity-90"
          onClick={() => onConfirm(selectedType, dnt)}
        >
          Mark Entity
        </button>
        <button
          className="rounded border border-border px-2 py-1 text-xs hover:bg-accent"
          onClick={onCancel}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
