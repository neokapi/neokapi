// The explorer lab's session: what the stage needs loaded, and which chapter
// is in view.
//
// An explorer lab has no sandbox and no transcript. The poster's Play loads
// what the lab declares (the engine unless it says otherwise, and the plugins
// it names, with their download shown), and from then on a chapter is a view:
// the stage's props for it, and the narration beside it. The reader moves
// between chapters with Previous and Next, or from the list; nothing moves on
// a timer. A playground has no chapters: Play opens the stage and that is all.

import { useCallback, useRef, useState } from "react";
import {
  configurePlugins,
  bootEngine,
  ensurePlugin,
  getPluginState,
  subscribePlugins,
} from "@neokapi/kapi-playground/plugins";
import type { PluginId } from "@neokapi/kapi-playground/plugins";
import { onBootProgress } from "@neokapi/kapi-playground/runtime";
import type { BootProgress } from "@neokapi/kapi-playground/runtime";
import type { Chapter, Lab } from "../curriculum/types.ts";
import type { LabLink } from "../deeplink.ts";
import { formatLabLink } from "../deeplink.ts";
import { markChapter, markPosition } from "../progress.ts";
import type { LabAssets, SessionStatus } from "./types.ts";

export interface UseExplorerSessionOptions {
  lab: Lab;
  assets: LabAssets;
  link?: LabLink;
  onChapterChange?: (chapter: Chapter, index: number) => void;
}

export interface ExplorerSession {
  status: SessionStatus;
  error: string | null;
  bootProgress: BootProgress | null;
  /** What is loading, in a phrase, while the poster shows progress. */
  loading: string | null;
  current: number;
  /** Chapters the reader has viewed. */
  visited: ReadonlySet<string>;
  start: () => void;
  goTo: (index: number) => void;
  next: () => void;
  previous: () => void;
  shareLink: () => { url: string; tooLarge: boolean; bytes: number };
}

export function useExplorerSession({
  lab,
  assets,
  link,
  onChapterChange,
}: UseExplorerSessionOptions): ExplorerSession {
  const [status, setStatus] = useState<SessionStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [bootProgress, setBootProgress] = useState<BootProgress | null>(null);
  const [loading, setLoading] = useState<string | null>(null);
  const [current, setCurrent] = useState(0);
  const [visited, setVisited] = useState<ReadonlySet<string>>(new Set());
  const currentRef = useRef(0);
  const startedRef = useRef(false);

  const view = useCallback(
    (index: number) => {
      if (lab.chapters.length === 0) return;
      const i = Math.max(0, Math.min(lab.chapters.length - 1, index));
      currentRef.current = i;
      setCurrent(i);
      const ch = lab.chapters[i];
      setVisited((v) => (v.has(ch.id) ? v : new Set([...v, ch.id])));
      markChapter(lab.id, ch.id);
      markPosition(lab.id, ch.id);
      onChapterChange?.(ch, i);
    },
    [lab, onChapterChange],
  );

  const goTo = useCallback((index: number) => view(index), [view]);
  const next = useCallback(() => view(currentRef.current + 1), [view]);
  const previous = useCallback(() => view(currentRef.current - 1), [view]);

  const start = useCallback(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    setStatus("booting");
    setError(null);
    configurePlugins(assets);
    const off = onBootProgress((p) => setBootProgress(p.done ? null : p));
    // Mirror a plugin's download into the poster while it is the one loading.
    let watching: PluginId | null = null;
    const offPlugins = subscribePlugins(() => {
      if (!watching) return;
      const st = getPluginState().plugins[watching];
      if (st?.phase === "downloading" && st.progress) {
        const { loaded, total, frac } = st.progress;
        if (loaded !== undefined) setBootProgress({ loaded, total: total ?? null, done: false });
        else if (frac !== undefined)
          setBootProgress({ loaded: frac * 100, total: 100, done: false });
      }
    });
    void (async () => {
      try {
        if (lab.engine !== false) {
          setLoading("the kapi engine");
          await bootEngine();
          setBootProgress(null);
        }
        for (const id of lab.plugins ?? []) {
          watching = id as PluginId;
          setLoading(`the ${id} plugin`);
          setBootProgress(null);
          await ensurePlugin(id as PluginId);
          watching = null;
          setBootProgress(null);
        }
        setLoading(null);
        setStatus("ready");
        const wanted = link?.chapter ? lab.chapters.findIndex((c) => c.id === link.chapter) : -1;
        view(wanted > 0 ? wanted : 0);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        setStatus("error");
        startedRef.current = false;
      } finally {
        off();
        offPlugins();
        setLoading(null);
      }
    })();
  }, [assets, lab, link, view]);

  const shareLink = useCallback(() => {
    const path = typeof window !== "undefined" ? window.location.pathname : `/learn/${lab.id}`;
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    const ch = lab.chapters[currentRef.current];
    return { url: origin + formatLabLink(path, { chapter: ch?.id }), tooLarge: false, bytes: 0 };
  }, [lab]);

  return {
    status,
    error,
    bootProgress,
    loading,
    current,
    visited,
    start,
    goTo,
    next,
    previous,
    shareLink,
  };
}
