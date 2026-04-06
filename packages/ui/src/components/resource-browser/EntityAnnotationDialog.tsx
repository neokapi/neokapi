import { useState, useCallback, useEffect } from "react";
import { ENTITY_TYPES, type EntityPatternRequest, type AnnotateResult } from "./types";

interface EntityAnnotationDialogProps {
  open: boolean;
  onClose: () => void;
  selectedCount: number;
  /** Initial pattern text (pre-filled from search query). */
  initialPattern?: string;
  /** Called to apply the annotation. Returns the result. */
  onApply: (patterns: EntityPatternRequest[]) => Promise<AnnotateResult>;
}

/**
 * Dialog for batch entity annotation on selected TM entries.
 * Lets the user define text→entity mappings and apply them.
 */
export function EntityAnnotationDialog({
  open,
  onClose,
  selectedCount,
  initialPattern,
  onApply,
}: EntityAnnotationDialogProps) {
  const [patterns, setPatterns] = useState<EntityPatternRequest[]>([
    {
      text: initialPattern ?? "",
      entity_type: "entity:person",
      case_sensitive: true,
    },
  ]);
  const [applying, setApplying] = useState(false);
  const [result, setResult] = useState<AnnotateResult | null>(null);

  // Sync initialPattern when dialog opens with a new value.
  useEffect(() => {
    if (open && initialPattern !== undefined) {
      setPatterns([{ text: initialPattern, entity_type: "entity:person", case_sensitive: true }]);
      setResult(null);
    }
  }, [open, initialPattern]);

  const addPattern = useCallback(() => {
    setPatterns((prev: EntityPatternRequest[]) => [
      ...prev,
      { text: "", entity_type: "entity:person", case_sensitive: true },
    ]);
  }, []);

  const removePattern = useCallback((idx: number) => {
    setPatterns((prev: EntityPatternRequest[]) =>
      prev.filter((_: EntityPatternRequest, i: number) => i !== idx),
    );
  }, []);

  const updatePattern = useCallback(
    (idx: number, field: keyof EntityPatternRequest, value: string | boolean) => {
      setPatterns((prev: EntityPatternRequest[]) =>
        prev.map((p: EntityPatternRequest, i: number) =>
          i === idx ? { ...p, [field]: value } : p,
        ),
      );
    },
    [],
  );

  const [error, setError] = useState<string | null>(null);

  const handleApply = useCallback(async () => {
    const valid = patterns.filter((p: EntityPatternRequest) => p.text.trim() !== "");
    if (valid.length === 0) return;
    setApplying(true);
    setError(null);
    try {
      const res = await onApply(valid);
      setResult(res);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setApplying(false);
    }
  }, [patterns, onApply]);

  const handleClose = useCallback(() => {
    setResult(null);
    setApplying(false);
    onClose();
  }, [onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <div className="w-full max-w-lg rounded-xl border border-border bg-card p-6 shadow-lg">
        <h3 className="text-base font-semibold text-foreground mb-1">Annotate Entities</h3>
        <p className="text-[12px] text-muted-foreground mb-4">
          Convert text patterns to named entities across {selectedCount} selected{" "}
          {selectedCount === 1 ? "entry" : "entries"}.
        </p>

        {result ? (
          /* Result view */
          <div className="py-4">
            <div className="text-sm text-foreground mb-2">
              Updated {result.entries_updated} {result.entries_updated === 1 ? "entry" : "entries"},{" "}
              added {result.entities_added} {result.entities_added === 1 ? "entity" : "entities"}.
            </div>
            <button
              onClick={handleClose}
              className="rounded-md bg-primary px-4 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
            >
              Done
            </button>
          </div>
        ) : (
          <>
            {/* Pattern rows */}
            <div className="flex flex-col gap-2 mb-4">
              {patterns.map((p: EntityPatternRequest, idx: number) => (
                <div key={idx} className="flex items-center gap-2">
                  <input
                    type="text"
                    value={p.text}
                    onChange={(e) => updatePattern(idx, "text", e.target.value)}
                    placeholder="Text to match..."
                    className="flex-1 rounded-md border border-input bg-transparent px-2.5 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
                    autoFocus={idx === 0}
                  />
                  <select
                    value={p.entity_type}
                    onChange={(e) => updatePattern(idx, "entity_type", e.target.value)}
                    className="rounded-md border border-input bg-transparent px-2 py-1.5 text-xs outline-none"
                  >
                    {ENTITY_TYPES.map((et) => (
                      <option key={et.value} value={et.value}>
                        {et.label}
                      </option>
                    ))}
                  </select>
                  <label className="flex items-center gap-1 text-[11px] text-muted-foreground whitespace-nowrap">
                    <input
                      type="checkbox"
                      checked={p.case_sensitive}
                      onChange={(e) => updatePattern(idx, "case_sensitive", e.target.checked)}
                      className="rounded"
                    />
                    Case
                  </label>
                  {patterns.length > 1 && (
                    <button
                      onClick={() => removePattern(idx)}
                      className="text-xs text-muted-foreground hover:text-destructive transition-colors px-1"
                    >
                      x
                    </button>
                  )}
                </div>
              ))}
            </div>

            <button
              onClick={addPattern}
              className="text-xs text-primary hover:text-primary/80 transition-colors mb-4"
            >
              + Add pattern
            </button>

            {/* Error */}
            {error && <p className="text-xs text-destructive mb-2">{error}</p>}

            {/* Actions */}
            <div className="flex gap-2 pt-2 border-t border-border">
              <button
                onClick={handleApply}
                disabled={applying || patterns.every((p: EntityPatternRequest) => !p.text.trim())}
                className="rounded-md bg-primary px-4 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                {applying ? "Applying..." : `Apply to ${selectedCount} entries`}
              </button>
              <button
                onClick={handleClose}
                className="rounded-md border border-border px-4 py-1.5 text-xs hover:bg-accent transition-colors"
              >
                Cancel
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
