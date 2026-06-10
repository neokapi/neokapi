/**
 * SelectableList — a reusable table with checkbox selection and bulk operations.
 *
 * Built on shadcn Table primitives. Supports select-all, filter, and
 * renders action buttons when items are selected.
 */

import { useState, useMemo, useCallback, type ReactNode } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from "./table";
import { Input } from "./input";
import { cn } from "../../lib/utils";

export interface SelectableListColumn<T> {
  /** Column header label. */
  header: string;
  /** Render cell content for a row. */
  cell: (item: T) => ReactNode;
  /** Optional className for the header and cells. */
  className?: string;
}

export interface SelectableListAction<T> {
  /** Button label. */
  label: ReactNode;
  /** Called with currently selected items. */
  onAction: (selected: T[]) => void;
  /** Show this action only when the predicate returns true for at least one selected item. */
  when?: (item: T) => boolean;
}

export interface SelectableListProps<T> {
  /** All items to display. */
  items: T[];
  /** Unique key for each item. */
  getKey: (item: T) => string;
  /** Column definitions. */
  columns: SelectableListColumn<T>[];
  /** Bulk actions shown when items are selected. */
  actions?: SelectableListAction<T>[];
  /** Filter predicate — receives the item and the search query. */
  filterFn?: (item: T, query: string) => boolean;
  /** Placeholder for the filter input. */
  filterPlaceholder?: string;
  /** Optional row className based on item state. */
  rowClassName?: (item: T) => string;
  /** Maximum height of the scrollable area. */
  maxHeight?: number;
  /** Content shown when no items match. */
  emptyMessage?: string;
  /** Class for the outer wrapper. */
  className?: string;
}

export function SelectableList<T>({
  items,
  getKey,
  columns,
  actions,
  filterFn,
  filterPlaceholder = t("Filter..."),
  rowClassName,
  maxHeight = 400,
  emptyMessage = t("No items match the filter."),
  className,
}: SelectableListProps<T>) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState("");

  const filtered = useMemo(() => {
    if (!filter || !filterFn) return items;
    return items.filter((item) => filterFn(item, filter));
  }, [items, filter, filterFn]);

  const allSelected = filtered.length > 0 && filtered.every((item) => selected.has(getKey(item)));

  const toggleSelect = useCallback((key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const toggleAll = useCallback(() => {
    if (allSelected) {
      setSelected(new Set());
    } else {
      setSelected(new Set(filtered.map(getKey)));
    }
  }, [allSelected, filtered, getKey]);

  const selectedItems = useMemo(
    () => items.filter((item) => selected.has(getKey(item))),
    [items, selected, getKey],
  );

  const visibleActions = useMemo(
    () => (actions ?? []).filter((a) => !a.when || selectedItems.some(a.when)),
    [actions, selectedItems],
  );

  const handleAction = useCallback(
    (action: SelectableListAction<T>) => {
      action.onAction(selectedItems);
      setSelected(new Set());
    },
    [selectedItems],
  );

  return (
    <div className={cn("space-y-3", className)}>
      {/* Toolbar */}
      <div className="flex items-center gap-2">
        {filterFn && (
          <Input
            type="text"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={filterPlaceholder}
            className="max-w-xs"
          />
        )}
        <div className="flex-1" />
        {selected.size > 0 && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span>{selected.size} selected</span>
            {visibleActions.map((action, i) => (
              <button
                key={i}
                onClick={() => handleAction(action)}
                className="inline-flex items-center gap-1 rounded-md border border-input bg-background px-2.5 py-1 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
              >
                {action.label}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Table */}
      <div className="overflow-auto rounded-lg border border-border" style={{ maxHeight }}>
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-8">
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={toggleAll}
                  className="rounded"
                  aria-label="Select all"
                />
              </TableHead>
              {columns.map((col, i) => (
                <TableHead key={i} className={col.className}>
                  {col.header}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((item) => {
              const key = getKey(item);
              const isSelected = selected.has(key);
              return (
                <TableRow
                  key={key}
                  data-state={isSelected ? "selected" : undefined}
                  className={cn("cursor-pointer", rowClassName?.(item))}
                  onClick={() => toggleSelect(key)}
                >
                  <TableCell className="w-8">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggleSelect(key)}
                      onClick={(e) => e.stopPropagation()}
                      className="rounded"
                      aria-label={`Select ${key}`}
                    />
                  </TableCell>
                  {columns.map((col, i) => (
                    <TableCell key={i} className={col.className}>
                      {col.cell(item)}
                    </TableCell>
                  ))}
                </TableRow>
              );
            })}
            {filtered.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={columns.length + 1}
                  className="py-6 text-center text-muted-foreground"
                >
                  {emptyMessage}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
