"use client";

import * as React from "react";

import { HelpPanel } from "./HelpPanel";
import { readCompleted, writeCompleted } from "./storage";
import { TourOverlay } from "./TourOverlay";
import type { Guide, GuideApi, HelpTopic } from "./types";

export interface GuideProviderProps {
  /** Every tour the app offers. */
  guides: Guide[];
  /** Every help topic the app offers. */
  topics: HelpTopic[];
  /** Opens a website link, such as a topic's `moreUrl`, outside the app. */
  onOpenUrl: (url: string) => void;
  /** The localStorage key that holds the ids of finished guides. */
  storageKey: string;
  children?: React.ReactNode;
}

const GuideContext = React.createContext<GuideApi | null>(null);

/**
 * How long a start from the Help panel waits for the panel to report that it
 * has closed before the tour begins anyway, in ms.
 */
const PANEL_CLOSE_FALLBACK = 400;

/**
 * Holds the guide state for the app: the running tour, the Help panel and the
 * guides finished on this machine. It renders the tour overlay while a tour
 * runs and the Help panel while it is open.
 */
export function GuideProvider({
  guides,
  topics,
  onOpenUrl,
  storageKey,
  children,
}: GuideProviderProps) {
  const [running, setRunning] = React.useState<{ guideId: string; step: number } | null>(null);
  const [help, setHelp] = React.useState<{ open: boolean; topicId: string | null }>({
    open: false,
    topicId: null,
  });
  const [completed, setCompleted] = React.useState<ReadonlySet<string>>(() =>
    readCompleted(storageKey),
  );
  const pendingStart = React.useRef<string | null>(null);
  const pendingTimer = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  React.useEffect(() => {
    setCompleted(readCompleted(storageKey));
  }, [storageKey]);

  React.useEffect(
    () => () => {
      if (pendingTimer.current) clearTimeout(pendingTimer.current);
    },
    [],
  );

  const guidesRef = React.useRef(guides);
  guidesRef.current = guides;
  const helpOpenRef = React.useRef(help.open);
  helpOpenRef.current = help.open;

  const findGuide = React.useCallback(
    (id: string) => guidesRef.current.find((g) => g.id === id),
    [],
  );

  const flushPendingStart = React.useCallback(() => {
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
    pendingTimer.current = null;
    const id = pendingStart.current;
    pendingStart.current = null;
    if (id) setRunning({ guideId: id, step: 0 });
  }, []);

  const start = React.useCallback(
    (guideId: string) => {
      const guide = findGuide(guideId);
      if (!guide || guide.steps.length === 0) {
        console.warn(`guide: no guide with steps has the id "${guideId}"`);
        return;
      }
      setRunning(null);
      if (helpOpenRef.current) {
        // The tour begins once the panel has closed, so the sheet's focus
        // trap and dim layer are gone before the overlay takes over.
        pendingStart.current = guideId;
        setHelp((h) => ({ ...h, open: false }));
        if (pendingTimer.current) clearTimeout(pendingTimer.current);
        pendingTimer.current = setTimeout(flushPendingStart, PANEL_CLOSE_FALLBACK);
        return;
      }
      setRunning({ guideId, step: 0 });
    },
    [findGuide, flushPendingStart],
  );

  const stop = React.useCallback(() => setRunning(null), []);

  const completedRef = React.useRef(completed);
  completedRef.current = completed;
  const runningRef = React.useRef(running);
  runningRef.current = running;

  const markCompleted = React.useCallback(
    (guideId: string) => {
      if (completedRef.current.has(guideId)) return;
      const updated = new Set(completedRef.current);
      updated.add(guideId);
      completedRef.current = updated;
      writeCompleted(storageKey, updated);
      setCompleted(updated);
    },
    [storageKey],
  );

  const next = React.useCallback(() => {
    const r = runningRef.current;
    if (!r) return;
    const guide = findGuide(r.guideId);
    if (!guide || r.step + 1 >= guide.steps.length) {
      if (guide) markCompleted(guide.id);
      runningRef.current = null;
      setRunning(null);
      return;
    }
    const moved = { ...r, step: r.step + 1 };
    runningRef.current = moved;
    setRunning(moved);
  }, [findGuide, markCompleted]);

  const back = React.useCallback(() => {
    setRunning((r) => (r && r.step > 0 ? { ...r, step: r.step - 1 } : r));
  }, []);

  const openHelp = React.useCallback((topicId?: string) => {
    setRunning(null);
    setHelp({ open: true, topicId: topicId ?? null });
  }, []);

  const topicsRef = React.useRef(topics);
  topicsRef.current = topics;
  const hasTopic = React.useCallback(
    (topicId: string) => topicsRef.current.some((tp) => tp.id === topicId),
    [],
  );

  const activeGuide = running ? guides.find((g) => g.id === running.guideId) : undefined;
  const active = React.useMemo(
    () => (running && activeGuide ? { guide: activeGuide, step: running.step } : null),
    [running, activeGuide],
  );

  const api = React.useMemo<GuideApi>(
    () => ({ start, stop, active, openHelp, hasTopic, completed }),
    [start, stop, active, openHelp, hasTopic, completed],
  );

  return (
    <GuideContext.Provider value={api}>
      {children}
      <HelpPanel
        open={help.open}
        onOpenChange={(open) => setHelp((h) => ({ ...h, open }))}
        topicId={help.topicId}
        onTopicChange={(topicId) => setHelp((h) => ({ ...h, topicId }))}
        guides={guides}
        topics={topics}
        completed={completed}
        onStartGuide={start}
        onOpenUrl={onOpenUrl}
        onClosed={flushPendingStart}
      />
      {active && (
        <TourOverlay
          guide={active.guide}
          step={active.step}
          onNext={next}
          onBack={back}
          onStop={stop}
        />
      )}
    </GuideContext.Provider>
  );
}

/** The guide API. Throws when called outside a `GuideProvider`. */
export function useGuide(): GuideApi {
  const api = React.useContext(GuideContext);
  if (!api) {
    throw new Error("useGuide must be called inside a <GuideProvider>.");
  }
  return api;
}

/** The guide API, or null outside a `GuideProvider`. */
export function useOptionalGuide(): GuideApi | null {
  return React.useContext(GuideContext);
}
