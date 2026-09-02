import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery, useQuery, useQueryClient } from "@tanstack/react-query";
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
import { projectQueryOptions, voiceProfilesQueryOptions } from "../../queries";
import { ModelQualityCard } from "./model-quality-card";

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

  const { data: voiceProfiles } = useQuery(voiceProfilesQueryOptions(adapter, ws));

  useEffect(() => {
    document.title = `Settings · ${project.name} · ${activeWorkspace.name} · Bowrain`;
  }, [project.name, activeWorkspace.name]);

  const invalidateProject = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["project", ws, project.id] });
  }, [queryClient, ws, project.id]);

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
                Create review tasks after AI translation completes. On by default; turn off to skip
                human review.
              </p>
            </div>
            <Switch
              checked={project.properties?.workflow_enabled !== "false"}
              onCheckedChange={async (checked) => {
                await adapter.updateProject(ws, project.id, {
                  properties: { workflow_enabled: checked ? "true" : "false" },
                });
                invalidateProject();
              }}
              aria-label="Enable translator workflow"
            />
          </div>
          {project.properties?.workflow_enabled !== "false" && (
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
          <CardTitle>Voice</CardTitle>
          <CardDescription>
            Choose the voice profile that governs checks and scoring for this project. Leave as the
            workspace default to inherit; streams and collections can override it.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Profile</p>
              <p className="text-xs text-muted-foreground">
                Applied when scoring or rewriting this project's content
              </p>
            </div>
            <select
              className="h-8 rounded-md border border-input bg-background px-2 text-sm"
              value={project.properties?.voice_profile_id ?? ""}
              onChange={async (e) => {
                await adapter.updateProject(ws, project.id, {
                  properties: { voice_profile_id: e.target.value },
                });
                invalidateProject();
              }}
              aria-label="Project voice profile"
            >
              <option value="">Workspace default</option>
              {voiceProfiles?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
        </CardContent>
      </Card>

      <ModelQualityCard ws={ws} projectId={project.id} />
    </div>
  );
}
