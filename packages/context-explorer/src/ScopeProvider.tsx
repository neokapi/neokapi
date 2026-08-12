// The scope a subtree asks its questions at, and the source that answers them
// (Apache-2.0).
//
// The provider owns two things and nothing else: the tuple, and the source. A
// mount decides which dimensions are free by what it passes as `pinned` — every
// dimension not pinned is free, so a surface that forgets to declare one gets a
// movable dimension rather than a silently frozen view.

import { createContext, useCallback, useContext, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { LADDER, FILTERS, contextAddress, withDimension } from "./scope";
import type { Dimension, LadderRung, ScopeTuple } from "./scope";
import { ladderOf } from "./scope";
import { deriveCapabilities } from "./source";
import type { ContextDataSource, ExplorerCapabilities } from "./source";

export interface ScopeContextValue {
  scope: ScopeTuple;
  /** Move one dimension; every finer ladder rung is cleared. */
  setDimension: (d: Dimension, value: string | undefined) => void;
  /** Replace the whole tuple — what a breadcrumb rung and a result row do. */
  setScope: (next: ScopeTuple) => void;
  source: ContextDataSource;
  capabilities: ExplorerCapabilities;
  /** Dimensions the mount pins. */
  pinned: Dimension[];
  /** Dimensions the reader may move. */
  free: Dimension[];
  /** The tuple's `context://` address. */
  address: string;
  ladder: LadderRung[];
}

const ScopeContext = createContext<ScopeContextValue | undefined>(undefined);

export interface ScopeProviderProps {
  source: ContextDataSource;
  /**
   * The dimensions this mount fixes. Their values come from `scope` and cannot
   * be moved: kapi pins workspace/project/stream to the project on disk,
   * Bowrain pins nothing.
   */
  pinned?: Dimension[];
  /** The tuple to start at. Pinned dimensions must carry their values here. */
  scope: ScopeTuple;
  /** Called whenever the tuple moves, for surfaces that mirror it into a URL. */
  onScopeChange?: (next: ScopeTuple) => void;
  children: ReactNode;
}

export function ScopeProvider({
  source,
  pinned = [],
  scope: initial,
  onScopeChange,
  children,
}: ScopeProviderProps) {
  const [scope, setScopeState] = useState<ScopeTuple>(initial);

  const free = useMemo(
    () => [...LADDER, ...FILTERS].filter((d) => !pinned.includes(d)),
    // The array identity changes on every render at most mounts; its contents
    // are what matters.
    [pinned.join(",")],
  );

  const capabilities = useMemo(() => {
    const derived = deriveCapabilities(source);
    // A mount's pinning is the authority on which dimensions move: a source may
    // be able to answer across projects while this mount shows exactly one.
    return { ...derived, free: derived.free.length ? derived.free : free };
  }, [source, free]);

  const setScope = useCallback(
    (next: ScopeTuple) => {
      // Pinned values are the mount's, not the caller's: a rung or a result row
      // carrying a stale pin must not be able to move the view off its project.
      const held: ScopeTuple = { ...next };
      for (const d of pinned) held[d] = initial[d];
      setScopeState(held);
      onScopeChange?.(held);
    },
    [onScopeChange, pinned.join(","), initial],
  );

  const setDimension = useCallback(
    (d: Dimension, value: string | undefined) => {
      if (pinned.includes(d)) return;
      setScopeState((current) => {
        const next = withDimension(current, d, value);
        onScopeChange?.(next);
        return next;
      });
    },
    [onScopeChange, pinned.join(",")],
  );

  const value = useMemo<ScopeContextValue>(
    () => ({
      scope,
      setDimension,
      setScope,
      source,
      capabilities,
      pinned,
      free: capabilities.free,
      address: contextAddress(scope, capabilities.free),
      ladder: ladderOf(scope, capabilities.free),
    }),
    [scope, setDimension, setScope, source, capabilities, pinned.join(",")],
  );

  return <ScopeContext.Provider value={value}>{children}</ScopeContext.Provider>;
}

/** The scope and source of the nearest ScopeProvider. */
export function useScope(): ScopeContextValue {
  const ctx = useContext(ScopeContext);
  if (!ctx) throw new Error("useScope must be used inside a ScopeProvider");
  return ctx;
}
