import { createContext, useContext, useState, useEffect, useMemo, type ReactNode } from "react";

/**
 * One step of the trail. Every step but the last is a place you can go back to;
 * the last is where you are, and carries no action.
 */
export interface BreadcrumbItem {
  label: string;
  onClick?: () => void;
  /** Test hook for the step, so "go up one level" stays addressable. */
  testId?: string;
}

interface BreadcrumbContextValue {
  /** The trail the shell derives from the URL. */
  base: BreadcrumbItem[];
  /** Steps the mounted route appends — the ones only it can name. */
  extra: BreadcrumbItem[];
  setExtra: (items: BreadcrumbItem[]) => void;
}

const BreadcrumbContext = createContext<BreadcrumbContextValue>({
  base: [],
  extra: [],
  setExtra: () => {},
});

/**
 * The trail is assembled from two sources because it is known in two places.
 *
 * The shell can read everything the URL says — workspace, section, project,
 * stream, the file a splat carries — and it derives that itself rather than
 * making every route repeat it. What the URL holds only as an opaque id — which
 * collection `?collection=m9fHXC1n` is, what a change-set is called — is known
 * to the route that fetched it, and it appends those with useBreadcrumbExtra.
 */
export function BreadcrumbProvider({
  base = [],
  children,
}: {
  base?: BreadcrumbItem[];
  children: ReactNode;
}) {
  const [extra, setExtra] = useState<BreadcrumbItem[]>([]);
  const value = useMemo<BreadcrumbContextValue>(() => ({ base, extra, setExtra }), [base, extra]);
  return <BreadcrumbContext.Provider value={value}>{children}</BreadcrumbContext.Provider>;
}

/** The full trail: what the shell derived, then what the route appended. */
export function useBreadcrumb(): BreadcrumbItem[] {
  const { base, extra } = useContext(BreadcrumbContext);
  return useMemo(() => [...base, ...extra], [base, extra]);
}

/**
 * Append steps for the current route; they clear when it unmounts.
 *
 * The effect keys on the labels rather than on the array, so a caller building
 * a fresh array each render — which every caller does — does not re-set state
 * on every render and re-render forever. Callbacks are read through the latest
 * array, so they never go stale even though they are not part of the key.
 */
export function useBreadcrumbExtra(items: BreadcrumbItem[]) {
  const { setExtra } = useContext(BreadcrumbContext);
  // A separator no label can contain, so two different trails cannot collide
  // into one key and leave the crumb stale.
  const key = items.map((i) => i.label).join("\u0000");
  const latest = useRefLike(items);
  useEffect(() => {
    setExtra(latest.current.map((i) => ({ ...i })));
    return () => setExtra([]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, setExtra]);
}

/** A ref that always holds the newest value, without an effect to update it. */
function useRefLike<T>(value: T): { current: T } {
  const [box] = useState(() => ({ current: value }));
  box.current = value;
  return box;
}

/**
 * Name the current page in a single step.
 *
 * The shorthand for a shell whose trail is one deep — ctrl's admin routes each
 * name themselves and nothing else — over the general
 * [useBreadcrumbExtra] form.
 */
export function useSetBreadcrumb(label: string) {
  useBreadcrumbExtra([{ label }]);
}
