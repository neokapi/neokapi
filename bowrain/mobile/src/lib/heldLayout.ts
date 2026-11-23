// What a list screen has already shown stays where it is while the screen is
// on view: late data fills rows in place but never re-orders them or adds a
// card above them. The held layout is taken again when the screen comes back
// into view and after a pull, the moments a person expects a list to change.
import { useFocusEffect } from "expo-router";
import { useCallback, useState } from "react";

import { heldOrder } from "./heldOrder";

/**
 * Hold a list's order, and whether a section above it showed, from the
 * screen's first paint. `release` takes them again, as a pull does; the
 * screen coming into view does the same.
 */
export function useHeldLayout(ready: boolean, order: readonly string[], aboveShown: boolean) {
  const [held, setHeld] = useState<{ order: string[]; above: boolean } | null>(null);
  if (ready && held === null) {
    // Taken during the render that first shows the list, so the first paint
    // and every later one agree.
    setHeld({ order: [...order], above: aboveShown });
  }
  const release = useCallback(() => setHeld(null), []);
  useFocusEffect(release);
  return {
    order: heldOrder(held?.order ?? null, order),
    above: held === null ? aboveShown : held.above && aboveShown,
    release,
  };
}
