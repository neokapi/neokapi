import { useCallback, useMemo, useRef, useReducer } from "react";
import {
  FlowsWorkspace,
  type FlowsDataAdapter,
  type ToolInfo as EditorToolInfo,
  type FlowDefinitionInfo as EditorFlowDefinitionInfo,
  type ComponentSchema,
} from "@neokapi/flow-editor";
import { useFlowDefinitions, useFlowDefinitionApi, useTools } from "../hooks/useApi";

// Wails v3 bindings — used directly for the optional tool-schema lookup, which
// the FlowEditor requests synchronously and we resolve via a cache.
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js";
import { optionalBinding } from "../api/optionalBinding";

/**
 * Visual flow builder for the Bowrain desktop app.
 *
 * The list / new / save / delete UX and the editing canvas are the shared
 * `@neokapi/flow-editor` <FlowsWorkspace>, the same container kapi-desktop and
 * the Bowrain web app render. This component is just the Wails data adapter: the
 * underlying calls (List/Save/DeleteFlowDefinition) are project-scoped and proxy
 * to the Bowrain server's flow-definition REST API (#766), and the tool-schema
 * lookup resolves through a cache over the optional GetToolSchema binding.
 *
 * Flows are connector-agnostic, project-scoped server resources. A project must
 * be selected to author flows; without one, only the built-in catalog shows.
 */
export function FlowBuilder({ projectId }: { projectId?: string }) {
  const { definitions, refresh } = useFlowDefinitions(projectId ?? "");
  const { saveFlowDefinition, deleteFlowDefinition } = useFlowDefinitionApi(projectId ?? "");
  const { tools } = useTools();

  // Map bowrain's ToolInfo (snake_case is_source_transform) onto the editor's
  // ToolInfo (camelCase isSourceTransform), threading the full IO metadata.
  const editorTools = useMemo<EditorToolInfo[]>(
    () =>
      tools.map((t) => ({
        name: t.name,
        description: t.description,
        category: t.category,
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

  // Tool-schema cache. The backend may not expose GetToolSchema yet (the
  // generated bindings omit it); onGetSchema returns null until it does.
  const schemasRef = useRef<Record<string, ComponentSchema | null>>({});
  const fetchingRef = useRef<Set<string>>(new Set());
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);

  const handleGetSchema = useCallback((toolName: string): ComponentSchema | null => {
    if (toolName in schemasRef.current) {
      return schemasRef.current[toolName] ?? null;
    }
    if (fetchingRef.current.has(toolName)) return null;
    const fn = optionalBinding<(name: string) => Promise<ComponentSchema | null>>(
      Backend,
      "GetToolSchema",
    );
    if (!fn) {
      schemasRef.current[toolName] = null;
      return null;
    }
    fetchingRef.current.add(toolName);
    void fn(toolName)
      .then((result) => {
        schemasRef.current[toolName] = result ?? null;
      })
      .catch(() => {
        schemasRef.current[toolName] = null;
      })
      .finally(() => {
        fetchingRef.current.delete(toolName);
        forceUpdate();
      });
    return null;
  }, []);

  const adapter = useMemo<FlowsDataAdapter>(
    () => ({
      flows: definitions as EditorFlowDefinitionInfo[],
      saveFlow: async (def) => {
        const saved = await saveFlowDefinition(def);
        refresh();
        return saved as EditorFlowDefinitionInfo;
      },
      deleteFlow: async (id) => {
        await deleteFlowDefinition(id);
        refresh();
      },
    }),
    [definitions, saveFlowDefinition, deleteFlowDefinition, refresh],
  );

  return (
    <FlowsWorkspace
      tools={editorTools}
      adapter={adapter}
      canAuthor={!!projectId}
      makeFlowId={() => `custom-flow-${Date.now()}`}
      showDescriptionInput
      onGetSchema={handleGetSchema}
    />
  );
}
