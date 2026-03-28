import { useCallback } from "react";
import type { SpectrumOption } from "./data/tone-spectrums";

interface ToneSpectrumSelectorProps<T extends string> {
  options: SpectrumOption<T>[];
  value: T;
  onChange: (value: T) => void;
  label?: string;
}

export function ToneSpectrumSelector<T extends string>({
  options,
  value,
  onChange,
  label,
}: ToneSpectrumSelectorProps<T>) {
  const selectedOption = options.find((o) => o.value === value);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent, optionValue: T) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        onChange(optionValue);
      }
    },
    [onChange],
  );

  return (
    <div className="space-y-3">
      {label && <div className="text-sm font-medium">{label}</div>}
      <div role="radiogroup" aria-label={label} className="flex gap-1 rounded-lg bg-muted/50 p-1">
        {options.map((option) => {
          const isSelected = option.value === value;
          return (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={isSelected}
              onClick={() => onChange(option.value)}
              onKeyDown={(e) => handleKeyDown(e, option.value)}
              className={`flex-1 rounded-md px-3 py-2 text-sm font-medium transition-all cursor-pointer border-none ${
                isSelected
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {option.label}
            </button>
          );
        })}
      </div>
      {selectedOption && (
        <div className="space-y-1.5 rounded-md border border-border/50 bg-muted/30 px-3 py-2.5">
          <p className="text-xs text-muted-foreground">{selectedOption.description}</p>
          <p className="text-xs italic text-foreground/70">
            &ldquo;{selectedOption.exampleText}&rdquo;
          </p>
        </div>
      )}
    </div>
  );
}
