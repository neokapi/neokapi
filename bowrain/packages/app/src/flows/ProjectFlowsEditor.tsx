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
import { ErrorNotice, asFlowChanged, useApi } from "@neokapi/ui";
import type { FlowDefinitionInfo, LinearFlowSpec } from "@neokapi/ui";
import { definitionToSpec, specToDefinition, toEditorDefinition, toEditorTools } from "./flowGraph";
import { ProjectFlowList } from "./ProjectFlowList";
import { ProjectFlowPane, type FlowSaveState } from "./ProjectFlowPane";
import { FlowChangedDialog } from "./FlowChangedDialog";
import {
  discardPendingEdit,
  pendingEdit,
  pendingEdits,
  recordPendingEdit,
  subscribePendingSaves,
} from "../pendingSaves";
import { useSchemaFormHost } from "./useSchemaFormHost";

/** A save refused because the flow changed elsewhere, as pendingSaves keeps it. */
interface RefusedSave {
  /** The person's version, which the server did not take. */
  edit: OpenFlow;
  /** The flow as the server held it when it refused. */
  current: FlowDefinitionInfo;
}

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
  const [saveState, setSaveState] = useState<FlowSaveState>("saved");
  const [saveError, setSaveError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  // A save the server refused because someone saved the flow after it was
  // opened is recorded per flow (pendingSaves): the person's version and the
  // flow as it now stands. Only Keep the current flow or Save my version
  // clears it. conflictKey is the record the dialog is open on.
  const scope = `flows:${workspaceSlug}/${projectId}`;
  const keyOf = useCallback((flowId: string) => `${scope}/${flowId}`, [scope]);
  const records = useSyncExternalStore(subscribePendingSaves, pendingEdits);
  const refused = useMemo(
    () =>
      new Map(
        records
          .filter((r) => r.scope === scope)
          .map((r) => [r.key, r.data as RefusedSave] as const),
      ),
    [records, scope],
  );
  const unsavedIds = useMemo(
    () => new Set([...refused.values()].map((r) => r.edit.flow.id)),
    [refused],
  );
  const [conflictKey, setConflictKey] = useState<string | null>(null);
  const conflict = conflictKey ? (refused.get(conflictKey) ?? null) : null;

  // The open flow as the surface last knew it, readable from a timer or an
  // in-flight save, which see no render. Every local change goes through
  // setOpen so the ref and the state agree.
  const openRef = useRef<OpenFlow | null>(null);
  const setOpen = useCallback((next: OpenFlow | null) => {
    openRef.current = next;
    setOpenState(next);
  }, []);

  // Saves run one after another, each sending the flow as it stands when its
  // turn comes, so a rename and a resting edit never race for the last word.
  const dirtyRef = useRef(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const chainRef = useRef<Promise<void>>(Promise.resolve());

  const enqueueSave = useCallback(
    (snapshot: OpenFlow) => {
      chainRef.current = chainRef.current.then(async () => {
        // The latest state of the same flow, or the snapshot if another flow
        // has been opened since.
        const live = openRef.current;
        const edit = live && live.flow.id === snapshot.flow.id ? live : snapshot;
        const current = () => openRef.current?.flow.id === edit.flow.id;
        if (current()) setSaveState("saving");
        try {
          const saved = await api.updateFlowDefinition(
            workspaceSlug,
            projectId,
            edit.flow.id,
            specToDefinition(edit.spec, edit.flow),
          );
          // Every view of this flow saves against the revision just written:
          // an editor reopened from the list before its refetch arrives would
          // otherwise base its next save on the older revision and be refused
          // as if someone else had saved.
          queryClient.setQueryData<FlowDefinitionInfo[]>(flowsKey, (list) =>
            list?.map((f) => (f.id === saved.id ? saved : f)),
          );
          void invalidate();
          if (!current()) return;
          const now = openRef.current!;
          setOpen({
            ...now,
            flow: {
              ...now.flow,
              created_at: saved.created_at,
              modified_at: saved.modified_at,
              // The next save is based on this one.
              revision: saved.revision,
            },
          });
          setSaveError(null);
          setSaveState(dirtyRef.current ? "unsaved" : "saved");
        } catch (e) {
          const changed = asFlowChanged(e);
          if (changed) {
            // Someone saved the flow since it was opened. Nothing was written;
            // the person's version is recorded for the flow and they decide,
            // whichever flow is on screen now.
            const key = keyOf(edit.flow.id);
            recordPendingEdit({
              key,
              label: edit.flow.name,
              scope,
              error: changed,
              data: { edit, current: changed.current } satisfies RefusedSave,
            });
            setConflictKey(key);
            // The list shows the flow as it now stands.
            void invalidate();
            if (current()) dirtyRef.current = false;
            return;
          }
          if (!current()) return;
          dirtyRef.current = true;
          setSaveError(e);
          setSaveState("error");
        }
      });
    },
    [api, workspaceSlug, projectId, invalidate, setOpen, queryClient, flowsKey, keyOf, scope],
  );

  /** Saves a resting edit now rather than after the pause. */
  const flush = useCallback(() => {
    clearTimeout(timerRef.current);
    const cur = openRef.current;
    if (!dirtyRef.current || !cur) return;
    dirtyRef.current = false;
    enqueueSave(cur);
  }, [enqueueSave]);

  // A pause that outlives the surface still saves: leaving the page must not
  // drop the last edit.
  useEffect(() => flush, [flush]);

  const handleChange = useCallback(
    (spec: LinearFlowSpec) => {
      const cur = openRef.current;
      if (!cur || cur.flow.source === "built-in") return;
      const next = { flow: cur.flow, spec };
      setOpen(next);
      // A flow someone else changed waits for the person's decision: the
      // record follows their edits, and nothing is sent until they choose.
      const record = pendingEdit(keyOf(cur.flow.id));
      if (record) {
        recordPendingEdit({
          ...record,
          data: { ...(record.data as RefusedSave), edit: next } satisfies RefusedSave,
        });
        return;
      }
      dirtyRef.current = true;
      setSaveState("unsaved");
      clearTimeout(timerRef.current);
      timerRef.current = setTimeout(flush, saveDelayMs);
    },
    [setOpen, flush, saveDelayMs, keyOf],
  );

  /** Keeps the flow as it now stands and drops the refused version. */
  const keepCurrent = useCallback(() => {
    if (!conflict || !conflictKey) return;
    const { current } = conflict;
    discardPendingEdit(conflictKey);
    setConflictKey(null);
    void invalidate();
    if (openRef.current?.flow.id !== current.id) return;
    clearTimeout(timerRef.current);
    dirtyRef.current = false;
    setOpen({ flow: current, spec: definitionToSpec(current) });
    setSaveError(null);
    setSaveState("saved");
  }, [conflict, conflictKey, invalidate, setOpen]);

  /** Saves the person's version over the flow as it now stands. */
  const saveMine = useCallback(() => {
    if (!conflict || !conflictKey) return;
    const { edit, current } = conflict;
    discardPendingEdit(conflictKey);
    setConflictKey(null);
    const live = openRef.current;
    // The open flow holds any edit made since the refused save.
    const mine = live && live.flow.id === edit.flow.id ? live : edit;
    const next = { ...mine, flow: { ...mine.flow, revision: current.revision } };
    if (live && live.flow.id === edit.flow.id) setOpen(next);
    dirtyRef.current = false;
    enqueueSave(next);
  }, [conflict, conflictKey, enqueueSave, setOpen]);

  /**
   * Closes the dialog without a choice. The person's version stays recorded
   * and on screen, marked as not saved; they are asked again when they ask
   * (Review) or leave the flow, never on each change.
   */
  const dismissConflict = useCallback(() => setConflictKey(null), []);

  /** Asks again about the open flow's refused version, on leaving it. */
  const askOnLeave = useCallback(() => {
    const cur = openRef.current;
    if (cur && pendingEdit(keyOf(cur.flow.id))) setConflictKey(keyOf(cur.flow.id));
  }, [keyOf]);

  const conflictDialog = (
    <FlowChangedDialog
      state={
        conflict && {
          current: conflict.current,
          mine: specToDefinition(conflict.edit.spec, conflict.edit.flow),
        }
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
      // A version the server refused opens as the person left it.
      const record = pendingEdit(keyOf(flow.id));
      setOpen(record ? (record.data as RefusedSave).edit : { flow, spec: definitionToSpec(flow) });
      setSaveState("saved");
      setSaveError(null);
      setActionError(null);
    },
    [flush, setOpen, askOnLeave, keyOf],
  );

  const handleBack = useCallback(() => {
    flush();
    askOnLeave();
    setOpen(null);
  }, [flush, setOpen, askOnLeave]);

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

  /** The new name applies at once and rides the next save with any resting edit. */
  const handleRename = useCallback(
    (name: string) => {
      const cur = openRef.current;
      if (!cur || cur.flow.source === "built-in") return;
      setOpen({ ...cur, flow: { ...cur.flow, name } });
      dirtyRef.current = true;
      flush();
    },
    [setOpen, flush],
  );

  const handleDelete = useCallback(
    async (flow: FlowDefinitionInfo) => {
      if (openRef.current?.flow.id === flow.id) {
        clearTimeout(timerRef.current);
        dirtyRef.current = false;
      }
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

  if (open) {
    const readOnly = open.flow.source === "built-in";
    const openKey = keyOf(open.flow.id);
    const changedElsewhere = refused.has(openKey);
    return (
      <>
        {actionError != null && (
          <ErrorNotice error={actionError} title="Could not update the flows" className="mb-3" />
        )}
        <ProjectFlowPane
          flow={open.flow}
          spec={open.spec}
          tools={tools}
          readOnly={readOnly}
          saveState={changedElsewhere ? "changed" : saveState}
          saveError={saveError}
          onReviewChange={changedElsewhere ? () => setConflictKey(openKey) : undefined}
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
      <ProjectFlowList
        flows={flows}
        tools={tools}
        loading={flowsQuery.isLoading}
        error={flowsQuery.error ?? actionError}
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
