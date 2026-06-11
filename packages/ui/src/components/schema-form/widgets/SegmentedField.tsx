import { cn } from "../../../lib/utils";

/** Segmented button group for small enums (the `segmented` widget). */
export function SegmentedField({
  values,
  current,
  disabled,
  getLabel,
  onChange,
}: {
  values: unknown[];
  current: string;
  disabled?: boolean;
  getLabel: (val: unknown) => string;
  onChange: (value: unknown) => void;
}) {
  return (
    <div className="flex mb-1.5">
      {values.map((opt, i) => {
        const val = String(opt);
        const isActive = current === val;
        const isFirst = i === 0;
        const isLast = i === values.length - 1;
        return (
          <button
            key={val}
            type="button"
            disabled={disabled}
            onClick={() => onChange(val)}
            className={cn(
              "flex-1 py-1 text-xs font-semibold tracking-wide border transition-colors capitalize",
              isFirst && "rounded-l-md",
              isLast && "rounded-r-md",
              !isLast && "border-r-0",
              isActive
                ? "bg-primary text-primary-foreground"
                : "bg-transparent text-muted-foreground hover:bg-accent/50",
              disabled && "opacity-50 cursor-not-allowed",
            )}
          >
            {getLabel(val)}
          </button>
        );
      })}
    </div>
  );
}
