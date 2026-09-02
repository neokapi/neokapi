import {
  LocaleLabel,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
  formatLocale,
} from "@neokapi/ui-primitives";
import { Languages } from "../icons";

/**
 * The value that stands for every language at once. It is not a well-formed
 * BCP 47 tag, so no language can collide with it, and Radix needs a non-empty
 * string for an item.
 */
export const ALL_LANGUAGES = "*";

/** One language a review surface can be read in. */
export interface LanguageScopeOption {
  /** BCP 47 tag. */
  locale: string;
  /** Units awaiting a decision in this language, where the surface counts them. */
  pending?: number;
  /** The project's source language, marked as such in the list. */
  source?: boolean;
  /** The workspace's own name for the language, when it has one. */
  displayName?: string;
}

export interface LanguageScopeSelectProps {
  /** The selected language, or `ALL_LANGUAGES`. */
  value: string;
  /** Every language the surface can be read in, source language included. */
  options: LanguageScopeOption[];
  onChange: (value: string) => void;
  /** Offer the "All languages" entry at the top of the list. */
  allowAll?: boolean;
  /** Units awaiting a decision across every language, for the "All" entry. */
  allPending?: number;
  /** Accessible name for the trigger. */
  label?: string;
  /** Trigger height, matching the toolbar it sits in. */
  size?: "sm" | "default";
  className?: string;
  "data-testid"?: string;
}

/**
 * LanguageScopeSelect is the one control a review surface offers for choosing
 * what it reads. One list holds every language of the item or the workspace,
 * the source language among them and marked as the source, so choosing it opens
 * the source review the same way choosing French opens the French review.
 *
 * It replaces the pair a review toolbar used to carry, a Target/Source lane
 * toggle beside a target-language dropdown, where the lane and the language
 * were two halves of one question and the reader had to hold both.
 *
 * Every language is named through `LocaleLabel`, so a list reads "French fr-FR"
 * in the reader's own UI language rather than a column of tags.
 */
export function LanguageScopeSelect({
  value,
  options,
  onChange,
  allowAll = false,
  allPending,
  label,
  size = "default",
  className,
  "data-testid": testId = "language-scope",
}: LanguageScopeSelectProps) {
  const selected = options.find((o) => o.locale === value);
  const allSelected = allowAll && value === ALL_LANGUAGES;

  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        size={size}
        aria-label={label ?? "Language"}
        className={cn("w-[220px]", className)}
        data-testid={testId}
      >
        <Languages className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden />
        <SelectValue>
          {allSelected ? (
            <span>All languages</span>
          ) : selected ? (
            <LocaleLabel
              locale={selected.locale}
              displayName={selected.displayName}
              source={selected.source}
            />
          ) : (
            <span className="text-muted-foreground">Choose a language</span>
          )}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {allowAll && (
          <SelectItem value={ALL_LANGUAGES} data-testid={`${testId}-all`}>
            <span className="flex w-full items-center gap-2">
              <span>All languages</span>
              <PendingCount count={allPending} />
            </span>
          </SelectItem>
        )}
        {options.map((option) => (
          <SelectItem
            key={option.locale}
            value={option.locale}
            data-testid={`${testId}-${option.locale}`}
            // The tag is the identity a reader searches the list by, and the
            // name is what they read, so the accessible name carries both.
            textValue={formatLocale(option.locale).title}
          >
            <span className="flex w-full items-center gap-2">
              <LocaleLabel
                locale={option.locale}
                displayName={option.displayName}
                source={option.source}
              />
              <PendingCount count={option.pending} />
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** How many units this language is waiting on, when the surface counts them. */
function PendingCount({ count }: { count?: number }) {
  if (count === undefined) return null;
  return (
    <span className="ml-auto rounded-full bg-muted px-1.5 text-[11px] tabular-nums text-muted-foreground">
      {count}
    </span>
  );
}
