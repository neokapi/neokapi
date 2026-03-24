import { X } from "lucide-react";

export interface PulseFilter {
  key: string;
  value: string;
}

interface PulseFilterBarProps {
  filters: PulseFilter[];
  onRemove: (key: string) => void;
  onClear: () => void;
  presets?: { label: string; filters: PulseFilter[] }[];
  onPreset?: (filters: PulseFilter[]) => void;
}

export function PulseFilterBar({
  filters,
  onRemove,
  onClear,
  presets,
  onPreset,
}: PulseFilterBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {filters.map((f) => (
        <span
          key={f.key}
          className="inline-flex items-center gap-1 rounded-full bg-primary/10 px-3 py-1 text-sm"
        >
          <span className="text-muted-foreground">{f.key}:</span>
          <span className="font-medium">{f.value}</span>
          <button
            onClick={() => onRemove(f.key)}
            className="ml-1 rounded-full p-0.5 hover:bg-muted"
          >
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
      {filters.length > 0 && (
        <button onClick={onClear} className="text-xs text-muted-foreground hover:text-foreground">
          Clear all
        </button>
      )}
      {presets && presets.length > 0 && filters.length === 0 && (
        <div className="flex gap-2">
          {presets.map((p) => (
            <button
              key={p.label}
              onClick={() => onPreset?.(p.filters)}
              className="rounded-full border px-3 py-1 text-xs hover:bg-muted"
            >
              {p.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
