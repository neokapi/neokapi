// Pull to refresh for every list and scroll screen. The spinner shows only
// while a refresh the person started by pulling is in flight. A refetch the
// query client starts on its own (on focus, on reconnect, after a decision)
// leaves `refreshing` false, so the native control never shows for it. On iOS
// the control pushes the content down by its height while it shows, so a
// spinner bound to background refetching would move the whole screen.
import { useCallback, useEffect, useRef, useState } from "react";

export interface PullRefresh {
  /** True from a pull until the refresh it started settles. */
  refreshing: boolean;
  /** Pass to `onRefresh` of a list or a `RefreshControl`. */
  onRefresh: () => void;
}

/**
 * Binds a list's refresh control to the person's pull. `refresh` runs on a
 * pull and returns a promise; the spinner holds until it settles, whether it
 * resolves or rejects. Spread the result on a FlatList or SectionList, or on
 * a `RefreshControl` for a ScrollView.
 */
export function usePullRefresh(refresh: () => Promise<unknown>): PullRefresh {
  const [refreshing, setRefreshing] = useState(false);
  const latest = useRef(refresh);
  useEffect(() => {
    latest.current = refresh;
  });
  // Counts the pulls in flight, so a second pull keeps the spinner up until
  // both settle.
  const inFlight = useRef(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const onRefresh = useCallback(() => {
    inFlight.current += 1;
    setRefreshing(true);
    const settle = () => {
      inFlight.current -= 1;
      if (inFlight.current === 0 && mounted.current) setRefreshing(false);
    };
    let pending: Promise<unknown>;
    try {
      pending = Promise.resolve(latest.current());
    } catch (err) {
      pending = Promise.reject(err);
    }
    // The screen shows a failed load in its own state; the spinner only ends.
    pending.then(settle, settle);
  }, []);

  return { refreshing, onRefresh };
}
