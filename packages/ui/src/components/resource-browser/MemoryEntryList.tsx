import { t } from "@neokapi/i18n-react/runtime";
import type { Run } from "@neokapi/kapi-format";
import type { MemoryEntryDTO, MemoryPointDTO } from "./types";
import { Pagination } from "./Pagination";
import { MemoryGroupedEntry } from "./MemoryGroupedEntry";
import { ItemCard } from "../ui/item-card";
import { withHint, PAGE_SIZE } from "./memory-browser-helpers";

interface MemoryEntryListProps {
  entries: MemoryEntryDTO[];
  loading: boolean;
  initialLoadDone: boolean;
  /** Submitted search text — drives the empty-state copy and clear affordance. */
  searchQuery: string;
  onClearSearch: () => void;
  /** Source-locale override for display (bilingual view), or null. */
  hintLocale: string | null;
  /** Locales to show per entry; undefined = show all. */
  visibleLocales: string[] | undefined;
  selected: Set<string>;
  onToggleSelect: (id: string) => void;
  onEditVariant: (entry: MemoryEntryDTO, locale: string, runs: Run[]) => void;
  onDelete: (id: string) => void;
  page: number;
  totalCount: number;
  onPageChange: (page: number) => void;
  /** Resolve where an uncontested entry's unit sits. Absent = no coordinate. */
  resolvePoint?: (entry: MemoryEntryDTO) => Promise<MemoryPointDTO | null>;
  /** Open the context surface at an entry's unit. */
  onOpenUnit?: (entry: MemoryEntryDTO) => void;
}

/** Loading skeleton, empty state, entry cards, and pagination of the content memory browser. */
export function MemoryEntryList({
  entries,
  loading,
  initialLoadDone,
  searchQuery,
  onClearSearch,
  hintLocale,
  visibleLocales,
  selected,
  onToggleSelect,
  onEditVariant,
  onDelete,
  page,
  totalCount,
  onPageChange,
  resolvePoint,
  onOpenUnit,
}: MemoryEntryListProps) {
  const isEmpty = entries.length === 0;

  return (
    <>
      {/* Loading skeleton */}
      {loading && !initialLoadDone && (
        <div className="flex flex-col gap-2">
          {[0, 1, 2].map((i) => (
            <ItemCard key={i} className="animate-pulse p-3">
              <div className="mb-2 h-3 w-3/4 rounded bg-muted" />
              <div className="h-3 w-2/3 rounded bg-muted" />
            </ItemCard>
          ))}
        </div>
      )}

      {/* Empty state */}
      {initialLoadDone && !loading && isEmpty && (
        <div className="py-12 text-center text-muted-foreground">
          <p className="text-sm mb-1">
            {searchQuery ? t("No entries match your search.") : t("No entries yet.")}
          </p>
          {searchQuery && (
            <button onClick={onClearSearch} className="text-xs text-primary hover:text-primary/80">
              Clear search
            </button>
          )}
        </div>
      )}

      {/* Entries */}
      {!isEmpty && (
        <div className="flex flex-col gap-1.5">
          {entries.map((entry) => (
            <MemoryGroupedEntry
              key={entry.id}
              entry={withHint(entry, hintLocale)}
              resolvePoint={resolvePoint}
              onOpenUnit={onOpenUnit}
              selected={selected.has(entry.id)}
              onToggleSelect={() => onToggleSelect(entry.id)}
              onEditVariant={(locale, runs) => onEditVariant(entry, locale, runs)}
              onDelete={() => onDelete(entry.id)}
              visibleLocales={visibleLocales}
            />
          ))}
        </div>
      )}

      <Pagination
        page={page}
        pageSize={PAGE_SIZE}
        totalCount={totalCount}
        onPageChange={onPageChange}
      />
    </>
  );
}
