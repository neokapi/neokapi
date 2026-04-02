interface BulkActionBarProps {
  selectedCount: number;
  onDelete: () => void;
  confirmDelete?: boolean;
  onAnnotateEntities?: () => void;
  onDeselectAll: () => void;
  className?: string;
}

/**
 * Floating bottom bar that appears when items are selected.
 * Shows selection count and bulk action buttons.
 * Delete requires a two-click confirmation to prevent accidents.
 */
export function BulkActionBar({
  selectedCount,
  onDelete,
  confirmDelete,
  onAnnotateEntities,
  onDeselectAll,
  className,
}: BulkActionBarProps) {
  if (selectedCount === 0) return null;

  return (
    <div
      className={`fixed bottom-4 left-1/2 -translate-x-1/2 z-40 flex items-center gap-3 rounded-lg border border-border bg-card px-4 py-2.5 shadow-lg animate-slide-up ${className ?? ""}`}
    >
      <span className="text-sm font-medium text-foreground">{selectedCount} selected</span>

      <div className="h-4 w-px bg-border" />

      {onAnnotateEntities && (
        <button
          onClick={onAnnotateEntities}
          className="text-xs font-medium text-primary hover:text-primary/80 transition-colors"
        >
          Annotate Entities
        </button>
      )}

      {confirmDelete ? (
        <span className="inline-flex items-center gap-1.5">
          <button
            onClick={onDelete}
            className="text-xs font-medium text-destructive-foreground bg-destructive px-2 py-0.5 rounded hover:bg-destructive/90 transition-colors"
          >
            Confirm delete {selectedCount}
          </button>
          <button
            onClick={onDeselectAll}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            Cancel
          </button>
        </span>
      ) : (
        <button
          onClick={onDelete}
          className="text-xs font-medium text-destructive hover:text-destructive/80 transition-colors"
        >
          Delete
        </button>
      )}

      <div className="h-4 w-px bg-border" />

      <button
        onClick={onDeselectAll}
        className="text-xs text-muted-foreground hover:text-foreground transition-colors"
      >
        Deselect all
      </button>
    </div>
  );
}
