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
import { useToolSchemas } from "@neokapi/flow-editor";
import { ErrorNotice, useApi } from "@neokapi/ui";
import type { FlowDefinitionInfo, LinearFlowSpec } from "@neokapi/ui";
import { definitionToSpec, specToDefinition, toEditorDefinition, toEditorTools } from "./flowGraph";
import { ProjectFlowList } from "./ProjectFlowList";
import { ProjectFlowPane, type FlowSaveState } from "./ProjectFlowPane";
import { useSchemaFormHost } from "./useSchemaFormHost";

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
          void invalidate();
          if (!current()) return;
          const now = openRef.current!;
          setOpen({
            ...now,
            flow: { ...now.flow, created_at: saved.created_at, modified_at: saved.modified_at },
          });
          setSaveError(null);
          setSaveState(dirtyRef.current ? "unsaved" : "saved");
        } catch (e) {
          if (!current()) return;
          dirtyRef.current = true;
          setSaveError(e);
          setSaveState("error");
        }
      });
    },
    [api, workspaceSlug, projectId, invalidate, setOpen],
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
      setOpen({ flow: cur.flow, spec });
      dirtyRef.current = true;
      setSaveState("unsaved");
      clearTimeout(timerRef.current);
      timerRef.current = setTimeout(flush, saveDelayMs);
    },
    [setOpen, flush, saveDelayMs],
  );

  const openFlow = useCallback(
    (flow: FlowDefinitionInfo) => {
      flush();
      setOpen({ flow, spec: definitionToSpec(flow) });
      setSaveState("saved");
      setSaveError(null);
      setActionError(null);
    },
    [flush, setOpen],
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
        if (openRef.current?.flow.id === flow.id) setOpen(null);
        setActionError(null);
        void invalidate();
      } catch (e) {
        setActionError(e);
      }
    },
    [api, workspaceSlug, projectId, invalidate, setOpen],
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
