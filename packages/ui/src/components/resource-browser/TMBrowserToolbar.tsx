import { LocalePill } from "./LocalePill";
import { Button } from "../ui/button";
import { Checkbox } from "../ui/checkbox";
import { List, Languages } from "lucide-react";
import { cn } from "../../lib/utils";
import { triStateChecked, type ViewMode } from "./tm-browser-helpers";

interface TMBrowserToolbarProps {
  isEmpty: boolean;
  allVisibleSelected: boolean;
  someVisibleSelected: boolean;
  onToggleSelectAll: () => void;
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  allLocales: string[];
  bilingualSrc: string | null;
  bilingualTgt: string | null;
  onBilingualSrcChange: (locale: string | null) => void;
  onBilingualTgtChange: (locale: string | null) => void;
  /** undefined = show all; array = show only those. */
  displayLocales: string[] | undefined;
  onToggleDisplayLocale: (locale: string) => void;
  onDisplayLocalesChange: (locales: string[] | undefined) => void;
}

/** Selection checkbox + locale controls + view toggle row of the TM browser. */
export function TMBrowserToolbar({
  isEmpty,
  allVisibleSelected,
  someVisibleSelected,
  onToggleSelectAll,
  viewMode,
  onViewModeChange,
  allLocales,
  bilingualSrc,
  bilingualTgt,
  onBilingualSrcChange,
  onBilingualTgtChange,
  displayLocales,
  onToggleDisplayLocale,
  onDisplayLocalesChange,
}: TMBrowserToolbarProps) {
  return (
    <div className="flex items-center gap-2 mb-2 pl-3 min-h-7">
      {!isEmpty && (
        <Checkbox
          checked={triStateChecked(allVisibleSelected, someVisibleSelected)}
          onCheckedChange={onToggleSelectAll}
          aria-label="Select all visible entries"
          title={allVisibleSelected ? "Deselect all" : "Select all on this page"}
        />
      )}

      {viewMode === "bilingual" && allLocales.length > 1 && (
        <>
          <span className="inline-flex shrink-0 items-center px-1.5 py-px text-[10px] font-medium text-muted-foreground ml-3">
            Pair:
          </span>
          <select
            value={bilingualSrc ?? ""}
            onChange={(e) => onBilingualSrcChange(e.target.value || null)}
            className="text-[11px] rounded border border-input bg-background px-1.5 py-0.5"
            aria-label="Bilingual source locale"
          >
            <option value="">— src —</option>
            {allLocales.map((l) => (
              <option key={l} value={l}>
                {l}
              </option>
            ))}
          </select>
          <span className="text-muted-foreground text-[11px]">→</span>
          <select
            value={bilingualTgt ?? ""}
            onChange={(e) => onBilingualTgtChange(e.target.value || null)}
            className="text-[11px] rounded border border-input bg-background px-1.5 py-0.5"
            aria-label="Bilingual target locale"
          >
            <option value="">— tgt —</option>
            {allLocales.map((l) => (
              <option key={l} value={l}>
                {l}
              </option>
            ))}
          </select>
        </>
      )}

      {viewMode === "multilang" && allLocales.length > 1 && (
        <>
          <span className="inline-flex shrink-0 items-center px-1.5 py-px text-[10px] font-medium text-muted-foreground ml-3">
            Show:
          </span>
          {allLocales.map((locale) => {
            const active = displayLocales === undefined || displayLocales.includes(locale);
            return (
              <button
                key={locale}
                onClick={() => onToggleDisplayLocale(locale)}
                className={cn(
                  "inline-flex items-center",
                  "transition-opacity",
                  !active && "opacity-30",
                )}
              >
                <LocalePill locale={locale} />
              </button>
            );
          })}
          <button
            onClick={() => onDisplayLocalesChange(undefined)}
            className="inline-flex shrink-0 items-center px-1.5 py-px rounded font-mono text-[10px] font-medium bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground transition-colors"
            title="Show all languages"
          >
            All
          </button>
          <button
            onClick={() => onDisplayLocalesChange([])}
            className="inline-flex shrink-0 items-center px-1.5 py-px rounded font-mono text-[10px] font-medium bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground transition-colors"
            title="Hide all languages"
          >
            None
          </button>
        </>
      )}

      {/* View toggle */}
      <div className="flex rounded-md border border-input ml-auto">
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={() => onViewModeChange("bilingual")}
          className={cn("rounded-r-none", viewMode === "bilingual" && "bg-accent text-foreground")}
          title="Bilingual view"
        >
          <List className="size-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={() => onViewModeChange("multilang")}
          className={cn("rounded-l-none", viewMode === "multilang" && "bg-accent text-foreground")}
          title="Multi-language view"
        >
          <Languages className="size-4" />
        </Button>
      </div>
    </div>
  );
}
