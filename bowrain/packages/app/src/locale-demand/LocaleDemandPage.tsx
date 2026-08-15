import { useCallback, useMemo, useState } from "react";
import {
  AnalyticsEvents,
  Button,
  Dialog,
  DialogContent,
  DialogTitle,
  useAnalytics,
} from "@neokapi/ui";
import type { ApiAdapter, PostHogConnectorConfig } from "@neokapi/ui";
import { LocaleDemandView } from "./LocaleDemandView";
import { PostHogConnectCard } from "./PostHogConnectCard";
import { PostHogDemandSource, parsePostHogApiError } from "./posthog-demand-source";
import { sampleDemandDataSource } from "./locale-demand-fixtures";

/** The slice of the ApiAdapter this page needs — narrow so stories/tests can mock it. */
export type LocaleDemandApi = Pick<
  ApiAdapter,
  "getPostHogConnector" | "savePostHogConnector" | "getPostHogDemand"
>;

export interface LocaleDemandPageProps {
  api: LocaleDemandApi;
  workspaceSlug: string;
  /** Project the PostHog connector hangs off; undefined = no projects yet. */
  projectId?: string;
  /** Source + target locales of that project, for live coverage matching. */
  projectLocales?: string[];
}

type SourceMode =
  | { kind: "live"; source: PostHogDemandSource }
  | { kind: "sample"; degraded?: string };

/**
 * Locale demand page: starts optimistically on the live PostHog source; a
 * `not_configured` answer from the server drops it back to the sample
 * dataset, which the view labels as such. Connecting (or fixing a broken
 * connection) goes through the PostHogConnectCard in a dialog — the server
 * test-connects before saving, and on success the page switches straight to
 * live data.
 */
export function LocaleDemandPage({
  api,
  workspaceSlug,
  projectId,
  projectLocales = [],
}: LocaleDemandPageProps) {
  const [mode, setMode] = useState<SourceMode>(() =>
    projectId
      ? {
          kind: "live",
          source: new PostHogDemandSource(api, workspaceSlug, projectId, projectLocales),
        }
      : { kind: "sample" },
  );
  const [connectOpen, setConnectOpen] = useState(false);
  const [existingConfig, setExistingConfig] = useState<PostHogConnectorConfig | null>(null);
  const { capture } = useAnalytics();

  const handleSnapshotError = useCallback((err: unknown) => {
    const parsed = parsePostHogApiError(err);
    setMode({
      kind: "sample",
      degraded: parsed.code === "not_configured" ? undefined : parsed.message,
    });
  }, []);

  const openConnect = useCallback(async () => {
    // AD-018 demand path: connecting our own analytics is the meta-dogfood CTA.
    capture(AnalyticsEvents.localeDemandConnectClicked, {
      reconnect: mode.kind === "sample" && !!mode.degraded,
    });
    setConnectOpen(true);
    if (!projectId) return;
    try {
      // Prefill host/project + key mask when re-connecting. Best-effort: a
      // member without manage-connectors just gets an empty form.
      setExistingConfig(await api.getPostHogConnector(workspaceSlug, projectId));
    } catch {
      setExistingConfig(null);
    }
  }, [api, capture, mode, workspaceSlug, projectId]);

  const handleSaved = useCallback(() => {
    setConnectOpen(false);
    if (!projectId) return;
    // Fresh source instance so the view re-fetches immediately.
    setMode({
      kind: "live",
      source: new PostHogDemandSource(api, workspaceSlug, projectId, projectLocales),
    });
  }, [api, workspaceSlug, projectId, projectLocales]);

  // The connector hangs off a project, so a workspace without one gets the
  // sample-data label without the button.
  const connectAction = useMemo(() => {
    if (mode.kind !== "sample" || !projectId) return undefined;
    return (
      <Button size="sm" variant="outline" onClick={openConnect} data-testid="posthog-connect-open">
        {mode.degraded ? "Fix connection" : "Connect PostHog"}
      </Button>
    );
  }, [mode, projectId, openConnect]);

  return (
    <>
      <LocaleDemandView
        dataSource={mode.kind === "live" ? mode.source : sampleDemandDataSource}
        connectAction={connectAction}
        degradedReason={mode.kind === "sample" ? mode.degraded : undefined}
        onSnapshotError={handleSnapshotError}
      />
      <Dialog open={connectOpen} onOpenChange={setConnectOpen}>
        <DialogContent className="p-0 sm:max-w-md" showCloseButton={false}>
          <DialogTitle className="sr-only">Connect PostHog</DialogTitle>
          <PostHogConnectCard
            className="max-w-none border-0 shadow-none"
            config={existingConfig}
            onSave={(req) => {
              if (!projectId) return Promise.reject(new Error("no project"));
              return api.savePostHogConnector(workspaceSlug, projectId, req);
            }}
            onSaved={handleSaved}
            onCancel={() => setConnectOpen(false)}
          />
        </DialogContent>
      </Dialog>
    </>
  );
}
