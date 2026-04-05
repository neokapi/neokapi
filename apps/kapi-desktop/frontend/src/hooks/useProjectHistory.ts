import { useCallback, useRef, useState } from "react";
import type { KapiProject } from "../types/api";

const MAX_UNDO = 50;
/** Pause in ms before a new undo boundary is created. */
const UNDO_DEBOUNCE_MS = 400;

export interface ProjectHistory {
  /** Current project state (source of truth while editing). */
  project: KapiProject;
  /** Update the project. Undo boundaries are debounced — rapid edits
   *  (e.g. typing) are grouped into a single undo step. */
  update: (project: KapiProject) => void;
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
 * Manages project undo/redo history and dirty-state tracking.
 *
 * Undo boundaries are **debounced**: rapid calls to `update()` (e.g. keystroke-
 * by-keystroke typing) are grouped into a single undo step. A new undo entry
 * is only created after a pause of UNDO_DEBOUNCE_MS.
 *
 * The hook owns the project state — callers get `project` and `update()`.
 * Reset happens only when `tabId` changes (tab switch).
 * `markSaved()` snapshots the current state as the "clean" baseline.
 */
export function useProjectHistory(
  initialProject: KapiProject,
  tabId: string | null,
): ProjectHistory {
  const [project, setProject] = useState(initialProject);
  const undoStack = useRef<KapiProject[]>([]);
  const redoStack = useRef<KapiProject[]>([]);
  const savedRef = useRef<string>(JSON.stringify(initialProject));
  const tabIdRef = useRef(tabId);

  // Debounce state: the snapshot before the current burst of edits started.
  const pendingBaseRef = useRef<KapiProject | null>(null);
  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Reset history only when the tab changes.
  if (tabId !== tabIdRef.current) {
    tabIdRef.current = tabId;
    undoStack.current = [];
    redoStack.current = [];
    pendingBaseRef.current = null;
    if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
    debounceTimerRef.current = null;
    savedRef.current = JSON.stringify(initialProject);
    setProject(initialProject);
  }

  const pushUndo = useCallback((snapshot: KapiProject) => {
    undoStack.current = [...undoStack.current.slice(-(MAX_UNDO - 1)), snapshot];
    redoStack.current = [];
  }, []);

  const update = useCallback(
    (next: KapiProject) => {
      setProject((prev) => {
        if (JSON.stringify(prev) === JSON.stringify(next)) return prev;

        // On the first edit in a burst, capture the base state.
        if (pendingBaseRef.current === null) {
          pendingBaseRef.current = prev;
        }

        // Reset the debounce timer.
        if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
        debounceTimerRef.current = setTimeout(() => {
          // Timer fired — commit the base snapshot to the undo stack.
          if (pendingBaseRef.current !== null) {
            pushUndo(pendingBaseRef.current);
            pendingBaseRef.current = null;
          }
        }, UNDO_DEBOUNCE_MS);

        return next;
      });
    },
    [pushUndo],
  );

  const replace = useCallback((next: KapiProject) => {
    pendingBaseRef.current = null;
    if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
    debounceTimerRef.current = null;
    undoStack.current = [];
    redoStack.current = [];
    setProject(next);
  }, []);

  const undo = useCallback(() => {
    // Flush any pending debounce before undoing.
    if (pendingBaseRef.current !== null) {
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
      debounceTimerRef.current = null;
      pushUndo(pendingBaseRef.current);
      pendingBaseRef.current = null;
    }

    setProject((prev) => {
      const stack = undoStack.current;
      if (stack.length === 0) return prev;
      const restored = stack[stack.length - 1];
      undoStack.current = stack.slice(0, -1);
      redoStack.current = [...redoStack.current, prev];
      return restored;
    });
  }, [pushUndo]);

  const redo = useCallback(() => {
    setProject((prev) => {
      const stack = redoStack.current;
      if (stack.length === 0) return prev;
      const restored = stack[stack.length - 1];
      redoStack.current = stack.slice(0, -1);
      undoStack.current = [...undoStack.current, prev];
      return restored;
    });
  }, []);

  const markSaved = useCallback(() => {
    setProject((prev) => {
      savedRef.current = JSON.stringify(prev);
      return prev;
    });
  }, []);

  const isDirty = JSON.stringify(project) !== savedRef.current;

  return {
    project,
    update,
    replace,
    undo,
    redo,
    markSaved,
    isDirty,
    canUndo: undoStack.current.length > 0 || pendingBaseRef.current !== null,
    canRedo: redoStack.current.length > 0,
  };
}
