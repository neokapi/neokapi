import { LanguageLabel } from "../LanguageLabel";
import { CompletionRing } from "./CompletionRing";

interface LocaleCardProps {
  locale: string;
  displayName?: string;
  translatedWords: number;
  totalWords: number;
  percentage: number;
}

function LocaleCard({
  locale,
  displayName,
  translatedWords,
  totalWords,
  percentage,
}: LocaleCardProps) {
  return (
    <div className="flex items-center gap-4 rounded-lg border bg-card p-4">
      <CompletionRing percentage={percentage} size={56} strokeWidth={5} />
      <div className="min-w-0 flex-1">
        <LanguageLabel code={locale} displayName={displayName} className="font-medium" />
        <div className="text-sm text-muted-foreground">
          {translatedWords.toLocaleString()} / {totalWords.toLocaleString()} words
        </div>
      </div>
    </div>
  );
}

interface LanguageProgressGridProps {
  languages: {
    locale: string;
    display_name?: string;
    translated_words: number;
    total_words: number;
    percentage: number;
  }[];
}

export function LanguageProgressGrid({ languages }: LanguageProgressGridProps) {
  if (languages.length === 0) {
    return (
      <div className="rounded-lg border bg-card p-8 text-center text-muted-foreground">
        No languages configured yet.
      </div>
    );
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {languages.map((lang) => (
        <LocaleCard
          key={lang.locale}
          locale={lang.locale}
          displayName={lang.display_name}
          translatedWords={lang.translated_words}
          totalWords={lang.total_words}
          percentage={lang.percentage}
        />
      ))}
    </div>
  );
}
