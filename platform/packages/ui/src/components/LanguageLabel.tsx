import { cn } from "../lib/utils";

interface LanguageLabelProps {
  code: string;
  displayName?: string;
  className?: string;
  /** Hide the code badge (show display name only). */
  hideCode?: boolean;
}

export function LanguageLabel({ code, displayName, className, hideCode }: LanguageLabelProps) {
  if (!displayName || displayName === code) {
    return <span className={cn("font-mono text-xs", className)}>{code}</span>;
  }

  return (
    <span className={cn("inline-flex items-center gap-1.5", className)}>
      <span>{displayName}</span>
      {!hideCode && (
        <span className="rounded bg-muted px-1 py-0.5 font-mono text-[10px] leading-none text-muted-foreground">
          {code}
        </span>
      )}
    </span>
  );
}
