import { useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { PageContainer, ProjectFeedView, useApi, useWorkspace } from "@neokapi/ui";
import type { FeedEntry, FeedLink } from "@neokapi/ui";
import { projectQueryOptions } from "../../queries";

/** The screens of the feed's links this app opens. */
const OPENS: ReadonlySet<FeedLink["screen"]> = new Set([
  "stream",
  "run",
  "review_deck",
  "change_set",
  "language",
]);

/**
 * A project's feed: how the project changed, newest first. List and detail at
 * full width. Each entry's link names a screen, and this route maps it to the
 * app's own: a push to the stream's languages, a run to Runs, a review summary
 * to the review session, a change set to its page under Context, a coverage
 * milestone to the languages.
 */
export function ProjectFeedRoute() {
  const { projectId = "", stream = "main" } = useParams({ strict: false });
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const navigate = useNavigate();
  const ws = activeWorkspace?.slug ?? "";

  const project = useQuery({
    ...projectQueryOptions(api, ws, projectId, stream),
    enabled: !!ws && !!projectId,
  });

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Activity · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const open = useCallback(
    (entry: FeedEntry) => {
      const params = { workspace: ws, projectId, stream: entry.link.stream || stream };
      switch (entry.link.screen) {
        case "stream":
        case "language":
          void navigate({ to: "/$workspace/p/$projectId/s/$stream/languages", params });
          return;
        case "run":
          void navigate({ to: "/$workspace/p/$projectId/s/$stream/runs", params });
          return;
        case "review_deck":
          void navigate({ to: "/$workspace/p/$projectId/s/$stream/review", params });
          return;
        case "change_set":
          if (entry.link.id) {
            void navigate({
              to: "/$workspace/context/changes/$id",
              params: { workspace: ws, id: entry.link.id },
            });
          }
          return;
      }
    },
    [navigate, ws, projectId, stream],
  );

  return (
    <PageContainer width="wide">
      <ProjectFeedView
        workspace={ws}
        projectId={projectId}
        languages={project.data?.target_languages ?? []}
        onOpenLink={open}
        onOpenLanguage={() =>
          void navigate({
            to: "/$workspace/p/$projectId/s/$stream/review",
            params: { workspace: ws, projectId, stream },
          })
        }
        canOpen={(link) => OPENS.has(link.screen) && (link.screen !== "change_set" || !!link.id)}
      />
    </PageContainer>
  );
}
