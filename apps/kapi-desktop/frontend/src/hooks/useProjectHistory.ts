import { useCallback, useRef, useState } from "react";
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

/**
 * Manages project undo/redo history and dirty-state tracking.
 *
 * Undo boundaries are debounced: rapid `set()` calls (e.g. typing) are
 * grouped into a single undo step. A new entry is committed to the undo
 * stack only after ARCHIVE_DEBOUNCE_MS of inactivity.
 *
 * Resets cleanly when `tabId` changes (tab switch) — the new tab's
 * project becomes the fresh baseline with empty undo/redo stacks.
 */
export function useProjectHistory(
  initialProject: KapiProject,
  tabId: string | null,
): ProjectHistory {
  const [project, setProject] = useState(initialProject);
  const undoStack = useRef<KapiProject[]>([]);
  const redoStack = useRef<KapiProject[]>([]);
  const savedRef = useRef(serialize(initialProject));
  const tabIdRef = useRef(tabId);
  const pendingBaseRef = useRef<KapiProject | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Reset when the tab changes — new project becomes the clean baseline.
  if (tabId !== tabIdRef.current) {
    tabIdRef.current = tabId;
    undoStack.current = [];
    redoStack.current = [];
    pendingBaseRef.current = null;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = null;
    savedRef.current = serialize(initialProject);
    setProject(initialProject);
  }

  const commitPending = useCallback(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
      debounceRef.current = null;
    }
    if (pendingBaseRef.current !== null) {
      undoStack.current = [...undoStack.current.slice(-(MAX_UNDO - 1)), pendingBaseRef.current];
      redoStack.current = [];
      pendingBaseRef.current = null;
    }
  }, []);

  const set = useCallback(
    (next: KapiProject) => {
      setProject((prev) => {
        if (serialize(prev) === serialize(next)) return prev;

        // Capture base state at start of edit burst.
        if (pendingBaseRef.current === null) {
          pendingBaseRef.current = prev;
        }

        // Reset debounce timer.
        if (debounceRef.current) clearTimeout(debounceRef.current);
        debounceRef.current = setTimeout(commitPending, ARCHIVE_DEBOUNCE_MS);

        return next;
      });
    },
    [commitPending],
  );

  const replace = useCallback((next: KapiProject) => {
    pendingBaseRef.current = null;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = null;
    undoStack.current = [];
    redoStack.current = [];
    setProject(next);
  }, []);

  const undo = useCallback(() => {
    // Flush any pending debounce before undoing.
    commitPending();

    setProject((prev) => {
      const stack = undoStack.current;
      if (stack.length === 0) return prev;
      const restored = stack[stack.length - 1];
      undoStack.current = stack.slice(0, -1);
      redoStack.current = [...redoStack.current, prev];
      return restored;
    });
  }, [commitPending]);

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
      savedRef.current = serialize(prev);
      return prev;
    });
  }, []);

  const isDirty = serialize(project) !== savedRef.current;

  return {
    project,
    set,
    replace,
    undo,
    redo,
    markSaved,
    isDirty,
    canUndo: undoStack.current.length > 0 || pendingBaseRef.current !== null,
    canRedo: redoStack.current.length > 0,
  };
}
