import { useCallback, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi, type FlowDefinitionInfo } from "@neokapi/ui";
import {
  FlowsWorkspace,
  type FlowsDataAdapter,
  type ToolInfo as EditorToolInfo,
  type FlowDefinitionInfo as EditorFlowDefinitionInfo,
} from "@neokapi/flow-editor";

/**
 * Web superset flow editor.
 *
 * Lists a project's flow definitions (built-in flows merged with server-stored
 * project flows) and edits them on the shared `@neokapi/flow-editor`
 * <FlowsWorkspace> — the same list / new / save / delete container and canonical
 * <FlowEditor> canvas that kapi-desktop and the Bowrain desktop app render. This
 * component is now just the REST data adapter: it maps the server tool list to
 * the editor's ToolInfo and bridges the flow-definition REST calls.
 *
 * Flows are connector-agnostic: they apply to content from any connector. The
 * flow graph never names a connector (Kapi is one source among many).
 */
export function ProjectFlowsEditor({
  workspaceSlug,
  projectId,
}: {
  workspaceSlug: string;
  projectId: string;
}) {
  const api = useApi();
  const queryClient = useQueryClient();
  const flowsKey = useMemo(() => ["flows", workspaceSlug, projectId], [workspaceSlug, projectId]);

  const { data: flows, isLoading } = useQuery({
    queryKey: flowsKey,
    queryFn: () => api.listFlowDefinitions(workspaceSlug, projectId),
    staleTime: 15_000,
  });

  const { data: tools } = useQuery({
    queryKey: ["tools"],
    queryFn: () => api.listTools(),
    staleTime: 5 * 60_000,
  });

  const editorTools = useMemo<EditorToolInfo[]>(
    () =>
      (tools ?? []).map((t) => ({
        name: t.name,
        display_name: t.display_name,
        description: t.description,
        category: t.category,
        source: t.source,
        tags: t.tags,
        requires: t.requires,
        cardinality: t.cardinality as EditorToolInfo["cardinality"],
        default_locale: t.default_locale,
        side_effects: t.side_effects,
        consumes: t.consumes,
        produces: t.produces,
        isSourceTransform: t.is_source_transform,
      })),
    [tools],
  );

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: flowsKey }),
    [queryClient, flowsKey],
  );

  const createMutation = useMutation({
    mutationFn: (def: FlowDefinitionInfo) =>
      api.createFlowDefinition(workspaceSlug, projectId, def),
    onSuccess: () => void invalidate(),
  });

  const updateMutation = useMutation({
    mutationFn: ({ flowId, def }: { flowId: string; def: FlowDefinitionInfo }) =>
      api.updateFlowDefinition(workspaceSlug, projectId, flowId, def),
    onSuccess: () => void invalidate(),
  });

  const deleteMutation = useMutation({
    mutationFn: (flowId: string) => api.deleteFlowDefinition(workspaceSlug, projectId, flowId),
    onSuccess: () => void invalidate(),
  });

  const adapter = useMemo<FlowsDataAdapter>(
    () => ({
      flows: (flows ?? []) as EditorFlowDefinitionInfo[],
      isLoading,
      saveFlow: async (def, isNew) => {
        const uiDef = def as FlowDefinitionInfo;
        const saved = isNew
          ? await createMutation.mutateAsync(uiDef)
          : await updateMutation.mutateAsync({ flowId: uiDef.id, def: uiDef });
        return saved as EditorFlowDefinitionInfo;
      },
      deleteFlow: (id) => deleteMutation.mutateAsync(id).then(() => undefined),
    }),
    [flows, isLoading, createMutation, updateMutation, deleteMutation],
  );

  return (
    <FlowsWorkspace
      tools={editorTools}
      adapter={adapter}
      newFlowSource="project"
      confirmDelete
      emptyStateText="Select a flow or create a new one. Flows run server-side on content from any connector."
      className="h-[640px] rounded-lg border border-border bg-card shadow-sm"
    />
  );
}
