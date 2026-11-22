// A project's settings: where its content sits, and what runs on it.
//
// The voice governing the project is the shared VoiceBindingSelect over the
// workspace's profiles, the same form kapi desktop binds a recipe's voice
// with. Streams and collections override it in their own dialogs.

import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useApi,
  useStream,
  Card,
  CardContent,
  Button,
  PageContainer,
  PageHeader,
  SectionHeading,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  SettingsRow,
  Switch,
  VoiceBindingSelect,
  voiceProfileOptions,
} from "@neokapi/ui";
import { Compass, Workflow } from "lucide-react";
import type { WorkspaceRouteContext } from "..";
import {
  invalidateProjectQueries,
  projectQueryOptions,
  voiceProfilesQueryOptions,
} from "../../queries";
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
    invalidateProjectQueries(queryClient, ws, project.id);
  }, [queryClient, ws, project.id]);

  const setProperty = async (key: string, value: string) => {
    await adapter.updateProject(ws, project.id, { properties: { [key]: value } });
    invalidateProject();
  };

  const workflowEnabled = project.properties?.workflow_enabled !== "false";

  return (
    <PageContainer width="standard" className="space-y-6">
      <PageHeader
        title="Project settings"
        subtitle={project.name}
        className="mb-0"
        backButton={
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
        }
      />

      {/* Where content sits: the voice governing the project's point. */}
      <section>
        <SectionHeading className="mb-3" icon={<Compass size={14} />}>
          Where content sits
        </SectionHeading>
        <Card>
          <CardContent className="p-4">
            <VoiceBindingSelect
              value={project.properties?.voice_profile_id || undefined}
              options={voiceProfileOptions(voiceProfiles)}
              inheritLabel="Workspace default"
              help="Governs checks and scoring for this project's content. Streams and collections can override it."
              onChange={(next) => void setProperty("voice_profile_id", next ?? "")}
            />
          </CardContent>
        </Card>
      </section>

      {/* What runs, and what is skipped: the translator workflow and its gate. */}
      <section>
        <SectionHeading className="mb-3" icon={<Workflow size={14} />}>
          What runs, and what is skipped
        </SectionHeading>
        <Card>
          <CardContent className="p-4">
            <SettingsRow
              label="Translator workflow"
              help="Create review tasks after AI translation completes. On by default; turn off to skip human review."
            >
              <Switch
                checked={workflowEnabled}
                onCheckedChange={(checked) =>
                  void setProperty("workflow_enabled", checked ? "true" : "false")
                }
                aria-label="Enable translator workflow"
              />
            </SettingsRow>
            {workflowEnabled && (
              <div className="border-t border-border/50">
                <SettingsRow
                  label="Mode"
                  help="Review: translators verify AI translations. Translate: translators work from source."
                >
                  <Select
                    value={project.properties?.workflow_mode ?? "review"}
                    onValueChange={(v) => void setProperty("workflow_mode", v)}
                  >
                    <SelectTrigger className="w-32" aria-label="Workflow mode">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="review">Review</SelectItem>
                      <SelectItem value="translate">Translate</SelectItem>
                    </SelectContent>
                  </Select>
                </SettingsRow>
                <SettingsRow
                  label="Source review gate"
                  help="Require source review before translation fan-out."
                >
                  <Switch
                    checked={project.properties?.workflow_source_review === "true"}
                    onCheckedChange={(checked) =>
                      void setProperty("workflow_source_review", checked ? "true" : "false")
                    }
                    aria-label="Enable source review gate"
                  />
                </SettingsRow>
              </div>
            )}
          </CardContent>
        </Card>
      </section>

      <ModelQualityCard ws={ws} projectId={project.id} />
    </PageContainer>
  );
}
