import type { AppMode } from "../types/api";

interface ModeToggleProps {
  mode: AppMode;
  onChange: (mode: AppMode) => void;
}

export function ModeToggle({ mode, onChange }: ModeToggleProps) {
  return (
    <div className="flex rounded-lg bg-accent p-0.5 text-xs">
      <button
        onClick={() => onChange("adhoc")}
        className={`rounded-md px-3 py-1 transition-colors ${
          mode === "adhoc"
            ? "bg-background text-foreground font-medium shadow-sm"
            : "text-muted-foreground hover:text-foreground"
        }`}
      >
        Ad-Hoc
      </button>
      <button
        onClick={() => onChange("projects")}
        className={`rounded-md px-3 py-1 transition-colors ${
          mode === "projects"
            ? "bg-background text-foreground font-medium shadow-sm"
            : "text-muted-foreground hover:text-foreground"
        }`}
      >
        Projects
      </button>
    </div>
  );
}
