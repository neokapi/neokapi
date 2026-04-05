import { useCallback, useRef, useState } from "react";
import type { KapiProject } from "../types/api";

const MAX_UNDO = 50;

export interface ProjectHistory {
  /** Current project state. */
  project: KapiProject;
  /** Update the project (pushes to undo stack). */
  update: (project: KapiProject) => void;
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

function eq(a: KapiProject, b: KapiProject): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/**
 * Manages project undo/redo history and dirty-state tracking.
 *
 * The hook owns the project state — callers get `project` and `update()`.
 * `markSaved()` snapshots the current state as the "clean" baseline.
 */
export function useProjectHistory(initial: KapiProject): ProjectHistory {
  const [project, setProject] = useState(initial);
  const undoStack = useRef<KapiProject[]>([]);
  const redoStack = useRef<KapiProject[]>([]);
  const savedRef = useRef<string>(JSON.stringify(initial));

  // When the external project changes (e.g. tab switch), reset history.
  const initialRef = useRef(initial);
  if (initial !== initialRef.current && !eq(initial, initialRef.current)) {
    initialRef.current = initial;
    undoStack.current = [];
    redoStack.current = [];
    savedRef.current = JSON.stringify(initial);
    // Return will use the new initial via useState's initial value on next render.
  }

  const update = useCallback((next: KapiProject) => {
    setProject((prev) => {
      if (eq(prev, next)) return prev;
      undoStack.current = [...undoStack.current.slice(-(MAX_UNDO - 1)), prev];
      redoStack.current = [];
      return next;
    });
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
    undo,
    redo,
    markSaved,
    isDirty,
    canUndo: undoStack.current.length > 0,
    canRedo: redoStack.current.length > 0,
  };
}
