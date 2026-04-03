import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery, useQueryClient } from "@tanstack/react-query";
import {
  useApi,
  useStream,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Switch,
} from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";
import { projectQueryOptions } from "../../queries";

export function ProjectSettingsRoute() {
  const navigate = useNavigate();
  const { workspace, projectId } = useParams({ strict: false });
  const adapter = useApi();
  const queryClient = useQueryClient();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream } = useStream();

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, activeStream),
  );

  useEffect(() => {
    document.title = `Settings — ${project.name} — ${activeWorkspace.name} — Bowrain`;
  }, [project.name, activeWorkspace.name]);

  const invalidateProject = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["project", ws, project.id] });
  }, [queryClient, ws, project.id]);

  const handleTogglePulseVisibility = useCallback(async () => {
    const newVis = project.dashboard_visibility === "public" ? "private" : "public";
    queryClient.setQueryData(
      ["project", ws, project.id, activeStream],
      (old: typeof project | undefined) => (old ? { ...old, dashboard_visibility: newVis } : old),
    );
    await adapter.updateProject(ws, project.id, {
      dashboard_visibility: newVis,
    });
    invalidateProject();
  }, [
    ws,
    adapter,
    project.id,
    project.dashboard_visibility,
    queryClient,
    activeStream,
    invalidateProject,
  ]);

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6 py-4">
      <div className="flex items-center gap-3 mb-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={() =>
            navigate({
              to: "/$workspace/p/$projectId/s/$stream",
              params: {
                workspace: workspace ?? ws,
                projectId: project.id,
                stream: activeStream,
              },
            })
          }
        >
          Back to project
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Translator Workflow</CardTitle>
          <CardDescription>
            Automatically create tasks for translators when content is ready
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Enable workflow</p>
              <p className="text-xs text-muted-foreground">
                Create review tasks after AI translation completes
              </p>
            </div>
            <Switch
              checked={project.properties?.workflow_enabled === "true"}
              onCheckedChange={async (checked) => {
                await adapter.updateProject(ws, project.id, {
                  properties: { workflow_enabled: checked ? "true" : "false" },
                });
                invalidateProject();
              }}
              aria-label="Enable translator workflow"
            />
          </div>
          {project.properties?.workflow_enabled === "true" && (
            <div className="space-y-3 pt-2 border-t border-border/50">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium">Mode</p>
                  <p className="text-xs text-muted-foreground">
                    Review: translators verify AI translations. Translate: translators work from
                    source.
                  </p>
                </div>
                <select
                  className="h-8 rounded-md border border-input bg-background px-2 text-sm"
                  value={project.properties?.workflow_mode ?? "review"}
                  onChange={async (e) => {
                    await adapter.updateProject(ws, project.id, {
                      properties: { workflow_mode: e.target.value },
                    });
                    invalidateProject();
                  }}
                >
                  <option value="review">Review</option>
                  <option value="translate">Translate</option>
                </select>
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium">Source review gate</p>
                  <p className="text-xs text-muted-foreground">
                    Require source review before translation fan-out
                  </p>
                </div>
                <Switch
                  checked={project.properties?.workflow_source_review === "true"}
                  onCheckedChange={async (checked) => {
                    await adapter.updateProject(ws, project.id, {
                      properties: { workflow_source_review: checked ? "true" : "false" },
                    });
                    invalidateProject();
                  }}
                  aria-label="Enable source review gate"
                />
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Pulse Dashboard</CardTitle>
          <CardDescription>
            Control whether this project appears on the public Pulse dashboard
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Show on Pulse</p>
              <p className="text-xs text-muted-foreground">
                Make this project visible on the public dashboard
              </p>
            </div>
            <Switch
              checked={project.dashboard_visibility === "public"}
              onCheckedChange={handleTogglePulseVisibility}
              aria-label="Toggle Pulse visibility"
            />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
