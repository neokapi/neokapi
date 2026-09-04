// The project's flows over the flow-definition REST API: a list of the
// built-in flows merged with the project's own, and one flow open in the
// shared linear editor. Edits save on their own after a short pause, and the
// pane says where the flow stands against the server.
//
// Flows are connector-agnostic: they apply to content from any connector, and
// the flow graph never names one.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import { ErrorNotice, useApi } from "@neokapi/ui";
import type { FlowDefinitionInfo, LinearFlowSpec } from "@neokapi/ui";
import { definitionToSpec, specToDefinition, toEditorDefinition } from "./flowGraph";
import { ProjectFlowList } from "./ProjectFlowList";
import { ProjectFlowPane, type FlowSaveState } from "./ProjectFlowPane";

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
  const tools = toolsQuery.data ?? [];

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: flowsKey }),
    [queryClient, flowsKey],
  );

  const [open, setOpen] = useState<OpenFlow | null>(null);
  const [saveState, setSaveState] = useState<FlowSaveState>("saved");
  const [saveError, setSaveError] = useState<unknown>(null);
  const [actionError, setActionError] = useState<unknown>(null);

  // The edit waiting to be saved, and its timer. A ref rather than state: the
  // save runs from a timer or from unmount, neither of which sees a render.
  const pendingRef = useRef<OpenFlow | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const persist = useCallback(
    async (edit: OpenFlow) => {
      setSaveState("saving");
      try {
        const saved = await api.updateFlowDefinition(
          workspaceSlug,
          projectId,
          edit.flow.id,
          specToDefinition(edit.spec, edit.flow),
        );
        setOpen((cur) => (cur && cur.flow.id === saved.id ? { ...cur, flow: saved } : cur));
        setSaveError(null);
        setSaveState(pendingRef.current ? "unsaved" : "saved");
        void invalidate();
      } catch (e) {
        setSaveError(e);
        setSaveState("error");
      }
    },
    [api, workspaceSlug, projectId, invalidate],
  );

  /** Saves the waiting edit now rather than after the pause. */
  const flush = useCallback(() => {
    clearTimeout(timerRef.current);
    const edit = pendingRef.current;
    pendingRef.current = null;
    if (edit) void persist(edit);
  }, [persist]);

  // A pause that outlives the surface still saves: leaving the page must not
  // drop the last edit.
  useEffect(() => flush, [flush]);

  const handleChange = useCallback(
    (spec: LinearFlowSpec) => {
      setOpen((cur) => {
        if (!cur || cur.flow.source === "built-in") return cur;
        const next = { flow: cur.flow, spec };
        pendingRef.current = next;
        return next;
      });
      setSaveState("unsaved");
      clearTimeout(timerRef.current);
      timerRef.current = setTimeout(flush, saveDelayMs);
    },
    [flush, saveDelayMs],
  );

  const openFlow = useCallback(
    (flow: FlowDefinitionInfo) => {
      flush();
      setOpen({ flow, spec: definitionToSpec(flow) });
      setSaveState("saved");
      setSaveError(null);
      setActionError(null);
    },
    [flush],
  );

  const handleBack = useCallback(() => {
    flush();
    setOpen(null);
  }, [flush]);

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

  const handleRename = useCallback(
    async (name: string) => {
      const cur = open;
      if (!cur) return;
      flush();
      setSaveState("saving");
      try {
        const saved = await api.updateFlowDefinition(
          workspaceSlug,
          projectId,
          cur.flow.id,
          specToDefinition(cur.spec, { ...cur.flow, name }),
        );
        setOpen((now) => (now && now.flow.id === saved.id ? { ...now, flow: saved } : now));
        setSaveError(null);
        setSaveState("saved");
        void invalidate();
      } catch (e) {
        setSaveError(e);
        setSaveState("error");
      }
    },
    [open, flush, api, workspaceSlug, projectId, invalidate],
  );

  const handleDelete = useCallback(
    async (flow: FlowDefinitionInfo) => {
      if (pendingRef.current?.flow.id === flow.id) {
        clearTimeout(timerRef.current);
        pendingRef.current = null;
      }
      try {
        await api.deleteFlowDefinition(workspaceSlug, projectId, flow.id);
        setOpen((cur) => (cur?.flow.id === flow.id ? null : cur));
        setActionError(null);
        void invalidate();
      } catch (e) {
        setActionError(e);
      }
    },
    [api, workspaceSlug, projectId, invalidate],
  );

  if (open) {
    const readOnly = open.flow.source === "built-in";
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
          saveState={saveState}
          saveError={saveError}
          onBack={handleBack}
          onChange={handleChange}
          onRename={(name) => void handleRename(name)}
          onCopy={readOnly ? () => void handleCopy(open.flow) : undefined}
          onDelete={readOnly ? undefined : () => void handleDelete(open.flow)}
        />
      </>
    );
  }

  return (
    <ProjectFlowList
      flows={flows}
      tools={tools}
      loading={flowsQuery.isLoading}
      error={flowsQuery.error ?? actionError}
      onOpen={openFlow}
      onCreate={handleCreate}
      onCopy={(flow) => void handleCopy(flow)}
      onDelete={(flow) => void handleDelete(flow)}
    />
  );
}
