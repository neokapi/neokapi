import { useCallback, useEffect, useReducer, useRef } from "react";
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

/** A tab's timeline: manual archive mode, so undo steps are the debounced
 *  batches committed by archive(). */
type ProjectTravels = Travels<KapiProject, false, false>;

function makeTravels(initial: KapiProject): ProjectTravels {
  return createTravels(toPlain(initial), {
    maxHistory: MAX_UNDO,
    autoArchive: false,
  });
}

interface TabHistory {
  timeline: ProjectTravels;
  saved: string;
  timer?: ReturnType<typeof setTimeout>;
}

export function useProjectHistory(initialProject: KapiProject, tabId: string | null) {
  const histories = useRef(new Map<string, TabHistory>()).current;
  const [, refresh] = useReducer((n: number) => n + 1, 0);
  const key = tabId ?? "";
  let history = histories.get(key);
  if (!history) {
    history = { timeline: makeTravels(initialProject), saved: serialize(initialProject) };
    histories.set(key, history);
  }
  const entry = history;
  const [state, , controls] = useTravelStore<KapiProject, false, false>(entry.timeline);

  useEffect(
    () => () => {
      for (const tab of histories.values()) clearTimeout(tab.timer);
    },
    [histories],
  );

  const set = useCallback(
    (project: KapiProject, id = key) => {
      const tab = histories.get(id);
      if (!tab) return;
      tab.timeline.setState(() => toPlain(project));
      clearTimeout(tab.timer);
      tab.timer = setTimeout(() => {
        if (tab.timeline.canArchive()) tab.timeline.archive();
      }, ARCHIVE_DEBOUNCE_MS);
    },
    [histories, key],
  );

  const replace = useCallback(
    (project: KapiProject) => {
      clearTimeout(entry.timer);
      histories.set(key, { timeline: makeTravels(project), saved: serialize(project) });
      refresh();
    },
    [histories, key, entry],
  );

  const undo = useCallback(() => {
    clearTimeout(entry.timer);
    if (controls.canArchive()) controls.archive();
    controls.back();
  }, [controls, entry]);

  const markSaved = useCallback(
    (id = key, project: KapiProject = state, savedName = project.name) => {
      const tab = histories.get(id);
      if (tab) {
        if (savedName !== project.name && tab.timeline.getState().name === project.name) {
          if (tab.timeline.canArchive()) tab.timeline.archive();
          tab.timeline.setState({ ...tab.timeline.getState(), name: savedName });
          tab.timeline.archive();
        }
        tab.saved = serialize({ ...project, name: savedName });
        refresh();
      }
    },
    [histories, key, state],
  );

  const hasChanges = useCallback(
    (id?: string) => {
      const dirty = (tab: TabHistory) => serialize(tab.timeline.getState()) !== tab.saved;
      if (id === undefined) return Array.from(histories.values()).some(dirty);
      const tab = histories.get(id);
      return !!tab && dirty(tab);
    },
    [histories],
  );

  const cleanup = useCallback(
    (id: string) => {
      clearTimeout(histories.get(id)?.timer);
      histories.delete(id);
    },
    [histories],
  );

  return {
    project: state,
    set,
    replace,
    undo,
    redo: () => controls.forward(),
    markSaved,
    isDirty: hasChanges(key),
    canUndo: controls.canUndo,
    canRedo: controls.canRedo,
    snapshot: (id: string) => histories.get(id)?.timeline.getState(),
    hasChanges,
    cleanup,
  };
}
