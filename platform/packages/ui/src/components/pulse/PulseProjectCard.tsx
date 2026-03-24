import { CompletionRing } from "./CompletionRing";

interface PulseProjectCardProps {
  name: string;
  sourceLanguage: string;
  targetLanguages: string[];
  totalWords: number;
  translatedWords: number;
  percentage: number;
  onClick?: () => void;
}

export function PulseProjectCard({
  name,
  sourceLanguage,
  targetLanguages,
  totalWords,
  translatedWords,
  percentage,
  onClick,
}: PulseProjectCardProps) {
  return (
    <div
      className="cursor-pointer rounded-lg border bg-card p-4 transition-colors hover:bg-muted/50"
      onClick={onClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === "Enter" && onClick?.()}
    >
      <div className="flex items-start justify-between">
        <div className="min-w-0 flex-1">
          <h3 className="font-semibold truncate">{name}</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            {sourceLanguage} → {targetLanguages.join(", ")}
          </p>
        </div>
        <CompletionRing percentage={percentage} size={48} strokeWidth={4} />
      </div>
      <div className="mt-3 text-xs text-muted-foreground">
        {translatedWords.toLocaleString()} / {totalWords.toLocaleString()} words translated
      </div>
    </div>
  );
}
