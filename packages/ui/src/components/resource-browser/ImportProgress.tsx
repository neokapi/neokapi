interface ImportProgressProps {
  active: boolean;
  fileName?: string;
  importedCount?: number;
  onClose?: () => void;
}

/**
 * Overlay with indeterminate progress bar during import operations.
 */
export function ImportProgress({ active, fileName, importedCount, onClose }: ImportProgressProps) {
  if (!active) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <div className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-lg">
        <div className="text-sm font-semibold text-foreground mb-1">Importing...</div>
        {fileName && (
          <div className="text-[12px] text-muted-foreground mb-3 truncate">{fileName}</div>
        )}

        {/* Indeterminate progress bar */}
        <div className="relative h-1.5 rounded-full bg-muted overflow-hidden mb-3">
          <div
            className="absolute inset-y-0 w-1/3 rounded-full bg-primary"
            style={{ animation: "indeterminate 1.5s ease-in-out infinite" }}
          />
        </div>

        {importedCount !== undefined && (
          <div className="text-[11px] text-muted-foreground">{importedCount} entries imported</div>
        )}

        {onClose && (
          <button
            onClick={onClose}
            className="mt-3 text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            Close
          </button>
        )}
      </div>
    </div>
  );
}
