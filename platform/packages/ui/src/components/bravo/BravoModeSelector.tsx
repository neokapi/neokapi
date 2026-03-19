import { cn } from "../../lib/utils";

export type BravoMode = "ask" | "coworker" | "bravo";

export interface BravoModeSelectorProps {
  mode: BravoMode;
  onChange: (mode: BravoMode) => void;
}

const modes: { value: BravoMode; label: string; description: string }[] = [
  { value: "ask", label: "Ask", description: "Expert Q&A" },
  { value: "coworker", label: "Co-worker", description: "Full assistant" },
  { value: "bravo", label: "Voice", description: "Brand review" },
];

export function BravoModeSelector({ mode, onChange }: BravoModeSelectorProps) {
  return (
    <div
      className="flex rounded-lg bg-muted p-0.5 gap-0.5"
      role="radiogroup"
      aria-label="Interaction mode"
    >
      {modes.map((m) => (
        <button
          key={m.value}
          role="radio"
          aria-checked={mode === m.value}
          onClick={() => onChange(m.value)}
          className={cn(
            "flex-1 rounded-md px-2.5 py-1 text-xs font-medium transition-all duration-200",
            mode === m.value
              ? "bg-background text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground",
          )}
          title={m.description}
        >
          {m.label}
        </button>
      ))}
    </div>
  );
}
