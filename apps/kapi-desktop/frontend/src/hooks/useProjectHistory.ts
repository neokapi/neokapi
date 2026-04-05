import { useCallback, useEffect, useRef } from "react";
import { useTravel } from "use-travel";
import type { KapiProject } from "../types/api";

const MAX_UNDO = 50;
/** Pause in ms before a batch of edits is committed as an undo step. */
const ARCHIVE_DEBOUNCE_MS = 400;

export interface ProjectHistory {
  /** Current project state (source of truth while editing). */
  project: KapiProject;
  /** Update the project via mutation function. Undo boundaries are debounced. */
  update: (recipe: (draft: KapiProject) => void) => void;
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

/**
 * Manages project undo/redo history and dirty-state tracking using
 * mutative/use-travel for patch-based storage.
 *
 * Undo boundaries are debounced: rapid `update()` calls are grouped
 * into a single undo step via manual archive after ARCHIVE_DEBOUNCE_MS.
 *
 * Reset happens only when `tabId` changes (tab switch).
 */
export function useProjectHistory(
  initialProject: KapiProject,
  tabId: string | null,
): ProjectHistory {
  const [state, setState, controls] = useTravel(initialProject, {
    maxHistory: MAX_UNDO,
    autoArchive: false,
  });

  const savedRef = useRef<string>(JSON.stringify(initialProject));
  const tabIdRef = useRef(tabId);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Reset when the tab changes.
  if (tabId !== tabIdRef.current) {
    tabIdRef.current = tabId;
    savedRef.current = JSON.stringify(initialProject);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = null;
    controls.reset();
  }

  // Clean up timer on unmount.
  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, []);

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

  const update = useCallback(
    (recipe: (draft: KapiProject) => void) => {
      setState(recipe);
      scheduleArchive();
    },
    [setState, scheduleArchive],
  );

  const set = useCallback(
    (project: KapiProject) => {
      setState(() => project);
      scheduleArchive();
    },
    [setState, scheduleArchive],
  );

  const replace = useCallback(
    (project: KapiProject) => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = null;
      setState(() => project);
      // Archive immediately so this becomes the new baseline.
      setTimeout(() => {
        if (controls.canArchive()) controls.archive();
      }, 0);
    },
    [setState, controls],
  );

  const undo = useCallback(() => {
    flushArchive();
    controls.back();
  }, [controls, flushArchive]);

  const redo = useCallback(() => {
    controls.forward();
  }, [controls]);

  const markSaved = useCallback(() => {
    savedRef.current = JSON.stringify(state);
  }, [state]);

  const isDirty = JSON.stringify(state) !== savedRef.current;

  return {
    project: state as KapiProject,
    update,
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
