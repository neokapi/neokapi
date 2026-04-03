import { localeDisplayName } from "../LanguageLabel";
import { CompletionRing } from "./CompletionRing";

interface PulseProjectCardProps {
  name: string;
  sourceLanguage: string;
  targetLanguages: string[];
  totalWords: number;
  translatedWords: number;
  percentage: number;
  onClick?: () => void;
  /** Optional display-name overrides (e.g. from workspace settings). */
  languageNames?: Record<string, string>;
}

export function PulseProjectCard({
  name,
  sourceLanguage,
  targetLanguages,
  totalWords,
  translatedWords,
  percentage,
  onClick,
  languageNames,
}: PulseProjectCardProps) {
  const resolveName = (code: string) => languageNames?.[code] ?? localeDisplayName(code, "short");

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
          <h3 className="truncate font-semibold">{name}</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            {resolveName(sourceLanguage)} → {targetLanguages.map(resolveName).join(", ")}
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
