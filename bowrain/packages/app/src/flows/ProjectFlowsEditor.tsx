// The project's flows over the flow-definition REST API: a list of the
// built-in flows merged with the project's own, and one flow open in the
// shared linear editor. Edits save on their own after a short pause, and the
// pane says where the flow stands against the server.
//
// Flows are connector-agnostic: they apply to content from any connector, and
// the flow graph never names one.

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import { useToolSchemas } from "@neokapi/flow-editor";
import { Button, ErrorNotice, asFlowChanged, useApi } from "@neokapi/ui";
import type { FlowDefinitionInfo, LinearFlowSpec } from "@neokapi/ui";
import { definitionToSpec, specToDefinition, toEditorDefinition, toEditorTools } from "./flowGraph";
import { ProjectFlowList } from "./ProjectFlowList";
import { ProjectFlowPane, type FlowSaveState } from "./ProjectFlowPane";
import { FlowChangedDialog } from "./FlowChangedDialog";
import { useSchemaFormHost } from "./useSchemaFormHost";
import {
  discardPendingEdit,
  pendingEdits,
  recordPendingEdit,
  savePendingEdit,
  subscribePendingSaves,
  updatePendingEditData,
  type PendingEdit,
} from "../pendingSaves";

interface OpenFlow {
  flow: FlowDefinitionInfo;
  spec: LinearFlowSpec;
}

export interface ProjectFlowsEditorProps {
  workspaceSlug: string;
  projectId: string;
  /** How long an edit rests before it is saved. */
  saveDelayMs?: number;
}

/** The flow as it stands on the server, when a save was refused because it changed. */
function changedFlow(edit: PendingEdit | undefined): FlowDefinitionInfo | null {
  if (edit?.state !== "failed") return null;
  return asFlowChanged(edit.error)?.current ?? null;
}

export function ProjectFlowsEditor({
  workspaceSlug,
  projectId,
  saveDelayMs = 500,
}: ProjectFlowsEditorProps) {
  const api = useApi();
  const queryClient = useQueryClient();
  const flowsKey = useMemo(() => ["flows", workspaceSlug, projectId], [workspaceSlug, projectId]);

  const flowsQuery = useQuery({
    queryKey: flowsKey,
    queryFn: () => api.listFlowDefinitions(workspaceSlug, projectId),
    staleTime: 15_000,
  });
  const toolsQuery = useQuery({
    queryKey: ["tools"],
    queryFn: () => api.listTools(),
    staleTime: 5 * 60_000,
  });
  const flows = flowsQuery.data ?? [];
  const tools = useMemo(() => toEditorTools(toolsQuery.data ?? []), [toolsQuery.data]);

  // A step's options come from the tool's schema, fetched once per tool for
  // the life of the surface, and its credential picker from the workspace's
  // provider configurations.
  const fetchToolSchema = useCallback((toolName: string) => api.getToolSchema(toolName), [api]);
  const getSchema = useToolSchemas(fetchToolSchema);
  const host = useSchemaFormHost(workspaceSlug);

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: flowsKey }),
    [queryClient, flowsKey],
  );

  const [open, setOpenState] = useState<OpenFlow | null>(null);
  const [actionError, setActionError] = useState<unknown>(null);

  // The open flow as the surface last knew it, readable from a timer, which
  // sees no render. Every local change goes through setOpen so the ref and the
  // state agree.
  const openRef = useRef<OpenFlow | null>(null);
  const setOpen = useCallback((next: OpenFlow | null) => {
    openRef.current = next;
    setOpenState(next);
  }, []);

  // An edit that has not reached the server is recorded in the pending-saves
  // registry, keyed by flow, rather than in this component: it outlives the
  // editor, a route change and a lost session, belongs to the user who made
  // it, and stays until a save of that flow succeeds or the person discards
  // it. A save refused because someone else saved the flow first stays there
  // too, with the flow as it now stands, until the person decides.
  const scope = `flows:${workspaceSlug}/${projectId}`;
  const keyOf = useCallback((id: string) => `${scope}/${id}`, [scope]);
  const allPending = useSyncExternalStore(subscribePendingSaves, pendingEdits);
  const pending = useMemo(() => allPending.filter((e) => e.scope === scope), [allPending, scope]);
  const entryOf = useCallback(
    (id: string) => pendingEdits().find((e) => e.key === keyOf(id)),
    [keyOf],
  );

  // The refused save the dialog is open on, by record key.
  const [conflictKey, setConflictKey] = useState<string | null>(null);

  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const save = useCallback(
    (id: string) => {
      void savePendingEdit(keyOf(id)).catch((e: unknown) => {
        // The record holds the failure; the pane and the list show it. A save
        // refused because the flow changed elsewhere asks the person, and the
        // list shows the flow as it now stands.
        if (asFlowChanged(e)) {
          setConflictKey(keyOf(id));
          void invalidate();
        }
      });
    },
    [keyOf, invalidate],
  );

  /**
   * Records the open flow as it now stands, to be saved. The record carries
   * the revision its save builds on, read when the save is sent: saves of one
   * flow run one after another, and an edit made while the previous save was
   * on its way is moved onto the revision that save wrote.
   */
  const record = useCallback(
    (edit: OpenFlow) => {
      const { flow, spec } = edit;
      const key = keyOf(flow.id);
      recordPendingEdit({
        key,
        label: flow.name,
        scope,
        data: edit,
        send: async () => {
          const base = (entryOf(flow.id)?.data as OpenFlow | undefined)?.flow.revision;
          const saved = await api.updateFlowDefinition(
            workspaceSlug,
            projectId,
            flow.id,
            specToDefinition(spec, { ...flow, revision: base ?? flow.revision }),
          );
          // A newer edit recorded while this save was on its way builds on
          // the revision just written, not on the one this save replaced.
          updatePendingEditData(key, (data) => {
            const d = data as OpenFlow;
            return { ...d, flow: { ...d.flow, revision: saved.revision } };
          });
          // Every view of this flow saves against the revision just written:
          // an editor reopened from the list before its refetch arrives would
          // otherwise base its next save on the older revision and be refused
          // as if someone else had saved.
          queryClient.setQueryData<FlowDefinitionInfo[]>(flowsKey, (list) =>
            list?.map((f) => (f.id === saved.id ? saved : f)),
          );
          void invalidate();
          const now = openRef.current;
          if (now?.flow.id === flow.id) {
            setOpen({
              ...now,
              flow: {
                ...now.flow,
                created_at: saved.created_at,
                modified_at: saved.modified_at,
                revision: saved.revision,
              },
            });
          }
        },
      });
    },
    [
      api,
      workspaceSlug,
      projectId,
      scope,
      keyOf,
      entryOf,
      invalidate,
      setOpen,
      queryClient,
      flowsKey,
    ],
  );

  /**
   * Saves the open flow's resting edit now rather than after the pause. A
   * version refused because the flow changed elsewhere waits for the person's
   * choice and is not sent again.
   */
  const flush = useCallback(() => {
    clearTimeout(timerRef.current);
    const cur = openRef.current;
    if (!cur) return;
    const entry = entryOf(cur.flow.id);
    if (!entry || changedFlow(entry)) return;
    // A failed edit is retried too: it is still the person's latest word.
    save(cur.flow.id);
  }, [entryOf, save]);

  // A pause that outlives the surface still saves: leaving the page must not
  // drop the last edit, and the registry tracks that save after this unmounts.
  useEffect(() => flush, [flush]);

  /** Asks again about the open flow's refused version, on leaving it. */
  const askOnLeave = useCallback(() => {
    const cur = openRef.current;
    if (cur && changedFlow(entryOf(cur.flow.id))) setConflictKey(keyOf(cur.flow.id));
  }, [entryOf, keyOf]);

  const openPending = open ? pending.find((e) => e.key === keyOf(open.flow.id)) : undefined;
  const openChanged = changedFlow(openPending) != null;
  const saveState: FlowSaveState = !openPending
    ? "saved"
    : openChanged
      ? "changed"
      : openPending.state === "saving"
        ? "saving"
        : openPending.state === "failed"
          ? "error"
          : "unsaved";
  const saveError = openPending?.state === "failed" && !openChanged ? openPending.error : null;
  // Flows with a version the server refused because they changed elsewhere,
  // and the other edits the server has not taken.
  const unsavedIds = useMemo(
    () => new Set(pending.filter((e) => changedFlow(e)).map((e) => (e.data as OpenFlow).flow.id)),
    [pending],
  );
  const otherFailures = pending.filter(
    (e) => e.state === "failed" && e !== openPending && !changedFlow(e),
  );

  /**
   * Takes a change to the open flow. A flow whose save was refused because it
   * changed elsewhere keeps the change in its record and sends nothing until
   * the person decides; any other change saves after the pause.
   */
  const change = useCallback(
    (next: OpenFlow, at: "pause" | "now") => {
      setOpen(next);
      record(next);
      clearTimeout(timerRef.current);
      if (changedFlow(entryOf(next.flow.id))) return;
      if (at === "now") flush();
      else timerRef.current = setTimeout(flush, saveDelayMs);
    },
    [setOpen, record, entryOf, flush, saveDelayMs],
  );

  const handleChange = useCallback(
    (spec: LinearFlowSpec) => {
      const cur = openRef.current;
      if (!cur || cur.flow.source === "built-in") return;
      change({ flow: cur.flow, spec }, "pause");
    },
    [change],
  );

  /** A new name is a change like any other; it saves at once unless held. */
  const handleRename = useCallback(
    (name: string) => {
      const cur = openRef.current;
      if (!cur || cur.flow.source === "built-in") return;
      change({ ...cur, flow: { ...cur.flow, name } }, "now");
    },
    [change],
  );

  const conflictEntry = conflictKey ? pending.find((e) => e.key === conflictKey) : undefined;
  const conflictCurrent = changedFlow(conflictEntry);
  const conflictMine = conflictEntry?.data as OpenFlow | undefined;

  /** Keeps the flow as it now stands and drops the refused version. */
  const keepCurrent = useCallback(() => {
    if (!conflictKey || !conflictCurrent) return;
    discardPendingEdit(conflictKey);
    setConflictKey(null);
    void invalidate();
    if (openRef.current?.flow.id !== conflictCurrent.id) return;
    clearTimeout(timerRef.current);
    setOpen({ flow: conflictCurrent, spec: definitionToSpec(conflictCurrent) });
  }, [conflictKey, conflictCurrent, invalidate, setOpen]);

  /** Saves the person's version over the flow as it now stands. */
  const saveMine = useCallback(() => {
    if (!conflictKey || !conflictCurrent || !conflictMine) return;
    setConflictKey(null);
    // The record holds any change made since the refusal; the save is based
    // on the revision the dialog showed.
    const next = {
      ...conflictMine,
      flow: { ...conflictMine.flow, revision: conflictCurrent.revision },
    };
    if (openRef.current?.flow.id === next.flow.id) setOpen(next);
    record(next);
    save(next.flow.id);
  }, [conflictKey, conflictCurrent, conflictMine, setOpen, record, save]);

  /**
   * Closes the dialog without a choice. The person's version stays recorded
   * and on screen, marked as not saved; they are asked again when they ask
   * (Review) or leave the flow, never on each change.
   */
  const dismissConflict = useCallback(() => setConflictKey(null), []);

  const conflictDialog = (
    <FlowChangedDialog
      state={
        conflictCurrent && conflictMine
          ? {
              current: conflictCurrent,
              mine: specToDefinition(conflictMine.spec, conflictMine.flow),
            }
          : null
      }
      tools={tools}
      onKeep={keepCurrent}
      onDismiss={dismissConflict}
      onSaveMine={saveMine}
    />
  );

  const openFlow = useCallback(
    (flow: FlowDefinitionInfo) => {
      flush();
      askOnLeave();
      // An edit the server has not taken opens as the person left it.
      const unsaved = entryOf(flow.id)?.data as OpenFlow | undefined;
      setOpen(unsaved ?? { flow, spec: definitionToSpec(flow) });
      setActionError(null);
    },
    [flush, askOnLeave, setOpen, entryOf],
  );

  const handleBack = useCallback(() => {
    flush();
    askOnLeave();
    setOpen(null);
  }, [flush, askOnLeave, setOpen]);

  const createFlow = useCallback(
    async (def: FlowDefinitionInfo) => {
      const created = await api.createFlowDefinition(workspaceSlug, projectId, def);
      void invalidate();
      openFlow(created);
    },
    [api, workspaceSlug, projectId, invalidate, openFlow],
  );

  const handleCreate = useCallback(
    (name: string) =>
      createFlow({ id: "", name, description: "", source: "project", nodes: [], edges: [] }),
    [createFlow],
  );

  const handleCopy = useCallback(
    async (flow: FlowDefinitionInfo) => {
      const graph = toEditorDefinition(flow);
      try {
        await createFlow({
          id: "",
          name: t("Copy of {name}", { name: flow.name }),
          description: flow.description ?? "",
          source: "project",
          nodes: graph.nodes,
          edges: graph.edges,
        });
      } catch (e) {
        setActionError(e);
      }
    },
    [createFlow],
  );

  const handleDelete = useCallback(
    async (flow: FlowDefinitionInfo) => {
      if (openRef.current?.flow.id === flow.id) clearTimeout(timerRef.current);
      try {
        await api.deleteFlowDefinition(workspaceSlug, projectId, flow.id);
        // A deleted flow has no version left to decide about.
        discardPendingEdit(keyOf(flow.id));
        if (openRef.current?.flow.id === flow.id) setOpen(null);
        setActionError(null);
        void invalidate();
      } catch (e) {
        setActionError(e);
      }
    },
    [api, workspaceSlug, projectId, invalidate, setOpen, keyOf],
  );

  const unsavedNotices = otherFailures.map((e) => (
    <UnsavedFlowEdit
      key={e.key}
      edit={e}
      onRetry={() => save((e.data as OpenFlow).flow.id)}
      onDiscard={() => {
        discardPendingEdit(e.key);
        void invalidate();
      }}
    />
  ));

  if (open) {
    const readOnly = open.flow.source === "built-in";
    const openKey = keyOf(open.flow.id);
    return (
      <>
        {actionError != null && (
          <ErrorNotice error={actionError} title="Could not update the flows" className="mb-3" />
        )}
        {unsavedNotices}
        <ProjectFlowPane
          flow={open.flow}
          spec={open.spec}
          tools={tools}
          readOnly={readOnly}
          saveState={saveState}
          saveError={saveError}
          onReviewChange={openChanged ? () => setConflictKey(openKey) : undefined}
          onGetSchema={getSchema}
          host={host}
          onBack={handleBack}
          onChange={handleChange}
          onRename={handleRename}
          onCopy={readOnly ? () => void handleCopy(open.flow) : undefined}
          onDelete={readOnly ? undefined : () => void handleDelete(open.flow)}
        />
        {conflictDialog}
      </>
    );
  }

  return (
    <>
      {actionError != null && (
        <ErrorNotice error={actionError} title="Could not update the flows" className="mb-3" />
      )}
      {unsavedNotices}
      <ProjectFlowList
        flows={flows}
        tools={tools}
        loading={flowsQuery.isLoading}
        error={flowsQuery.error}
        onOpen={openFlow}
        onCreate={handleCreate}
        onCopy={(flow) => void handleCopy(flow)}
        onDelete={(flow) => void handleDelete(flow)}
        unsaved={unsavedIds}
      />
      {conflictDialog}
    </>
  );
}

/**
 * A flow edit the server did not take, shown until a retry saves it or the
 * person discards it.
 */
function UnsavedFlowEdit({
  edit,
  onRetry,
  onDiscard,
}: {
  edit: PendingEdit;
  onRetry: () => void;
  onDiscard: () => void;
}) {
  return (
    <div className="mb-3 flex flex-col gap-2" data-testid="unsaved-flow-edit">
      <ErrorNotice
        error={edit.error}
        title={t("Not saved: {name}", { name: edit.label })}
        onRetry={onRetry}
      />
      <div>
        <Button variant="outline" size="sm" onClick={onDiscard}>
          {t("Discard")}
        </Button>
      </div>
    </div>
  );
}
