import { useEffect, useState } from "react";

/**
 * Debounce a fast-changing value: returns `value` only after it has held still
 * for `delayMs`. Keeps a list or query from re-running on every keystroke. The
 * pending timer is cleared on unmount, so a late value never lands after the
 * component is gone.
 */
export function useDebounced<T>(value: T, delayMs = 200): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(id);
  }, [value, delayMs]);
  return debounced;
}
