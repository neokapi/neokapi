// The feed wired to the backend: the workspace's operations on the home
// screen, one project's inside its Context hub.
//
// The panel follows the other processes without asking them anything. The
// backend's workspace watcher reads the operation log's head once a second and
// emits `workspace:changed` when it moves, and that refetches this. The
// previous answer stays on screen while the new one is read, so the list is
// never unmounted, the scroll position is kept, and a half-typed edit survives
// an agent recording something in the middle of it.

import { useCallback, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PanelHeader } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { AwaitingBadge, ContextFeedList, type ContextRuleEdit } from "./ContextFeed";
import { ContextRevertDialog } from "./ContextRevertDialog";
import { ContextWidenDialog } from "./ContextWidenDialog";
import type {
  ContextFeed,
  ContextFeedEntry,
  ContextFeedGroup,
  ContextRevertRequest,
} from "../types/api";

export interface ContextFeedPanelProps {
  /** Narrow to one project by its workspace key. */
  projectKey?: string;
  /** Narrow to the project a tab holds. */
  tabID?: string;
  /** The heading above the feed. Omitted renders the feed alone. */
  title?: string;
  /** Pre-loaded feed for Storybook and tests, which reach no backend. */
  feed?: ContextFeed;
  /** Take the keyboard. A panel that is not on screen does not. */
  keyboard?: boolean;
}

/** How many operations one read carries. */
export const CONTEXT_FEED_LIMIT = 200;

export function ContextFeedPanel({
  projectKey,
  tabID,
  title,
  feed: given,
  keyboard = true,
}: ContextFeedPanelProps) {
  const qc = useQueryClient();
  const [widening, setWidening] = useState<{ entry: ContextFeedEntry; to: string } | null>(null);
  const [reverting, setReverting] = useState<ContextRevertRequest | null>(null);

  const key = tabID ? qk.projectContextFeed(tabID) : qk.contextFeed(projectKey ?? "");
  const feedQuery = useQuery({
    queryKey: key,
    queryFn: () =>
      tabID
        ? api.projectContextFeed(tabID, CONTEXT_FEED_LIMIT)
        : api.contextFeed(projectKey ?? "", CONTEXT_FEED_LIMIT),
    enabled: !given,
    // The previous answer stays mounted through a refetch, so a refresh
    // triggered by another process keeps the scroll and any open edit.
    placeholderData: (previous) => previous,
  });

  // An agent's proposal reaches the screen through the workspace watcher, and
  // the project list carries the same counts.
  useInvalidateOnEvent("workspace:changed", [key, qk.contextAwaiting()]);

  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: key });
    void qc.invalidateQueries({ queryKey: qk.contextAwaiting() });
    void qc.invalidateQueries({ queryKey: qk.workspaceProjects() });
  }, [qc, key]);

  const confirm = useMutation({
    mutationFn: ({ entry, edit }: { entry: ContextFeedEntry; edit?: ContextRuleEdit }) =>
      api.confirmContextCandidate({
        project: entry.project_key,
        id: entry.id,
        replacement: edit?.replacement,
        severity: edit?.severity,
      }),
    onSettled: refresh,
  });
  const discard = useMutation({
    mutationFn: (entry: ContextFeedEntry) =>
      api.discardContextCandidate({ project: entry.project_key, id: entry.id }),
    onSettled: refresh,
  });
  const revert = useMutation({
    mutationFn: (request: ContextRevertRequest) => api.revertContextOperations(request),
    onSettled: refresh,
  });
  const widen = useMutation({
    mutationFn: ({ entry, to }: { entry: ContextFeedEntry; to: string }) =>
      api.widenContextRule({ project: entry.project_key, id: entry.id, widen_to: to }),
    onSettled: refresh,
  });

  const feed = given ?? feedQuery.data ?? null;

  return (
    <div data-slot="context-feed-panel">
      {title && (
        <PanelHeader title={title} actions={<AwaitingBadge count={feed?.awaiting_here ?? 0} />} />
      )}
      <ContextFeedList
        feed={feed}
        loading={feedQuery.isLoading}
        error={feedQuery.error}
        keyboard={keyboard}
        showProject={!projectKey && !tabID}
        onConfirm={async (entry, edit) => {
          await confirm.mutateAsync({ entry, edit });
        }}
        onDiscard={async (entry) => {
          await discard.mutateAsync(entry);
        }}
        onRevert={(entry: ContextFeedEntry) =>
          setReverting({ project: entry.project_key, id: entry.id })
        }
        onRevertSession={(group: ContextFeedGroup) =>
          setReverting({ project: group.project_key ?? "", session: group.session })
        }
        onWiden={(entry, to) => setWidening({ entry, to })}
      />

      <ContextWidenDialog
        entry={widening?.entry ?? null}
        to={widening?.to ?? ""}
        onClose={() => setWidening(null)}
        onConfirm={async (entry, to) => {
          await widen.mutateAsync({ entry, to });
        }}
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
