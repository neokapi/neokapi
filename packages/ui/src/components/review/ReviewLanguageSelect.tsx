import { useMemo, useState } from "react";
import { ChevronsUpDown, Languages } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button } from "../ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "../ui/command";
import { LocaleLabel } from "../ui/locale-label";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { formatLocale, uiLocaleTag } from "../../lib/locale-name";
import { cn } from "../../lib/utils";

/**
 * The value standing for every language at once. It is not a well-formed BCP 47
 * tag, so no language collides with it.
 */
export const ALL_LANGUAGES = "*";

/**
 * One language lane a review surface can be read in, as
 * `convergence.SummarizeReviewLanguages` describes it: the tag, what is waiting
 * behind it, and whether it is the project's source language.
 */
export interface ReviewLanguageLane {
  /** BCP 47 tag. */
  language: string;
  /** Units awaiting a decision in this language, where the surface counts them. */
  pending?: number;
  /** The project's source language, marked as such wherever it is listed. */
  source?: boolean;
  /** Name this language as the workspace does, rather than as CLDR does. */
  displayName?: string;
}

export interface ReviewLanguageSelectProps {
  /** The chosen language tag, `ALL_LANGUAGES`, or "" for nothing chosen yet. */
  value: string;
  /** Every language the surface can be read in, the source language among them. */
  lanes: ReviewLanguageLane[];
  onChange: (language: string) => void;
  /** Offer the "All languages" entry at the top of the list. */
  allowAll?: boolean;
  /** Units awaiting a decision across every language, for the "All" entry. */
  allPending?: number;
  /** Accessible name for the trigger. */
  label?: string;
  /** Trigger height, matching the toolbar it sits in. */
  size?: "xs" | "sm" | "default";
  className?: string;
  "data-slot"?: string;
  /** Test hook; each entry derives `${testId}-${tag}` from it. */
  "data-testid"?: string;
}

/**
 * The one control a review surface offers for choosing what it reads.
 *
 * One list holds every language a project has review work in, the source
 * language among them and marked as the source, so one control picks a language
 * rather than a control picking a lane and a second picking a language inside
 * it. Choosing the source language puts the reviewer in front of the author's
 * own wording; choosing a target puts them in front of translations of it; "All
 * languages" reads everything at once.
 *
 * Every language is named through `LocaleLabel`, so the list reads "French
 * (France) fr-FR" in the reader's own UI language rather than a column of tags,
 * and each entry carries what is waiting behind it where the surface counts it.
 */
export function ReviewLanguageSelect({
  value,
  lanes,
  onChange,
  allowAll = false,
  allPending,
  label,
  size = "default",
  className,
  "data-slot": dataSlot,
  "data-testid": testId,
}: ReviewLanguageSelectProps) {
  const [open, setOpen] = useState(false);

  // A language the queue no longer has rows for is still the selection until the
  // reviewer changes it, so it keeps an entry rather than reading as unchosen.
  const entries = useMemo(() => {
    if (!value || value === ALL_LANGUAGES) return lanes;
    if (lanes.some((l) => l.language === value)) return lanes;
    return [...lanes, { language: value }];
  }, [lanes, value]);

  const selected = entries.find((l) => l.language === value);
  const allSelected = allowAll && value === ALL_LANGUAGES;
  const allLabel = t("All languages");
  const triggerCount = allSelected ? allPending : selected?.pending;

  const choose = (language: string) => {
    onChange(language);
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size={size}
          role="combobox"
          aria-expanded={open}
          aria-label={label ?? t("Language under review")}
          data-slot={dataSlot}
          data-testid={testId}
          className={cn("font-normal", className)}
        >
          <Languages className="shrink-0 text-muted-foreground" aria-hidden />
          {allSelected ? (
            <span className="min-w-0 flex-1 truncate text-left">{allLabel}</span>
          ) : selected ? (
            <LocaleLabel
              locale={selected.language}
              displayName={selected.displayName}
              source={selected.source}
              hideCode
              className="min-w-0 flex-1 truncate text-left"
            />
          ) : (
            <span className="min-w-0 flex-1 truncate text-left text-muted-foreground">
              {t("Choose a language")}
            </span>
          )}
          {triggerCount !== undefined && (
            <span className="tabular-nums text-muted-foreground">{triggerCount}</span>
          )}
          <ChevronsUpDown className="size-3 shrink-0 opacity-50" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="min-w-[260px] p-0" align="start">
        <Command>
          <CommandInput placeholder={t("Search languages")} />
          <CommandList>
            <CommandEmpty>No language matches.</CommandEmpty>
            <CommandGroup>
              {allowAll && (
                <CommandItem
                  value={allLabel}
                  onSelect={() => choose(ALL_LANGUAGES)}
                  data-checked={allSelected}
                  data-slot="review-language-all"
                  data-testid={testId ? `${testId}-all` : undefined}
                >
                  <span className="flex w-full items-center gap-2">
                    <span className="flex-1 whitespace-nowrap">{allLabel}</span>
                    <PendingCount count={allPending} />
                  </span>
                </CommandItem>
              )}
              {entries.map((lane) => (
                <CommandItem
                  key={lane.language}
                  // Typed text matches the name as well as the tag, so a reviewer
                  // who reads "French" can search for it.
                  value={`${lane.displayName ?? formatLocale(lane.language, { uiLocale: uiLocaleTag() }).name} ${lane.language}`}
                  onSelect={() => choose(lane.language)}
                  data-checked={lane.language === value}
                  data-slot="review-language-option"
                  data-language={lane.language}
                  data-testid={testId ? `${testId}-${lane.language}` : undefined}
                >
                  <span className="flex w-full items-center gap-2">
                    <LocaleLabel
                      locale={lane.language}
                      displayName={lane.displayName}
                      source={lane.source}
                      className="flex-1"
                    />
                    <PendingCount count={lane.pending} />
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

/** How many units this language is waiting on, where the surface counts them. */
function PendingCount({ count }: { count?: number }) {
  if (count === undefined) return null;
  return <span className="tabular-nums text-muted-foreground">{count}</span>;
}
