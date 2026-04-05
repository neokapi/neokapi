import { useCallback, useRef, useState } from "react";
import type { KapiProject } from "../types/api";

const MAX_UNDO = 50;

export interface ProjectHistory {
  /** Current project state (source of truth while editing). */
  project: KapiProject;
  /** Update the project (pushes to undo stack). */
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
 * The hook owns the project state — callers get `project` and `update()`.
 * Reset happens only when `tabId` changes (tab switch), not on every render.
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

  // Reset history only when the tab changes.
  if (tabId !== tabIdRef.current) {
    tabIdRef.current = tabId;
    undoStack.current = [];
    redoStack.current = [];
    savedRef.current = JSON.stringify(initialProject);
    // Force state update for the new tab's project.
    // This is safe in render — React handles it as an initial state correction.
    setProject(initialProject);
  }

  const update = useCallback((next: KapiProject) => {
    setProject((prev) => {
      if (JSON.stringify(prev) === JSON.stringify(next)) return prev;
      undoStack.current = [...undoStack.current.slice(-(MAX_UNDO - 1)), prev];
      redoStack.current = [];
      return next;
    });
  }, []);

  const replace = useCallback((next: KapiProject) => {
    setProject(next);
    undoStack.current = [];
    redoStack.current = [];
  }, []);

  const undo = useCallback(() => {
    setProject((prev) => {
      const stack = undoStack.current;
      if (stack.length === 0) return prev;
      const restored = stack[stack.length - 1];
      undoStack.current = stack.slice(0, -1);
      redoStack.current = [...redoStack.current, prev];
      return restored;
    });
  }, []);

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
    canUndo: undoStack.current.length > 0,
    canRedo: redoStack.current.length > 0,
  };
}
