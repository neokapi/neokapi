import { useMemo } from "react";
import { cn } from "@neokapi/ui-primitives";

type LanguageLabelVariant = "full" | "short";

interface LanguageLabelProps {
  /** BCP-47 locale code (e.g. "fr-FR"). */
  code: string;
  /** Override the auto-resolved display name (e.g. workspace-specific name). */
  displayName?: string;
  className?: string;
  /** Hide the code badge (show display name only). */
  hideCode?: boolean;
  /**
   * Display variant:
   * - `"full"` (default): "French (France)" for fr-FR, "Portuguese (Brazil)" for pt-BR
   * - `"short"`: "French" for fr-FR, "Portuguese" for pt-BR (language only, no region)
   */
  variant?: LanguageLabelVariant;
}

const displayNamesCache = new Map<string, Intl.DisplayNames>();

function getDisplayNames(variant: LanguageLabelVariant): Intl.DisplayNames | undefined {
  const key = variant;
  if (displayNamesCache.has(key)) return displayNamesCache.get(key);
  try {
    const dn = new Intl.DisplayNames(["en"], { type: "language" });
    displayNamesCache.set(key, dn);
    return dn;
  } catch {
    return undefined;
  }
}

function resolveDisplayName(code: string, variant: LanguageLabelVariant): string | undefined {
  const dn = getDisplayNames(variant);
  if (!dn) return undefined;
  try {
    if (variant === "short") {
      // Extract the language subtag to get just "French" instead of "French (France)"
      const lang = code.split("-")[0];
      return dn.of(lang) ?? undefined;
    }
    return dn.of(code) ?? undefined;
  } catch {
    return undefined;
  }
}

/**
 * Resolve a BCP-47 locale code to its English display name using the browser's
 * Intl.DisplayNames API. Returns the code itself if resolution fails.
 *
 * @param code  BCP-47 locale code (e.g. "fr-FR")
 * @param variant "full" → "French (France)", "short" → "French"
 */
export function localeDisplayName(code: string, variant: LanguageLabelVariant = "full"): string {
  return resolveDisplayName(code, variant) ?? code;
}

export function LanguageLabel({
  code,
  displayName,
  className,
  hideCode,
  variant = "full",
}: LanguageLabelProps) {
  const resolved = useMemo(
    () => displayName ?? resolveDisplayName(code, variant),
    [code, displayName, variant],
  );

  if (!resolved || resolved === code) {
    return <span className={cn("font-mono text-xs", className)}>{code}</span>;
  }

  return (
    <span className={cn("inline-flex items-center gap-1.5", className)}>
      <span>{resolved}</span>
      {!hideCode && (
        <span className="rounded bg-muted px-1 py-0.5 font-mono text-[10px] leading-none text-muted-foreground">
          {code}
        </span>
      )}
    </span>
  );
}
