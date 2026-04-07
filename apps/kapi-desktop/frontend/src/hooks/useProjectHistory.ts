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

function makeTravels(initial: KapiProject) {
  return createTravels(toPlain(initial), {
    maxHistory: MAX_UNDO,
    autoArchive: false,
  }) as unknown as Travels<KapiProject, false, true>;
}

/**
 * Manages project undo/redo history and dirty-state tracking using
 * mutative/travels for patch-based storage.
 *
 * Each tab gets its own persistent Travels instance (stored in a Map).
 * Switching tabs swaps the active instance without destroying history.
 * Undo boundaries are debounced at ARCHIVE_DEBOUNCE_MS.
 */
export function useProjectHistory(
  initialProject: KapiProject,
  tabId: string | null,
): ProjectHistory {
  // Persistent map of Travels instances and saved snapshots per tab.
  const instancesRef = useRef(new Map<string, Travels<KapiProject, false, true>>());
  const savedMapRef = useRef(new Map<string, string>());
  const tabIdRef = useRef(tabId);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Get or create the Travels instance for the current tab.
  const key = tabId ?? "";
  if (!instancesRef.current.has(key)) {
    instancesRef.current.set(key, makeTravels(initialProject));
    savedMapRef.current.set(key, serialize(initialProject));
  }

  // On tab switch, flush any pending debounce from the previous tab.
  if (tabId !== tabIdRef.current) {
    tabIdRef.current = tabId;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = null;
  }

  // useState to hold the active instance — triggers re-render on tab switch.
  const [activeInstance, setActiveInstance] = useState(() => instancesRef.current.get(key)!);
  if (activeInstance !== instancesRef.current.get(key)) {
    setActiveInstance(instancesRef.current.get(key)!);
  }

  const [state, setState, rawControls] = useTravelStore(activeInstance);
  // Cast to manual controls — autoArchive is false so canArchive/archive are available.
  const controls = rawControls as typeof rawControls & {
    canArchive: () => boolean;
    archive: () => void;
  };

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

  const replace = useCallback(
    (project: KapiProject) => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = null;
      const inst = makeTravels(project);
      instancesRef.current.set(key, inst);
      savedMapRef.current.set(key, serialize(project));
      setActiveInstance(inst);
    },
    [key],
  );

  const undo = useCallback(() => {
    flushArchive();
    controls.back();
  }, [controls, flushArchive]);

  const redo = useCallback(() => {
    controls.forward();
  }, [controls]);

  const markSaved = useCallback(() => {
    savedMapRef.current.set(key, serialize(state));
  }, [key, state]);

  const isDirty = serialize(state) !== (savedMapRef.current.get(key) ?? "");

  /** Remove a tab's history (call when closing a tab). */
  const cleanup = useCallback((id: string) => {
    instancesRef.current.delete(id);
    savedMapRef.current.delete(id);
  }, []);

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
    /** @internal Remove a closed tab's history. */
    cleanup,
  } as ProjectHistory & { cleanup: (id: string) => void };
}
