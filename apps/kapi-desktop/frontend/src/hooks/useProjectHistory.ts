import { useCallback, useRef, useState } from "react";
import { createTravels, type Travels } from "travels";
import { useTravelStore } from "use-travel";
import type { KapiProject } from "../types/api";

const MAX_UNDO = 50;
/** Pause in ms before a batch of edits is committed as an undo step. */
const ARCHIVE_DEBOUNCE_MS = 400;

export interface ProjectHistory {
  /** Current project state (source of truth while editing). */
  project: KapiProject;
  /** Update by replacing the entire project object. Undo boundaries are debounced. */
  set: (project: KapiProject) => void;
  /** Replace project without pushing to undo (e.g. after applying a preset). */
  replace: (project: KapiProject) => void;
  /** Undo the last change. */
  undo: () => void;
  /** Redo the last undone change. */
  redo: () => void;
  /** Mark current state as saved (clears dirty flag). */
  markSaved: () => void;
  /** Whether the project has unsaved changes. */
  isDirty: boolean;
  canUndo: boolean;
  canRedo: boolean;
}

function serialize(p: KapiProject): string {
  return JSON.stringify(p);
}

/** Deep-clone to plain objects — Wails bindings return class instances
 *  which mutative's create() doesn't accept. */
function toPlain(p: KapiProject): KapiProject {
  return JSON.parse(JSON.stringify(p));
}

function makeTravels(initial: KapiProject): Travels<KapiProject> {
  return createTravels(toPlain(initial), {
    maxHistory: MAX_UNDO,
    autoArchive: false,
  }) as Travels<KapiProject>;
}

/**
 * Manages project undo/redo history and dirty-state tracking using
 * mutative/travels for patch-based storage.
 *
 * Undo boundaries are debounced: rapid `set()` calls (e.g. typing) are
 * grouped into a single undo step via manual archive after ARCHIVE_DEBOUNCE_MS.
 *
 * On tab switch (tabId change), a fresh Travels instance is created with
 * the new tab's project as the initial state. Uses useState (not useRef)
 * so React re-renders and useTravelStore binds to the new instance.
 */
export function useProjectHistory(
  initialProject: KapiProject,
  tabId: string | null,
): ProjectHistory {
  const tabIdRef = useRef(tabId);
  const [travels, setTravels] = useState(() => makeTravels(initialProject));
  const savedRef = useRef(serialize(initialProject));
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Recreate travels instance when the tab changes.
  // Setting state during render is the React pattern for derived state
  // (equivalent to getDerivedStateFromProps).
  if (tabId !== tabIdRef.current) {
    tabIdRef.current = tabId;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = null;
    const next = makeTravels(initialProject);
    setTravels(next);
    savedRef.current = serialize(initialProject);
  }

  const [state, setState, controls] = useTravelStore(travels);

  const scheduleArchive = useCallback(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      debounceRef.current = null;
      if (controls.canArchive()) controls.archive();
    }, ARCHIVE_DEBOUNCE_MS);
  }, [controls]);

  const flushArchive = useCallback(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
      debounceRef.current = null;
    }
    if (controls.canArchive()) controls.archive();
  }, [controls]);

  const set = useCallback(
    (project: KapiProject) => {
      const plain = toPlain(project);
      setState(() => plain);
      scheduleArchive();
    },
    [setState, scheduleArchive],
  );

  const replace = useCallback((project: KapiProject) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = null;
    const next = makeTravels(project);
    setTravels(next);
    savedRef.current = serialize(project);
  }, []);

  const undo = useCallback(() => {
    flushArchive();
    controls.back();
  }, [controls, flushArchive]);

  const redo = useCallback(() => {
    controls.forward();
  }, [controls]);

  const markSaved = useCallback(() => {
    savedRef.current = serialize(state);
  }, [state]);

  const isDirty = serialize(state) !== savedRef.current;

  return {
    project: state as KapiProject,
    set,
    replace,
    undo,
    redo,
    markSaved,
    isDirty,
    canUndo: controls.canBack(),
    canRedo: controls.canForward(),
  };
}
