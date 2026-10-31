// Decide for a project tab, drawn by @neokapi/project-screens' DecideView.
// This page reads the questions through the desktop backend and records each
// answer through the call `kapi rules` makes.

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  DecideView,
  answerArgs,
  type DecidePick,
  type ScreenTarget,
} from "@neokapi/project-screens";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import type { RuleAnswer, RuleDecision } from "../types/api";

export interface DecidePageProps {
  tabID: string;
  /** The Map row to open on (`suggested:<id>`, `conflict:<id>`). */
  defaultSelected?: string;
  /** Open the file a suggestion was seen in. Absent shows the file without a link. */
  onOpenFile?: (path: string) => void;
  /** Open the project's Map, from the empty state. */
  onOpenMap?: () => void;
}

/** How long a typed form rests before what keeping it does is read. */
const OTHER_DEBOUNCE_MS = 250;

/** Decide for the project a tab holds. */
export function DecidePage({ tabID, defaultSelected, onOpenFile, onOpenMap }: DecidePageProps) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: qk.rulesDecide(tabID),
    queryFn: () => api.rulesDecideView(tabID),
    placeholderData: (previous) => previous,
  });
  // A decision recorded here or elsewhere, and a file written or read again,
  // change the questions or their counts. A decision made here arrives as
  // workspace:changed, so the answer itself refetches nothing.
  useInvalidateOnEvent(
    ["workspace:changed", "project-files-changed", "project:extracted"],
    [qk.rulesDecide(tabID)],
  );

  // The home's "since you last looked" marker moves once Decide shows what
  // waits. A project whose news is only a rule kept elsewhere shows nothing
  // here, and its marker moves in History, which lists it.
  const shows = (query.data?.decisions.length ?? 0) > 0;
  const marked = useRef(false);
  useEffect(() => {
    if (!shows || marked.current) return;
    marked.current = true;
    void api.markProjectContextDigestSeen(tabID).then(() => {
      void qc.invalidateQueries({ queryKey: qk.workspaceProjects() });
    });
  }, [shows, tabID, qc]);

  const decide = useMutation({
    mutationFn: async ({
      d,
      answer,
      pick,
    }: {
      d: RuleDecision;
      answer: RuleAnswer;
      pick: DecidePick;
    }) => {
      const { id, form } = answerArgs(d, answer, pick);
      return api.decideRule(tabID, id, answer, form);
    },
    // Decide itself reads again on the workspace:changed the backend emits
    // for a recorded answer.
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["context-digest", tabID] });
      void qc.invalidateQueries({ queryKey: ["context-feed"] });
      void qc.invalidateQueries({ queryKey: qk.mapSummary(tabID) });
      void qc.invalidateQueries({ queryKey: qk.workspaceProjects() });
    },
  });

  // What keeping a typed form does, read once the person stops typing.
  const [typed, setTyped] = useState<{ id: string; form: string } | null>(null);
  const [settled, setSettled] = useState<{ id: string; form: string } | null>(null);
  useEffect(() => {
    const handle = window.setTimeout(() => setSettled(typed), OTHER_DEBOUNCE_MS);
    return () => window.clearTimeout(handle);
  }, [typed]);
  const consequence = useQuery({
    // Keyed on the view's read as well, so a typed form is counted again once
    // the view (and the backend's copy of its blocks) is.
    queryKey: [
      ...qk.rulesDecideConsequence(tabID, settled?.id ?? "", settled?.form ?? ""),
      query.dataUpdatedAt,
    ],
    queryFn: () => api.rulesDecideConsequence(tabID, settled!.id, settled!.form),
    enabled: !!settled?.id && !!settled.form.trim(),
  });

  return (
    <DecideView
      view={query.data ?? null}
      loading={query.isLoading}
      error={query.error}
      defaultSelected={defaultSelected}
      onAnswer={(d, answer, pick) => decide.mutateAsync({ d, answer, pick })}
      onOtherForm={(d, form) => setTyped({ id: d.id ?? "", form: form.trim() })}
      otherConsequence={(d, form) =>
        settled && settled.id === d.id && settled.form === form
          ? (consequence.data ?? undefined)
          : undefined
      }
      navigate={(target: ScreenTarget) => {
        if (target.kind === "map") onOpenMap?.();
        else if (target.kind === "reader") onOpenFile?.(target.path);
      }}
      canNavigate={(target) =>
        target.kind === "map" ? !!onOpenMap : target.kind === "reader" ? !!onOpenFile : false
      }
    />
  );
}
