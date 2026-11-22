// Live feed updates: the workspace event stream (GET /:ws/events?project=),
// read as server-sent events over a streaming fetch. A change of a type the
// feed is filed from refetches the feed's first page, so the new entry
// appears under Today. The stream reconnects with a growing delay and stops
// when the screen closes or the app goes to the background.
import { isFeedSourceEvent } from "@neokapi/bowrain-client";
import { useQueryClient } from "@tanstack/react-query";
import { fetch } from "expo/fetch";
import { useEffect, useRef } from "react";
import { AppState } from "react-native";

import { useAuth } from "./auth";
import { feedKey } from "./feed";
import { changeTypeOf, createSseParser } from "./sse";

/** How long after a relayed event the feed is read: the server files the entry just after. */
const SETTLE_MS = 1500;
const MAX_BACKOFF_MS = 60_000;

/** Keeps a project's feed current while the screen is open. */
export function useFeedEvents(ws: string, project: string): void {
  const { session } = useAuth();
  const qc = useQueryClient();
  const tokenRef = useRef(session?.accessToken);
  tokenRef.current = session?.accessToken;
  const serverUrl = session?.serverUrl;

  useEffect(() => {
    if (!serverUrl) return;
    let closed = false;
    let paused = false;
    let controller: AbortController | null = null;
    let retry: ReturnType<typeof setTimeout> | null = null;
    let settle: ReturnType<typeof setTimeout> | null = null;
    let backoff = 1000;
    const url = `${serverUrl}/api/v1/${encodeURIComponent(ws)}/events?project=${encodeURIComponent(project)}`;

    const refresh = () => {
      if (settle) return;
      settle = setTimeout(() => {
        settle = null;
        // The first page only: older pages hold no new entries.
        qc.setQueryData(
          feedKey(ws, project),
          (data: { pages: unknown[]; pageParams: unknown[] } | undefined) =>
            data
              ? { pages: data.pages.slice(0, 1), pageParams: data.pageParams.slice(0, 1) }
              : data,
        );
        void qc.invalidateQueries({ queryKey: feedKey(ws, project) });
      }, SETTLE_MS);
    };

    const schedule = () => {
      if (closed || paused) return;
      retry = setTimeout(connect, backoff);
      backoff = Math.min(backoff * 2, MAX_BACKOFF_MS);
    };

    const connect = async () => {
      retry = null;
      if (closed || paused) return;
      controller = new AbortController();
      try {
        const res = await fetch(url, {
          headers: {
            Accept: "text/event-stream",
            ...(tokenRef.current ? { Authorization: `Bearer ${tokenRef.current}` } : {}),
          },
          signal: controller.signal,
        });
        if (!res.ok || !res.body) throw new Error(`event stream answered ${res.status}`);
        backoff = 1000;
        // Entries filed while the stream was down.
        refresh();
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        const parse = createSseParser();
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          for (const data of parse(decoder.decode(value, { stream: true }))) {
            if (isFeedSourceEvent(changeTypeOf(data))) refresh();
          }
        }
      } catch {
        // A closed screen aborts the request; anything else reconnects.
      }
      controller = null;
      schedule();
    };

    const stop = () => {
      if (retry) clearTimeout(retry);
      retry = null;
      controller?.abort();
      controller = null;
    };

    void connect();
    const sub = AppState.addEventListener("change", (status) => {
      if (status === "active") {
        paused = false;
        if (!controller && !retry) void connect();
      } else {
        paused = true;
        stop();
      }
    });

    return () => {
      closed = true;
      sub.remove();
      stop();
      if (settle) clearTimeout(settle);
    };
  }, [serverUrl, ws, project, qc]);
}
