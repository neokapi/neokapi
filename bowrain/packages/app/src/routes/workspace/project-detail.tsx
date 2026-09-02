import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import { useSuspenseQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Button,
  ProjectView,
  useApi,
  useStream,
  StreamCreateDialog,
  StreamEditDialog,
  StreamMergeDialog,
  ProjectFormDialog,
  StreamDiffView,
  CreateCollectionDialog,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  useStreamActions,
  ConfirmDialog,
  ProjectMemberManager,
  X,
} from "@neokapi/ui";
import type {
  SkippedFile,
  StreamVisibility,
  StreamMergeResult,
  StreamDiffResult,
  StreamInfo,
} from "@neokapi/ui";
import { projectDetailQueryOptions, voiceProfilesQueryOptions } from "../../queries";
import { usePlatform } from "../../platform";
import type { WorkspaceRouteContext } from "..";

/**
 * The project's source content: the collections it is organised into, the items
 * inside them, its streams, and the project-level actions. This is the one
 * surface that reads the project's embedded item array — every other project
 * route reads the summary, and the overview serves the item list paged.
 */
export function ProjectDetailRoute() {
  const navigate = useNavigate();
  const { workspace, projectId } = useParams({ strict: false });
  const adapter = useApi();
  const platform = usePlatform();
  const queryClient = useQueryClient();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream, setActiveStream } = useStream();

  const { data: project } = useSuspenseQuery(
    projectDetailQueryOptions(adapter, ws, projectId!, activeStream),
  );

  // Which collection is being read, and which item's preview is open. Search
  // params rather than component state, so the reading survives a reload, can
  // be linked to, and Back returns to the one before. Each setter carries the
  // other parameter forward — closing a preview must not also throw the reader
  // back to the first collection.
  const search = useSearch({ strict: false }) as { preview?: string; collection?: string };
  const setSourceSearch = (next: { preview?: string; collection?: string }) =>
    void navigate({
      to: "/$workspace/p/$projectId/s/$stream/source",
      params: { workspace: workspace ?? ws, projectId: project.id, stream: activeStream },
      search: (prev: Record<string, unknown>) => ({ ...prev, ...next }),
    });
  const setPreview = (itemName: string | undefined) => setSourceSearch({ preview: itemName });

  // The three item surfaces share a shape: the item name travels as the splat.
  const openItemIn =
    (
      to:
        | "/$workspace/p/$projectId/s/$stream/translate/$"
        | "/$workspace/p/$projectId/s/$stream/review/$"
        | "/$workspace/p/$projectId/s/$stream/pre-process/$",
    ) =>
    (itemName: string) =>
      void navigate({
        to,
        params: {
          workspace: workspace ?? ws,
          projectId: project.id,
          stream: activeStream,
          _splat: itemName,
        },
      });

  // Workspace voice profiles feed the per-collection / per-stream voice
  // pickers; the dialogs render the control only when profiles exist.
  const { data: voiceProfiles } = useQuery(voiceProfilesQueryOptions(adapter, ws));

  useEffect(() => {
    document.title = `${project.name} · ${activeWorkspace.name} · Bowrain`;
  }, [project.name, activeWorkspace.name]);

  // ── File handlers ────────────────────────────────────────────────────

  const invalidateProject = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["project", ws, project.id] });
  }, [queryClient, ws, project.id]);

  // Files the server declined to import on the last upload (and why).
  const [uploadSkipped, setUploadSkipped] = useState<SkippedFile[]>([]);

  // Why the last file/collection/stream mutation failed. Dialogs and the
  // ProjectView fire these handlers fire-and-forget, so a throw is caught here
  // and surfaced in the strip rather than lost as an unhandled rejection.
  const [actionError, setActionError] = useState<string | null>(null);
  const runAction = useCallback(async (fn: () => Promise<void>): Promise<void> => {
    setActionError(null);
    try {
      await fn();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  const handleUploadFiles = useCallback(
    (files: File[]) =>
      runAction(async () => {
        const result = await adapter.uploadFiles(ws, project.id, files, activeStream);
        setUploadSkipped(result.skipped ?? []);
        invalidateProject();
      }),
    [ws, adapter, project.id, activeStream, invalidateProject, runAction],
  );

  const handleRemoveFile = useCallback(
    (fileName: string) =>
      runAction(async () => {
        await adapter.removeFile(ws, project.id, fileName, activeStream);
        invalidateProject();
      }),
    [ws, adapter, project.id, activeStream, invalidateProject, runAction],
  );

  // ── Project actions ────────────────────────────────────────────────

  const [showEditProject, setShowEditProject] = useState(false);
  const [showMembers, setShowMembers] = useState(false);

  const handleEditProjectSubmit = useCallback(
    async (data: { name: string; default_source_language: string; target_languages: string[] }) => {
      await adapter.updateProject(ws, project.id, {
        name: data.name,
        target_languages: data.target_languages,
      });
      setShowEditProject(false);
      invalidateProject();
    },
    [ws, adapter, project.id, invalidateProject],
  );

  const [showArchiveProject, setShowArchiveProject] = useState(false);
  const confirmArchiveProject = useCallback(async () => {
    await adapter.deleteProject(ws, project.id);
    setShowArchiveProject(false);
    void queryClient.invalidateQueries({ queryKey: ["projects", ws] });
    void navigate({ to: "/$workspace", params: { workspace: workspace ?? ws } });
  }, [ws, adapter, project.id, queryClient, navigate, workspace]);

  // ── Collection handlers ──────────────────────────────────────────────

  const [showCollectionDialog, setShowCollectionDialog] = useState(false);
  const [editingCollection, setEditingCollection] = useState<
    import("@neokapi/ui").CollectionInfo | undefined
  >(undefined);

  const handleCreateCollection = useCallback(
    (data: {
      name: string;
      kind: "uploaded" | "connected";
      item_label: string;
      connector_config?: Record<string, string>;
    }) =>
      runAction(async () => {
        if (editingCollection) {
          // Edit mode — update existing collection
          await adapter.updateCollection(ws, project.id, editingCollection.id, data);
        } else {
          // Create mode
          await adapter.createCollection(ws, project.id, data);
        }
        setShowCollectionDialog(false);
        setEditingCollection(undefined);
        invalidateProject();
      }),
    [ws, adapter, project.id, editingCollection, invalidateProject, runAction],
  );

  const handleEditCollection = useCallback((collection: import("@neokapi/ui").CollectionInfo) => {
    setEditingCollection(collection);
    setShowCollectionDialog(true);
  }, []);

  const [deleteCollectionId, setDeleteCollectionId] = useState<string | null>(null);
  const confirmDeleteCollection = useCallback(
    () =>
      runAction(async () => {
        if (!deleteCollectionId) return;
        await adapter.deleteCollection(ws, project.id, deleteCollectionId);
        setDeleteCollectionId(null);
        invalidateProject();
      }),
    [ws, adapter, project.id, deleteCollectionId, invalidateProject, runAction],
  );

  const handleUploadToCollection = useCallback(
    (collectionId: string, files: File[]) =>
      runAction(async () => {
        const result = await adapter.uploadToCollection(
          ws,
          project.id,
          collectionId,
          files,
          activeStream,
        );
        setUploadSkipped(result.skipped ?? []);
        invalidateProject();
      }),
    [ws, adapter, project.id, activeStream, invalidateProject, runAction],
  );

  // ── Stream handlers ──────────────────────────────────────────────────

  const [showCreateStream, setShowCreateStream] = useState(false);
  const [editingStream, setEditingStream] = useState<StreamInfo | null>(null);
  const [mergeResult, setMergeResult] = useState<{
    result: StreamMergeResult;
    streamName: string;
    parentName: string;
  } | null>(null);
  const [diffResult, setDiffResult] = useState<StreamDiffResult | null>(null);

  const handleCreateStream = useCallback(
    (data: {
      name: string;
      parent: string;
      visibility: StreamVisibility;
      description: string;
      properties?: Record<string, string>;
    }) =>
      runAction(async () => {
        await adapter.createStream(ws, project.id, data);
        setShowCreateStream(false);
        setActiveStream(data.name);
        invalidateProject();
      }),
    [ws, adapter, project.id, setActiveStream, invalidateProject, runAction],
  );

  const handleEditStream = useCallback((stream: StreamInfo) => {
    setEditingStream(stream);
  }, []);

  const handleEditStreamSubmit = useCallback(
    (data: {
      description: string;
      visibility: StreamVisibility;
      properties?: Record<string, string>;
    }) =>
      runAction(async () => {
        if (!editingStream) return;
        await adapter.updateStream(ws, project.id, editingStream.name, data);
        setEditingStream(null);
        invalidateProject();
      }),
    [ws, adapter, project.id, editingStream, invalidateProject, runAction],
  );

  const handleMergeStream = useCallback(
    (streamName: string) =>
      runAction(async () => {
        const stream = project.streams?.find((s) => s.name === streamName);
        if (!stream) return;
        // Dry run first
        const result = await adapter.mergeStream(ws, project.id, streamName, true);
        setMergeResult({
          result,
          streamName,
          parentName: stream.parent || "main",
        });
      }),
    [ws, adapter, project.id, project.streams, runAction],
  );

  const handleConfirmMerge = useCallback(
    () =>
      runAction(async () => {
        if (!mergeResult) return;
        await adapter.mergeStream(ws, project.id, mergeResult.streamName);
        setMergeResult(null);
        setActiveStream(mergeResult.parentName);
        invalidateProject();
      }),
    [ws, adapter, project.id, mergeResult, setActiveStream, invalidateProject, runAction],
  );

  const handleDiffStream = useCallback(
    (streamName: string) =>
      runAction(async () => {
        const result = await adapter.diffStream(ws, project.id, streamName);
        setDiffResult(result);
      }),
    [ws, adapter, project.id, runAction],
  );

  const [archiveStreamName, setArchiveStreamName] = useState<string | null>(null);
  const handleDeleteStream = useCallback((streamName: string) => {
    setArchiveStreamName(streamName);
  }, []);
  const confirmArchiveStream = useCallback(
    () =>
      runAction(async () => {
        if (!archiveStreamName) return;
        await adapter.deleteStream(ws, project.id, archiveStreamName);
        setArchiveStreamName(null);
        setActiveStream("main");
        invalidateProject();
      }),
    [ws, adapter, project.id, archiveStreamName, setActiveStream, invalidateProject, runAction],
  );

  // Register stream actions into context so the TopBar StreamSelector can use them
  const { setActions } = useStreamActions();
  useEffect(() => {
    setActions({
      onCreateStream: () => setShowCreateStream(true),
      onEditStream: handleEditStream,
      onMergeStream: handleMergeStream,
      onDiffStream: handleDiffStream,
      onDeleteStream: handleDeleteStream,
    });
    return () => setActions({});
  }, [setActions, handleEditStream, handleMergeStream, handleDiffStream, handleDeleteStream]);

  return (
    <>
      {/* Files the server declined to import on the last upload */}
      {uploadSkipped.length > 0 && (
        <Alert
          variant="destructive"
          className="mb-3 relative pr-10"
          data-testid="upload-skipped-alert"
        >
          <AlertTitle>
            {uploadSkipped.length === 1
              ? "1 file was not imported"
              : `${uploadSkipped.length} files were not imported`}
          </AlertTitle>
          <AlertDescription>
            <ul className="list-disc pl-4">
              {uploadSkipped.map((f) => (
                <li key={f.name}>
                  <span className="font-medium">{f.name}</span>: {f.reason}
                </li>
              ))}
            </ul>
          </AlertDescription>
          <Button
            variant="ghost"
            size="sm"
            className="absolute top-2 right-2 h-6 w-6 p-0"
            onClick={() => setUploadSkipped([])}
            aria-label="Dismiss"
            data-testid="upload-skipped-dismiss"
          >
            <X className="w-3.5 h-3.5" />
          </Button>
        </Alert>
      )}
      {/* Why the last file/collection/stream action failed */}
      {actionError && (
        <Alert
          variant="destructive"
          className="mb-3 relative pr-10"
          data-testid="action-error-alert"
        >
          <AlertTitle>Action failed</AlertTitle>
          <AlertDescription>{actionError}</AlertDescription>
          <Button
            variant="ghost"
            size="sm"
            className="absolute top-2 right-2 h-6 w-6 p-0"
            onClick={() => setActionError(null)}
            aria-label="Dismiss"
            data-testid="action-error-dismiss"
          >
            <X className="w-3.5 h-3.5" />
          </Button>
        </Alert>
      )}
      <ProjectView
        project={project}
        onBack={() => navigate({ to: "/$workspace", params: { workspace: workspace ?? ws } })}
        onOpenFile={openItemIn("/$workspace/p/$projectId/s/$stream/translate/$")}
        // A row reads the file; the editors are entered from the preview's own
        // actions. Which file is open lives in the URL, so the reading
        // deep-links and Back closes it.
        preview={{
          itemName: search.preview ?? null,
          onOpen: (itemName) => setPreview(itemName),
          onClose: () => setPreview(undefined),
          targetLocales: project.target_languages,
          sourceLocale: project.default_source_language,
          onOpenTranslate: openItemIn("/$workspace/p/$projectId/s/$stream/translate/$"),
          onOpenReview: openItemIn("/$workspace/p/$projectId/s/$stream/review/$"),
          onOpenPreProcess: openItemIn("/$workspace/p/$projectId/s/$stream/pre-process/$"),
        }}
        collection={{
          id: search.collection ?? null,
          onSelect: (id) => setSourceSearch({ collection: id }),
        }}
        onUploadFiles={handleUploadFiles}
        onRemoveFile={handleRemoveFile}
        onOpenDashboard={() =>
          navigate({
            to: "/$workspace/p/$projectId/s/$stream",
            params: {
              workspace: workspace ?? ws,
              projectId: project.id,
              stream: activeStream || "main",
            },
          })
        }
        onOpenMemory={() =>
          navigate({ to: "/$workspace/memory", params: { workspace: workspace ?? ws } })
        }
        onOpenTerms={() =>
          navigate({ to: "/$workspace/terms", params: { workspace: workspace ?? ws } })
        }
        // The "Open in Bowrain Desktop" banner is a web-only upsell — never show
        // it inside the desktop app itself (window.location.origin there is the
        // webview asset host, not the server).
        serverMode={
          platform.kind === "web" && ws
            ? { serverURL: window.location.origin, workspaceSlug: ws }
            : undefined
        }
        // Project actions
        onManageMembers={() => setShowMembers(true)}
        onEditProject={() => setShowEditProject(true)}
        onArchiveProject={() => setShowArchiveProject(true)}
        onOpenSettings={() =>
          navigate({
            to: "/$workspace/p/$projectId/s/$stream/settings",
            params: {
              workspace: workspace ?? ws,
              projectId: project.id,
              stream: activeStream,
            },
          })
        }
        // Collection callbacks
        onCreateCollection={() => {
          setEditingCollection(undefined);
          setShowCollectionDialog(true);
        }}
        onEditCollection={handleEditCollection}
        onDeleteCollection={setDeleteCollectionId}
        onUploadToCollection={handleUploadToCollection}
      />

      {/* Edit Project Dialog */}
      <ProjectFormDialog
        open={showEditProject}
        onOpenChange={setShowEditProject}
        editProject={project}
        workspaceLanguages={activeWorkspace.languages}
        onSubmit={handleEditProjectSubmit}
      />

      {/* Project Members Dialog */}
      <Dialog open={showMembers} onOpenChange={setShowMembers}>
        <DialogContent className="sm:max-w-[640px]">
          <DialogHeader>
            <DialogTitle>Project Members</DialogTitle>
          </DialogHeader>
          <ProjectMemberManager
            workspace={activeWorkspace}
            projectId={project.id}
            projectLanguages={project.target_languages}
          />
        </DialogContent>
      </Dialog>

      {/* Create / Edit Collection Dialog */}
      <CreateCollectionDialog
        open={showCollectionDialog}
        onClose={() => {
          setShowCollectionDialog(false);
          setEditingCollection(undefined);
        }}
        onSubmit={handleCreateCollection}
        editCollection={editingCollection}
        voiceProfiles={voiceProfiles}
      />

      {/* Create Stream Dialog */}
      <StreamCreateDialog
        streams={project.streams ?? []}
        open={showCreateStream}
        onClose={() => setShowCreateStream(false)}
        onSubmit={handleCreateStream}
        voiceProfiles={voiceProfiles}
      />

      {/* Edit Stream Dialog */}
      <StreamEditDialog
        stream={editingStream}
        open={editingStream !== null}
        onClose={() => setEditingStream(null)}
        onSubmit={handleEditStreamSubmit}
        voiceProfiles={voiceProfiles}
      />

      {/* Merge Stream Dialog */}
      {mergeResult && (
        <StreamMergeDialog
          result={mergeResult.result}
          streamName={mergeResult.streamName}
          parentName={mergeResult.parentName}
          open={true}
          onConfirm={handleConfirmMerge}
          onClose={() => setMergeResult(null)}
        />
      )}

      {/* Diff View Dialog */}
      <Dialog
        open={diffResult !== null}
        onOpenChange={(v: boolean) => {
          if (!v) setDiffResult(null);
        }}
      >
        <DialogContent className="sm:max-w-[800px]">
          <DialogHeader>
            <DialogTitle>Stream Comparison</DialogTitle>
          </DialogHeader>
          {diffResult && <StreamDiffView diff={diffResult} />}
        </DialogContent>
      </Dialog>

      {/* Archive project confirmation */}
      <ConfirmDialog
        open={showArchiveProject}
        onOpenChange={setShowArchiveProject}
        title="Archive project"
        description="This project will be moved to the Recycle Bin. You can restore it at any time."
        confirmLabel="Archive"
        variant="destructive"
        onConfirm={confirmArchiveProject}
      />

      {/* Delete collection confirmation */}
      <ConfirmDialog
        open={deleteCollectionId !== null}
        onOpenChange={(v) => {
          if (!v) setDeleteCollectionId(null);
        }}
        title="Delete collection"
        description="Items in this collection will be moved to the default collection."
        confirmLabel="Delete"
        variant="destructive"
        onConfirm={confirmDeleteCollection}
      />

      {/* Archive stream confirmation */}
      <ConfirmDialog
        open={archiveStreamName !== null}
        onOpenChange={(v) => {
          if (!v) setArchiveStreamName(null);
        }}
        title="Archive stream"
        description={`Archive "${archiveStreamName ?? ""}"? You can restore it later.`}
        confirmLabel="Archive"
        variant="destructive"
        onConfirm={confirmArchiveStream}
      />
    </>
  );
}
