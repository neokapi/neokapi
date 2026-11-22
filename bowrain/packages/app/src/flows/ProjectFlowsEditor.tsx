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
import { Button, ErrorNotice, useApi } from "@neokapi/ui";
import type { FlowDefinitionInfo, LinearFlowSpec } from "@neokapi/ui";
import { definitionToSpec, specToDefinition, toEditorDefinition, toEditorTools } from "./flowGraph";
import { ProjectFlowList } from "./ProjectFlowList";
import { ProjectFlowPane, type FlowSaveState } from "./ProjectFlowPane";
import { useSchemaFormHost } from "./useSchemaFormHost";
import {
  discardPendingEdit,
  pendingEdits,
  recordPendingEdit,
  savePendingEdit,
  subscribePendingSaves,
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
  // editor, a route change and a lost session, and stays until a save of that
  // flow succeeds or the person discards it.
  const scope = `flows:${workspaceSlug}/${projectId}`;
  const keyOf = useCallback((id: string) => `${scope}/${id}`, [scope]);
  const allPending = useSyncExternalStore(subscribePendingSaves, pendingEdits);
  const pending = useMemo(() => allPending.filter((e) => e.scope === scope), [allPending, scope]);

  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const save = useCallback(
    (id: string) => {
      // The record holds the failure; the pane and the list show it.
      void savePendingEdit(keyOf(id)).catch(() => undefined);
    },
    [keyOf],
  );

  /** Records the open flow as it now stands, to be saved. */
  const record = useCallback(
    (edit: OpenFlow) => {
      const { flow, spec } = edit;
      recordPendingEdit({
        key: keyOf(flow.id),
        label: flow.name,
        scope,
        data: edit,
        send: async () => {
          const saved = await api.updateFlowDefinition(
            workspaceSlug,
            projectId,
            flow.id,
            specToDefinition(spec, flow),
          );
          void invalidate();
          const now = openRef.current;
          if (now?.flow.id === flow.id) {
            setOpen({
              ...now,
              flow: { ...now.flow, created_at: saved.created_at, modified_at: saved.modified_at },
            });
          }
        },
      });
    },
    [api, workspaceSlug, projectId, scope, keyOf, invalidate, setOpen],
  );

  /** Saves the open flow's resting edit now rather than after the pause. */
  const flush = useCallback(() => {
    clearTimeout(timerRef.current);
    const cur = openRef.current;
    if (!cur) return;
    // A failed edit is retried too: it is still the person's latest word.
    if (pendingEdits().some((e) => e.key === keyOf(cur.flow.id))) save(cur.flow.id);
  }, [keyOf, save]);

  // A pause that outlives the surface still saves: leaving the page must not
  // drop the last edit, and the registry tracks that save after this unmounts.
  useEffect(() => flush, [flush]);

  const openPending = open ? pending.find((e) => e.key === keyOf(open.flow.id)) : undefined;
  const saveState: FlowSaveState = !openPending
    ? "saved"
    : openPending.state === "saving"
      ? "saving"
      : openPending.state === "failed"
        ? "error"
        : "unsaved";
  const saveError = openPending?.state === "failed" ? openPending.error : null;
  // Edits to flows other than the open one that the server has not taken.
  const otherFailures = pending.filter((e) => e.state === "failed" && e !== openPending);

  const handleChange = useCallback(
    (spec: LinearFlowSpec) => {
      const cur = openRef.current;
      if (!cur || cur.flow.source === "built-in") return;
      const next = { flow: cur.flow, spec };
      setOpen(next);
      record(next);
      clearTimeout(timerRef.current);
      timerRef.current = setTimeout(flush, saveDelayMs);
    },
    [setOpen, record, flush, saveDelayMs],
  );

  const openFlow = useCallback(
    (flow: FlowDefinitionInfo) => {
      flush();
      // An edit the server has not taken opens as the person left it.
      const unsaved = pendingEdits().find((e) => e.key === keyOf(flow.id))?.data as
        | OpenFlow
        | undefined;
      setOpen(unsaved ?? { flow, spec: definitionToSpec(flow) });
      setActionError(null);
    },
    [flush, setOpen, keyOf],
  );

  const handleBack = useCallback(() => {
    flush();
    setOpen(null);
  }, [flush, setOpen]);

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
      const next = { ...cur, flow: { ...cur.flow, name } };
      setOpen(next);
      record(next);
      flush();
    },
    [setOpen, record, flush],
  );

  const handleDelete = useCallback(
    async (flow: FlowDefinitionInfo) => {
      if (openRef.current?.flow.id === flow.id) clearTimeout(timerRef.current);
      try {
        await api.deleteFlowDefinition(workspaceSlug, projectId, flow.id);
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
      onRetry={() => void savePendingEdit(e.key).catch(() => undefined)}
      onDiscard={() => {
        discardPendingEdit(e.key);
        void invalidate();
      }}
    />
  ));

  if (open) {
    const readOnly = open.flow.source === "built-in";
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
          onGetSchema={getSchema}
          host={host}
          onBack={handleBack}
          onChange={handleChange}
          onRename={handleRename}
          onCopy={readOnly ? () => void handleCopy(open.flow) : undefined}
          onDelete={readOnly ? undefined : () => void handleDelete(open.flow)}
        />
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
      />
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
