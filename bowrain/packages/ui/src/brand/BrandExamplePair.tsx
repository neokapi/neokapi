import type { VoiceExample } from "./types";
import { cn } from "@neokapi/ui-primitives";

interface BrandExamplePairProps {
  example: VoiceExample;
  className?: string;
}

export function BrandExamplePair({ example, className }: BrandExamplePairProps) {
  return (
    <div className={cn("grid grid-cols-2 gap-3 rounded-md border p-3 text-sm", className)}>
      <div className="space-y-1">
        <span className="text-[10px] font-medium uppercase tracking-wider text-destructive/80">
          Before
        </span>
        <p className="text-muted-foreground line-through decoration-red-400/50">{example.before}</p>
      </div>
      <div className="space-y-1">
        <span className="text-[10px] font-medium uppercase tracking-wider text-success/80">
          After
        </span>
        <p>{example.after}</p>
      </div>
      {example.explanation && (
        <p className="col-span-2 text-xs text-muted-foreground border-t pt-2">
          {example.explanation}
        </p>
      )}
    </div>
  );
}
