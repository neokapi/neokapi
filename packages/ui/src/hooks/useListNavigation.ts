import * as React from "react";

/**
 * useListNavigation: the keyboard model of a list whose selection follows
 * focus, the macOS list convention.
 *
 * ArrowDown and `j` select the next item, ArrowUp and `k` the previous one,
 * Home and End the first and the last. Enter and Space select the current
 * item, or the first when nothing is selected. The ends do not wrap. With
 * nothing selected, the next item is the first and the previous is the last.
 * No other key is handled, a letter included, so a page's own shortcuts keep
 * working while the list has focus; a key held with Cmd, Ctrl or Alt is left
 * alone for the same reason.
 *
 * `ListDetailList` drives it from its own keydown. A page that wants the same
 * movement from elsewhere (a shortcut while the detail pane has focus, a
 * toolbar's next and previous buttons) calls the hook with the ids it renders
 * and calls `next()` and the rest itself.
 */

/** A move through the list. `current` selects the selected item again. */
export type ListMove = "next" | "previous" | "first" | "last" | "current";

export interface UseListNavigationOptions {
  /**
   * The selectable ids in the order they are shown, or a function that reads
   * them when a key arrives (a list reads its own rendered rows).
   */
  ids: readonly string[] | (() => readonly string[]);
  /** The selected id, or nothing. */
  selectedId: string | null | undefined;
  /** Called with the id to select. */
  onSelect: (id: string) => void;
}

/** The parts of a keyboard event the hook reads. */
export type ListNavigationKey = Pick<
  KeyboardEvent,
  "key" | "metaKey" | "ctrlKey" | "altKey" | "isComposing"
>;

export interface ListNavigation {
  /** Select the item after the selected one. */
  next: () => void;
  /** Select the item before the selected one. */
  previous: () => void;
  /** Select the first item. */
  first: () => void;
  /** Select the last item. */
  last: () => void;
  /** Select the selected item again, or the first when nothing is selected. */
  selectCurrent: () => void;
  /**
   * Act on one key press. Returns true when the key moved or selected, so the
   * caller can prevent its default (Space would otherwise scroll the pane).
   */
  handleKey: (event: ListNavigationKey) => boolean;
}

/** Where a move lands, given the ids in order and the selected one. */
export function navigationTarget(
  ids: readonly string[],
  selectedId: string | null | undefined,
  move: ListMove,
): string | undefined {
  if (ids.length === 0) return undefined;
  const at = selectedId == null ? -1 : ids.indexOf(selectedId);
  switch (move) {
    case "first":
      return ids[0];
    case "last":
      return ids[ids.length - 1];
    case "current":
      return at === -1 ? ids[0] : ids[at];
    case "next":
      return at === -1 ? ids[0] : ids[Math.min(at + 1, ids.length - 1)];
    case "previous":
      return at === -1 ? ids[ids.length - 1] : ids[Math.max(at - 1, 0)];
  }
}

/** The keys the list answers, and the move each makes. */
export const LIST_NAVIGATION_KEYS: ReadonlyMap<string, ListMove> = new Map<string, ListMove>([
  ["ArrowDown", "next"],
  ["j", "next"],
  ["ArrowUp", "previous"],
  ["k", "previous"],
  ["Home", "first"],
  ["End", "last"],
  ["Enter", "current"],
  [" ", "current"],
]);

/** The keyboard model of a list whose selection follows focus. */
export function useListNavigation(options: UseListNavigationOptions): ListNavigation {
  // The latest options, read when a key arrives, so the returned functions
  // keep one identity across renders and a caller can bind them once.
  const latest = React.useRef(options);
  latest.current = options;

  return React.useMemo(() => {
    const go = (move: ListMove): boolean => {
      const { ids, selectedId, onSelect } = latest.current;
      const target = navigationTarget(typeof ids === "function" ? ids() : ids, selectedId, move);
      if (target === undefined) return false;
      if (target !== selectedId || move === "current") onSelect(target);
      return true;
    };
    return {
      next: () => void go("next"),
      previous: () => void go("previous"),
      first: () => void go("first"),
      last: () => void go("last"),
      selectCurrent: () => void go("current"),
      handleKey: (event) => {
        if (event.metaKey || event.ctrlKey || event.altKey || event.isComposing) return false;
        const move = LIST_NAVIGATION_KEYS.get(event.key);
        return move ? go(move) : false;
      },
    };
  }, []);
}
