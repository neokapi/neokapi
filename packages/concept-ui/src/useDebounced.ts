// Debounce a fast-changing value (Apache-2.0) — used to keep the concept list
// from re-querying a remote source on every keystroke.
import { useEffect, useState } from "react";

export function useDebounced<T>(value: T, delayMs = 200): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(id);
  }, [value, delayMs]);
  return debounced;
}
