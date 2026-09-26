// The digest wired to the backend, for the project a tab holds.
//
// "Since you last looked" is read once, when the panel opens: the first read
// uses the person's marker, the panel holds the instant it answered from, and
// the marker moves to now. Every refresh while the panel is open reads from
// the held instant, so what was new when the person arrived stays marked new
// until they leave, and an agent recording something meanwhile appears as new
// too. A person who has never looked holds the epoch, so everything is new.

import { useCallback, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { ContextDigestView } from "./ContextDigest";
import { ContextRevertDialog } from "./ContextRevertDialog";
import type { ContextRevertRequest, DigestItem } from "../types/api";

/** The instant a person who never looked reads from. */
const NEVER = "1970-01-01T00:00:00Z";

export interface ContextDigestPanelProps {
  tabID: string;
  /** Decisions can be written: the tab holds a checkout. */
  canDecide?: boolean;
  /** Open the explorer at a file the digest names. */
  onOpenFile?: (path: string) => void;
}

export function ContextDigestPanel({
  tabID,
  canDecide = true,
  onOpenFile,
}: ContextDigestPanelProps) {
  const qc = useQueryClient();
  // The instant this view reads from: null until the first read answers.
  const [held, setHeld] = useState<string | null>(null);
  const [reverting, setReverting] = useState<ContextRevertRequest | null>(null);

  const key = qk.projectContextDigest(tabID, held ?? "");
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.projectContextDigest(tabID, held ?? ""),
    placeholderData: (previous) => previous,
  });

  useEffect(() => {
    if (held !== null || !query.data) return;
    setHeld(query.data.since || NEVER);
    void api.markProjectContextDigestSeen(tabID);
  }, [held, query.data, tabID]);

  useInvalidateOnEvent("workspace:changed", [["context-digest", tabID]]);

  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ["context-digest", tabID] });
    void qc.invalidateQueries({ queryKey: ["context-feed"] });
    void qc.invalidateQueries({ queryKey: qk.contextAwaiting() });
  }, [qc, tabID]);

  const project = query.data?.project ?? "";
  const keep = useMutation({
    mutationFn: ({ item, replacement }: { item: DigestItem; replacement?: string }) =>
      api.keepContextSuggestion({ project, id: item.id, replacement }),
    onSettled: refresh,
  });
  const choose = useMutation({
    mutationFn: ({ item, replacement }: { item: DigestItem; replacement?: string }) =>
      api.chooseContextSide({ project, id: item.id, replacement }),
    onSettled: refresh,
  });
  const keepGroup = useMutation({
    mutationFn: (items: DigestItem[]) =>
      api.keepContextGroup(
        project,
        items.map((i) => i.id),
      ),
    onSettled: refresh,
  });
  const drop = useMutation({
    mutationFn: (item: DigestItem) => api.dropContextSuggestion({ project, id: item.id }),
    onSettled: refresh,
  });
  const revert = useMutation({
    mutationFn: (request: ContextRevertRequest) => api.revertContextOperations(request),
    onSettled: refresh,
  });

  const since = held && held !== NEVER ? held : undefined;
  return (
    <div data-slot="context-digest-panel">
      <ContextDigestView
        digest={query.data ?? null}
        loading={query.isLoading}
        error={query.error}
        since={since}
        canDecide={canDecide}
        onKeep={async (item, replacement) => {
          await keep.mutateAsync({ item, replacement });
        }}
        onChoose={async (item, replacement) => {
          await choose.mutateAsync({ item, replacement });
        }}
        onKeepGroup={async (items) => {
          await keepGroup.mutateAsync(items);
        }}
        onDrop={async (item) => {
          await drop.mutateAsync(item);
        }}
        onRevert={(item) => setReverting({ project, id: item.id })}
        onOpenFile={onOpenFile}
      />
      <ContextRevertDialog
        request={reverting}
        onClose={() => setReverting(null)}
        onConfirm={async (request) => {
          await revert.mutateAsync(request);
        }}
      />
    </div>
  );
}
