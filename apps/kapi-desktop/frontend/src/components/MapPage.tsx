// The Map: the page a project tab opens on, drawn by @neokapi/project-screens'
// MapView. This page reads it through the desktop backend and fills the parts
// only the desktop has: the deeper look's row, the Connect your AI card, Add
// files and the file preview.
//
// The page renders from one read, MapSummary, which the backend answers from
// what it already holds. It reads again when the store sync finishes, when the
// files change and when the context changes. Findings come from the last
// checks run over the whole project: the page runs one the first time it opens
// a tab that has none, and again when the person asks. A file's Open leads to
// the Reader when the block cache holds blocks for it, and to the preview when
// it holds none.
//
// "Deeper look with AI" under "What your files do" reads the files with the
// connected model and files what it proposes as suggestions, which appear as
// rows when the run ends. With no model connected the button opens the
// Connect your AI card in its place.

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { Button, ErrorNotice, LoadingSpinner, ScrollArea } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { MapView, type ScreenTarget } from "@neokapi/project-screens";
import { api } from "../hooks/useApi";
import { ConnectAICard } from "./ConnectAICard";
import { qk } from "../lib/queryKeys";
import { desktopView } from "../lib/screenTargets";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useStoreSyncState } from "../hooks/useProjectStoreSync";
import { FilePreview } from "./FilePreview";
import type { DeeperLookModel, DeeperLookResult, MapFileRow, MapSummary } from "../types/api";

/** The tabs the Map has run a first check for in this session. */
const autoChecked = new Set<string>();

/** How long the deeper look's result line stays, in milliseconds. */
export const DEEPER_LOOK_RESULT_MS = 6000;

/** What the deeper look's row shows. */
export interface DeeperLookState {
  /** The model a run asks; null when none is connected. */
  model: string | null;
  /** Whether a model is connected is not known yet; the button waits. */
  pending?: boolean;
  /** A run is in progress. */
  running?: boolean;
  /** The run that just ended, shown for a few seconds. */
  result?: DeeperLookResult | null;
  error?: unknown;
}

/** Whether an error is the refusal for a project with no model to ask. */
function noModelError(error: unknown): boolean {
  return /no AI provider configured/.test(String((error as Error)?.message ?? error));
}

export interface MapPageProps {
  tabID: string;
  /** The project's name, the page title. */
  projectName: string;
  /** Pre-loaded summary for Storybook and tests, which reach no backend. */
  summary?: MapSummary;
  /** Pre-loaded deeper look model, for Storybook and tests. */
  deeperLookModel?: DeeperLookModel;
  /** Open the Decide view, on one row when a key is given. */
  onDecide: (rowKey?: string) => void;
  /** Go to another view of the project: "checks", "context:atlas", … */
  onNavigate: (view: string) => void;
  /**
   * Open a file the block cache has read in the Reader, by its
   * project-relative path. A file with no blocks opens in the preview.
   */
  onOpenReader?: (relative: string) => void;
  /**
   * The project waits to be set up: its folder is empty, or it matches a
   * preset. The files' empty state then leads to the page that sets it up.
   */
  setupPending?: boolean;
}

export function MapPage({
  tabID,
  projectName,
  summary: given,
  deeperLookModel: givenModel,
  onDecide,
  onNavigate,
  onOpenReader,
  setupPending,
}: MapPageProps) {
  const qc = useQueryClient();
  const key = qk.mapSummary(tabID);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.mapSummary(tabID),
    enabled: !given,
    placeholderData: (previous) => previous,
  });
  useInvalidateOnEvent(["project:extracted", "project-files-changed", "workspace:changed"], [key]);

  // The store sync records when it found the cache current, so the page reads
  // again each time one settles.
  const { syncing } = useStoreSyncState(tabID);
  const wasSyncing = useRef(syncing);
  useEffect(() => {
    if (wasSyncing.current && !syncing) void qc.invalidateQueries({ queryKey: key });
    wasSyncing.current = syncing;
  }, [syncing, qc, key]);

  const summary = given ?? query.data ?? null;

  const run = useMutation({
    mutationFn: () =>
      api.runChecks(tabID, {
        id: "",
        name: "",
        languages: summary?.target_languages ?? [],
      }),
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });

  // A tab that has not been checked in this session is checked once, as soon
  // as the files are read, so the findings count is there when the page is.
  useEffect(() => {
    if (given || !summary || syncing || run.isPending) return;
    if (summary.findings !== null || summary.files === 0 || autoChecked.has(tabID)) return;
    autoChecked.add(tabID);
    run.mutate();
  }, [given, summary, syncing, run, tabID]);

  const modelKey = qk.deeperLookModel(tabID);
  const modelQuery = useQuery({
    queryKey: modelKey,
    queryFn: () => api.deeperLookModel(tabID),
    enabled: !givenModel,
  });
  const lookModel = givenModel ?? modelQuery.data ?? null;
  const [connectOpen, setConnectOpen] = useState(false);
  const [lookResult, setLookResult] = useState<DeeperLookResult | null>(null);
  const look = useMutation({
    mutationFn: () => api.deeperLook(tabID),
    onMutate: () => setLookResult(null),
    onSuccess: (res) => {
      if (res) setLookResult(res);
    },
    onError: (error) => {
      if (noModelError(error)) {
        void qc.invalidateQueries({ queryKey: modelKey });
        setConnectOpen(true);
      }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });
  useEffect(() => {
    if (!lookResult) return;
    const id = setTimeout(() => setLookResult(null), DEEPER_LOOK_RESULT_MS);
    return () => clearTimeout(id);
  }, [lookResult]);

  const [preview, setPreview] = useState<MapFileRow | null>(null);
  const filesRef = useRef<HTMLElement>(null);

  // Decide and the Reader open through the tab's own handlers, which also
  // reset the hub's pin and remember the page the Reader returns to.
  const navigate = (target: ScreenTarget) => {
    if (target.kind === "rules") onDecide(target.row);
    else if (target.kind === "reader") onOpenReader?.(target.path);
    else onNavigate(desktopView(target));
  };
  const canNavigate = (target: ScreenTarget) => target.kind !== "reader" || !!onOpenReader;

  if (!summary) {
    return query.error ? (
      <div className="p-6">
        <ErrorNotice error={query.error} title={t("The project could not be read")} />
      </div>
    ) : (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    );
  }

  return (
    <ScrollArea className="h-full">
      <MapView
        projectName={projectName}
        summary={summary}
        running={run.isPending}
        runError={run.error}
        onRun={() => run.mutate()}
        deeperLook={
          <DeeperLookRow
            state={{
              model: lookModel?.connected ? (lookModel.model ?? lookModel.provider ?? "") : null,
              pending: lookModel === null,
              running: look.isPending,
              result: lookResult,
              error: look.error && !noModelError(look.error) ? look.error : undefined,
            }}
            disabled={summary.files === 0}
            onRun={() => {
              // Until the backend says whether a model is connected, a click has
              // nothing to run and nothing to offer.
              if (lookModel === null) return;
              if (!lookModel.connected) setConnectOpen((open) => !open);
              else look.mutate();
            }}
          />
        }
        connectAI={
          connectOpen &&
          lookModel &&
          !lookModel.connected && (
            <ConnectAICard
              description={t(
                "Pick the model kapi reads your files with. You can change it any time in Settings.",
              )}
              onConfigured={() => {
                setConnectOpen(false);
                void qc.invalidateQueries({ queryKey: ["ai"] });
              }}
            />
          )
        }
        navigate={navigate}
        canNavigate={canNavigate}
        setupPending={setupPending}
        onShowFiles={() => filesRef.current?.scrollIntoView({ behavior: "smooth" })}
        onPreviewFile={setPreview}
        addFiles={
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const added = await api.addFilesDialog(tabID, "");
              if (added && added.length > 0) void qc.invalidateQueries({ queryKey: key });
            }}
          >
            Add files
          </Button>
        }
        filePreview={
          <FilePreview
            tabID={tabID}
            filePath={preview?.path ?? null}
            filename={preview?.relative ?? ""}
            onClose={() => setPreview(null)}
          />
        }
        filesRef={filesRef}
      />
    </ScrollArea>
  );
}

/**
 * The deeper look's button and its one line: what it does, the model it is
 * reading with while it runs, and what it filed for a few seconds after.
 */
export function DeeperLookRow({
  state,
  disabled,
  onRun,
}: {
  state: DeeperLookState;
  disabled?: boolean;
  onRun: () => void;
}) {
  const { model, pending, running, result, error } = state;
  let line: React.ReactNode = t("Reads your files with the connected model and suggests rules.");
  if (running) line = null;
  else if (result && result.filed > 0) {
    line = t(
      "{count, plural, one {# new suggestion} other {# new suggestions}} from {model}",
      "after the deeper look on the Map",
      { count: result.filed, model: result.model },
    );
  } else if (result) line = t("Nothing more to suggest");
  return (
    <div className="space-y-2 pt-2">
      <div data-slot="map-deeper-look" className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <Button
          size="sm"
          variant="outline"
          data-slot="map-deeper-look-run"
          data-connected={model !== null}
          disabled={pending || running || disabled}
          onClick={onRun}
        >
          {running ? <LoadingSpinner size="sm" /> : <Sparkles size={14} />}
          {running ? (
            t("Reading with {model}", "the deeper look's button while it runs", {
              model: model ?? "",
            })
          ) : (
            <>Deeper look with AI</>
          )}
        </Button>
        {line && (
          <span
            data-slot="map-deeper-look-line"
            data-result={result ? "done" : undefined}
            aria-live="polite"
            className="text-xs text-muted-foreground"
          >
            {line}
          </span>
        )}
      </div>
      {!!error && <ErrorNotice error={error} title={t("The deeper look did not run")} />}
    </div>
  );
}
