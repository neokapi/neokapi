import * as React from "react";

import { cn } from "../../lib/utils";
import { useListNavigation } from "../../hooks/useListNavigation";
import { EmptyState } from "../EmptyState";
import { ScrollArea } from "./scroll-area";

/**
 * ListDetail: the list-and-detail page shape. A narrow list of one-line rows
 * with a selection, and a detail pane for the selected item, which holds the
 * item's actions in its toolbar.
 *
 * The split is fixed: the list takes `listWidth` and the detail the rest, each
 * scrolling on its own. Where the container is narrower than 760px the list
 * stacks above the detail and scrolls within a shorter pane.
 *
 * The list is a listbox whose selection follows focus. Arrow keys, `j` and `k`,
 * Home and End move the selection while the list has focus, and the list names
 * the selected row through `aria-activedescendant`, so focus stays on the list
 * and the rows are not tab stops. `useListNavigation` is the same model for a
 * page that moves the selection from elsewhere.
 *
 *     <ListDetail
 *       list={
 *         <ListDetailList selectedId={id} onSelect={setId} aria-label="Suggestions">
 *           <ListDetailGroup label="Needs you" count={1}>
 *             <ListDetailItem id="c1" leading={<KindIcon kind="conflict" />}>…</ListDetailItem>
 *           </ListDetailGroup>
 *         </ListDetailList>
 *       }
 *       detail={id ? <ListDetailPane toolbar={…}>…</ListDetailPane> : <ListDetailEmpty title="…" />}
 *     />
 */

// ── The split ────────────────────────────────────────────────────────────────

export interface ListDetailProps extends Omit<React.ComponentProps<"div">, "children"> {
  /** The list pane, usually a `ListDetailList`. */
  list: React.ReactNode;
  /** The detail pane, usually a `ListDetailPane` or a `ListDetailEmpty`. */
  detail: React.ReactNode;
  /** The list pane's width in pixels. Defaults to 300. */
  listWidth?: number;
}

// Radix wraps a scroll area's content in a `display: table` box, which grows
// to fit its widest line and so defeats a row's ellipsis. A flex column keeps
// the content to the pane's width, and its full height lets an empty state
// sit in the middle of the pane.
const PANE_CLASS =
  "min-h-0 [&>[data-slot=scroll-area-viewport]>div]:flex! [&>[data-slot=scroll-area-viewport]>div]:min-h-full [&>[data-slot=scroll-area-viewport]>div]:flex-col";

/** A fixed two-column split: the list on the left, the detail on the right. */
export function ListDetail({
  list,
  detail,
  listWidth = 300,
  className,
  style,
  ...props
}: ListDetailProps) {
  return (
    <div
      data-slot="list-detail"
      className={cn("@container/list-detail h-full min-h-0", className)}
      style={style}
      {...props}
    >
      <div
        className="grid h-full min-h-0 grid-cols-[var(--list-detail-width)_minmax(0,1fr)] @max-[760px]/list-detail:grid-cols-1 @max-[760px]/list-detail:grid-rows-[auto_minmax(0,1fr)]"
        style={{ "--list-detail-width": `${listWidth}px` } as React.CSSProperties}
      >
        <ScrollArea
          data-pane="list"
          className={cn(
            PANE_CLASS,
            "border-r border-border @max-[760px]/list-detail:border-r-0 @max-[760px]/list-detail:border-b @max-[760px]/list-detail:[&>[data-slot=scroll-area-viewport]]:max-h-64",
          )}
        >
          {list}
        </ScrollArea>
        <ScrollArea data-pane="detail" className={PANE_CLASS}>
          {detail}
        </ScrollArea>
      </div>
    </div>
  );
}

// ── The list ─────────────────────────────────────────────────────────────────

interface ListDetailContextValue {
  selectedId: string | null | undefined;
  select: (id: string) => void;
  optionId: (id: string) => string;
}

const ListDetailContext = React.createContext<ListDetailContextValue | null>(null);

function useListDetailContext(component: string): ListDetailContextValue {
  const ctx = React.useContext(ListDetailContext);
  if (!ctx) throw new Error(`${component} must be rendered inside a ListDetailList`);
  return ctx;
}

/** The attribute a row carries its id in, which the list reads its order from. */
const ITEM_ID_ATTR = "data-list-detail-id";

/** The ids of the rows a list renders, in the order it shows them. */
function renderedIds(list: HTMLElement | null): string[] {
  if (!list) return [];
  return Array.from(list.querySelectorAll<HTMLElement>(`[role="option"][${ITEM_ID_ATTR}]`)).map(
    (el) => el.getAttribute(ITEM_ID_ATTR) ?? "",
  );
}

export interface ListDetailListProps extends Omit<
  React.ComponentProps<"div">,
  "onSelect" | "role" | "tabIndex"
> {
  /** The selected row's id, or nothing. */
  selectedId: string | null | undefined;
  /** Called with a row's id when the person selects it. */
  onSelect: (id: string) => void;
  /** Names the list for assistive technology, e.g. "Suggestions". */
  "aria-label"?: string;
}

/** A listbox of one-line rows, keyboard-driven, with selection that follows focus. */
export function ListDetailList({
  selectedId,
  onSelect,
  className,
  children,
  onKeyDown,
  ...props
}: ListDetailListProps) {
  const ref = React.useRef<HTMLDivElement>(null);
  const baseId = React.useId();
  const optionId = React.useCallback(
    // An id may hold any character; encoding it leaves no whitespace, which
    // is all a DOM id must not contain.
    (id: string) => `${baseId}-option-${encodeURIComponent(id)}`,
    [baseId],
  );
  const nav = useListNavigation({
    ids: () => renderedIds(ref.current),
    selectedId,
    onSelect,
  });

  // Keep the selected row in view when the selection moves, by key or from
  // the page. The first render is left alone, so a list that opens on a
  // selection does not scroll the window to it.
  const mounted = React.useRef(false);
  React.useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      return;
    }
    if (selectedId == null) return;
    document.getElementById(optionId(selectedId))?.scrollIntoView?.({ block: "nearest" });
  }, [selectedId, optionId]);

  const ctx = React.useMemo<ListDetailContextValue>(
    () => ({ selectedId, select: onSelect, optionId }),
    [selectedId, onSelect, optionId],
  );

  return (
    <ListDetailContext.Provider value={ctx}>
      <div
        ref={ref}
        role="listbox"
        tabIndex={0}
        aria-activedescendant={selectedId == null ? undefined : optionId(selectedId)}
        data-slot="list-detail-list"
        className={cn(
          "group/listbox flex flex-col gap-px rounded-md p-2 outline-none [&:focus-visible:not(:has([aria-selected=true]))]:ring-2 [&:focus-visible:not(:has([aria-selected=true]))]:ring-ring/50 [&:focus-visible:not(:has([aria-selected=true]))]:ring-inset",
          className,
        )}
        onKeyDown={(event) => {
          onKeyDown?.(event);
          // A key pressed on a control inside a row belongs to that control.
          if (event.defaultPrevented || event.target !== event.currentTarget) return;
          if (nav.handleKey(event.nativeEvent)) event.preventDefault();
        }}
        {...props}
      >
        {children}
      </div>
    </ListDetailContext.Provider>
  );
}

// ── Groups and rows ──────────────────────────────────────────────────────────

export interface ListDetailGroupProps extends Omit<React.ComponentProps<"div">, "children"> {
  /** The group's name, e.g. "Needs you". */
  label: React.ReactNode;
  /** How many rows the group holds, drawn at the right. */
  count?: React.ReactNode;
  /**
   * The group's rows. Given, the group wraps them, and its header is pushed
   * off by the next group's as the list scrolls. Omitted, the header is drawn
   * on its own and the rows follow it as siblings.
   */
  children?: React.ReactNode;
}

/** A group header, label left and count right, that stays at the top while its rows scroll. */
export function ListDetailGroup({
  label,
  count,
  children,
  className,
  ...props
}: ListDetailGroupProps) {
  // The group is named by its label alone; the count is drawn, not read.
  const labelId = React.useId();
  const header = (
    <div
      data-slot="list-detail-group"
      className={cn(
        "sticky top-0 z-10 -mx-2 flex items-baseline justify-between gap-2 bg-background px-4 pt-2.5 pb-1 text-[11px] font-semibold text-muted-foreground",
        children === undefined && className,
      )}
      {...(children === undefined ? props : {})}
    >
      <span id={labelId} className="min-w-0 truncate">
        {label}
      </span>
      {count !== undefined && <span className="shrink-0 tabular-nums">{count}</span>}
    </div>
  );
  if (children === undefined) return header;
  return (
    <div
      role="group"
      aria-labelledby={labelId}
      className={cn("flex flex-col gap-px", className)}
      {...props}
    >
      {header}
      {children}
    </div>
  );
}

export interface ListDetailItemProps extends Omit<
  React.ComponentProps<"div">,
  "id" | "role" | "onSelect"
> {
  /** The row's id, which `onSelect` is called with. Unique within the list. */
  id: string;
  /** A kind icon, in a 20px column. */
  leading?: React.ReactNode;
  /** An actor mark or a count, at the right. */
  trailing?: React.ReactNode;
  /** The row's one line. It is truncated with an ellipsis where it runs long. */
  children: React.ReactNode;
}

/** One row of the list: a selectable option on one line. */
export function ListDetailItem({
  id,
  leading,
  trailing,
  children,
  className,
  onClick,
  ...props
}: ListDetailItemProps) {
  const { selectedId, select, optionId } = useListDetailContext("ListDetailItem");
  const selected = selectedId === id;
  return (
    <div
      role="option"
      id={optionId(id)}
      data-list-detail-id={id}
      aria-selected={selected}
      data-slot="list-detail-item"
      data-selected={selected || undefined}
      className={cn(
        // scroll-mt clears the sticky group header when a row scrolls into view.
        "grid min-h-9 cursor-default scroll-mt-9 grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-2 rounded-md px-2 py-1.5 text-sm",
        selected
          ? "bg-primary/10 ring-1 ring-primary group-focus-visible/listbox:ring-2"
          : "hover:bg-muted",
        className,
      )}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented) select(id);
      }}
      {...props}
    >
      <span className="flex items-center justify-center text-muted-foreground">{leading}</span>
      <span className="min-w-0 truncate">{children}</span>
      <span className="flex shrink-0 items-center gap-1 empty:hidden">{trailing}</span>
    </div>
  );
}

// ── The detail ───────────────────────────────────────────────────────────────

export interface ListDetailPaneProps extends React.ComponentProps<"div"> {
  /** A `ListDetailToolbar` naming the item and holding its actions. */
  toolbar?: React.ReactNode;
}

/** The detail column: an optional toolbar and the selected item's content. */
export function ListDetailPane({ toolbar, children, className, ...props }: ListDetailPaneProps) {
  return (
    <div
      data-slot="list-detail-pane"
      className={cn("flex min-w-0 flex-col gap-3.5 px-5 py-4", className)}
      {...props}
    >
      {toolbar}
      {children}
    </div>
  );
}

export interface ListDetailToolbarProps extends Omit<React.ComponentProps<"div">, "title"> {
  /** The selected item's name. */
  title: React.ReactNode;
  /** The item's actions, at the right. */
  actions?: React.ReactNode;
}

/** The detail pane's header: the item's name at the left, its actions at the right. */
export function ListDetailToolbar({ title, actions, className, ...props }: ListDetailToolbarProps) {
  return (
    <div
      data-slot="list-detail-toolbar"
      className={cn(
        "flex flex-wrap items-center justify-between gap-2 border-b border-border pb-2.5",
        className,
      )}
      {...props}
    >
      <h2 className="min-w-0 text-sm font-semibold text-foreground">{title}</h2>
      {actions && <div className="flex flex-wrap items-center gap-1.5">{actions}</div>}
    </div>
  );
}

export interface ListDetailEmptyProps {
  /** What the pane shows once a row is selected, e.g. "Choose a suggestion to see it here". */
  title: string;
  /** One more sentence, when the title is not enough. */
  description?: string;
  icon?: React.ReactNode;
  /** The one action that fills the list, when it is empty. */
  action?: React.ReactNode;
  className?: string;
}

/** What the detail pane shows when nothing is selected. */
export function ListDetailEmpty({ className, ...props }: ListDetailEmptyProps) {
  return (
    <div
      data-slot="list-detail-empty"
      className={cn("flex min-h-48 flex-1 items-center justify-center p-6", className)}
    >
      <EmptyState className="w-full max-w-sm" {...props} />
    </div>
  );
}
