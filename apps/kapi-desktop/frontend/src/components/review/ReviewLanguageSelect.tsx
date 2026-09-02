import { useMemo, useState } from "react";
import { ChevronsUpDown } from "lucide-react";
import {
  Button,
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  LocaleLabel,
  Popover,
  PopoverContent,
  PopoverTrigger,
  formatLocale,
  uiLocaleTag,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewLanguage } from "../../types/api";

/**
 * The Review page's one selector: which language the reviewer is working in.
 *
 * One queue holds every language a project has work in, the source language
 * among them, so one control picks a language rather than a control picking a
 * lane and a second picking a language inside it. Choosing the source language
 * puts the reviewer in front of the author's own wording; choosing a target
 * puts them in front of translations of it; "All languages" lists everything
 * with each row saying which language it belongs to.
 *
 * The source language is marked the way it is marked everywhere else, by
 * LocaleLabel's own `source` marker, and every entry carries the number of
 * units waiting behind it.
 */
export interface ReviewLanguageSelectProps {
  /** The chosen language tag, or "" for every language. */
  value: string;
  onChange: (language: string) => void;
  /** Each language in the queue with its pending count. */
  languages: ReviewLanguage[];
  /** Total pending across every language, shown against "All languages". */
  total: number;
  className?: string;
  "data-slot"?: string;
}

export function ReviewLanguageSelect({
  value,
  onChange,
  languages,
  total,
  className,
  "data-slot": dataSlot,
}: ReviewLanguageSelectProps) {
  const [open, setOpen] = useState(false);

  // A language the queue no longer has rows for is still the selection until the
  // reviewer changes it, so it keeps an entry rather than reading as "All".
  const entries = useMemo(() => {
    if (!value || languages.some((l) => l.language === value)) return languages;
    return [...languages, { language: value, pending: 0 }];
  }, [languages, value]);

  const selected = entries.find((l) => l.language === value);
  const allLabel = t("All languages");

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="xs"
          role="combobox"
          aria-expanded={open}
          aria-label={t("Language under review")}
          data-slot={dataSlot}
          className={className}
        >
          {selected ? (
            <LocaleLabel
              locale={selected.language}
              variant="short"
              hideCode
              source={selected.source}
            />
          ) : (
            <span>{allLabel}</span>
          )}
          <span className="tabular-nums text-muted-foreground">
            {selected ? selected.pending : total}
          </span>
          <ChevronsUpDown className="size-3 shrink-0 opacity-50" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="min-w-[260px] p-0" align="end">
        <Command>
          <CommandInput placeholder={t("Search languages")} />
          <CommandList>
            <CommandEmpty>{t("No language matches.")}</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value={allLabel}
                onSelect={() => {
                  onChange("");
                  setOpen(false);
                }}
                data-checked={value === ""}
                data-slot="review-language-all"
              >
                <span className="flex w-full items-center gap-2">
                  <span className="flex-1 whitespace-nowrap">{allLabel}</span>
                  <span className="tabular-nums text-muted-foreground">{total}</span>
                </span>
              </CommandItem>
              {entries.map((l) => (
                <CommandItem
                  key={l.language}
                  // Typed text matches the name as well as the tag, so a reviewer
                  // who reads "French" can search for it.
                  value={`${formatLocale(l.language, { uiLocale: uiLocaleTag() }).name} ${l.language}`}
                  onSelect={() => {
                    onChange(l.language);
                    setOpen(false);
                  }}
                  data-checked={l.language === value}
                  data-slot="review-language-option"
                  data-language={l.language}
                >
                  <span className="flex w-full items-center gap-2">
                    <LocaleLabel locale={l.language} source={l.source} className="flex-1" />
                    <span className="tabular-nums text-muted-foreground">{l.pending}</span>
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
