import { cn } from "@neokapi/ui-primitives";

export type EditorSurface = "translate" | "review" | "pre-process";

export interface EditorSurfaceTabsProps {
  active: EditorSurface;
  onSelect: (surface: EditorSurface) => void;
  className?: string;
}

const TABS: { id: EditorSurface; label: string }[] = [
  { id: "pre-process", label: "Pre-process" },
  { id: "translate", label: "Translate" },
  { id: "review", label: "Review" },
];

/**
 * EditorSurfaceTabs is the shared switcher between the three per-file editor
 * surfaces — Pre-process, Translate, Review. It is presentational: the host
 * (web TanStack route, desktop state) supplies `onSelect` so the same strip
 * drives navigation in both apps without a fork.
 */
export function EditorSurfaceTabs({ active, onSelect, className }: EditorSurfaceTabsProps) {
  return (
    <div
      className={cn("flex items-center gap-0.5 rounded-md bg-muted p-0.5", className)}
      data-testid="editor-surface-tabs"
    >
      {TABS.map((tab) => (
        <button
          key={tab.id}
          type="button"
          onClick={() => onSelect(tab.id)}
          data-testid={`surface-tab-${tab.id}`}
          aria-current={active === tab.id ? "page" : undefined}
          className={cn(
            "rounded px-3 py-1 text-[11px] transition-colors",
            active === tab.id
              ? "bg-primary text-primary-foreground font-semibold"
              : "bg-transparent text-muted-foreground hover:text-foreground",
          )}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}
