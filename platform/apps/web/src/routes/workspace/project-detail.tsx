import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery, useQueryClient } from "@tanstack/react-query";
import {
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
} from "@neokapi/ui";
import type {
  StreamVisibility,
  StreamMergeResult,
  StreamDiffResult,
  StreamInfo,
} from "@neokapi/ui";
import { projectQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";

export function ProjectDetailRoute() {
  const navigate = useNavigate();
  const { workspace, projectId } = useParams({ strict: false });
  const adapter = useApi();
  const queryClient = useQueryClient();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream, setActiveStream } = useStream();

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, activeStream),
  );

  useEffect(() => {
    document.title = `${project.name} — ${activeWorkspace.name} — Bowrain`;
  }, [project.name, activeWorkspace.name]);

  // ── File handlers ────────────────────────────────────────────────────

  const invalidateProject = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["project", ws, project.id] });
  }, [queryClient, ws, project.id]);

  const handleUploadFiles = useCallback(
    async (files: File[]) => {
      await adapter.uploadFiles(ws, project.id, files, activeStream);
      invalidateProject();
    },
    [ws, adapter, project.id, activeStream, invalidateProject],
  );

  const handleRemoveFile = useCallback(
    async (fileName: string) => {
      await adapter.removeFile(ws, project.id, fileName, activeStream);
      invalidateProject();
    },
    [ws, adapter, project.id, activeStream, invalidateProject],
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
    async (data: { name: string; kind: "uploaded" | "connected"; item_label: string }) => {
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
    },
    [ws, adapter, project.id, editingCollection, invalidateProject],
  );

  const handleEditCollection = useCallback((collection: import("@neokapi/ui").CollectionInfo) => {
    setEditingCollection(collection);
    setShowCollectionDialog(true);
  }, []);

  const [deleteCollectionId, setDeleteCollectionId] = useState<string | null>(null);
  const confirmDeleteCollection = useCallback(async () => {
    if (!deleteCollectionId) return;
    await adapter.deleteCollection(ws, project.id, deleteCollectionId);
    setDeleteCollectionId(null);
    invalidateProject();
  }, [ws, adapter, project.id, deleteCollectionId, invalidateProject]);

  const handleUploadToCollection = useCallback(
    async (collectionId: string, files: File[]) => {
      await adapter.uploadToCollection(ws, project.id, collectionId, files, activeStream);
      invalidateProject();
    },
    [ws, adapter, project.id, activeStream, invalidateProject],
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
    async (data: {
      name: string;
      parent: string;
      visibility: StreamVisibility;
      description: string;
    }) => {
      await adapter.createStream(ws, project.id, data);
      setShowCreateStream(false);
      setActiveStream(data.name);
      invalidateProject();
    },
    [ws, adapter, project.id, setActiveStream, invalidateProject],
  );

  const handleEditStream = useCallback((stream: StreamInfo) => {
    setEditingStream(stream);
  }, []);

  const handleEditStreamSubmit = useCallback(
    async (data: { description: string; visibility: StreamVisibility }) => {
      if (!editingStream) return;
      await adapter.updateStream(ws, project.id, editingStream.name, data);
      setEditingStream(null);
      invalidateProject();
    },
    [ws, adapter, project.id, editingStream, invalidateProject],
  );

  const handleMergeStream = useCallback(
    async (streamName: string) => {
      const stream = project.streams?.find((s) => s.name === streamName);
      if (!stream) return;
      // Dry run first
      const result = await adapter.mergeStream(ws, project.id, streamName, true);
      setMergeResult({
        result,
        streamName,
        parentName: stream.parent || "main",
      });
    },
    [ws, adapter, project.id, project.streams],
  );

  const handleConfirmMerge = useCallback(async () => {
    if (!mergeResult) return;
    await adapter.mergeStream(ws, project.id, mergeResult.streamName);
    setMergeResult(null);
    setActiveStream(mergeResult.parentName);
    invalidateProject();
  }, [ws, adapter, project.id, mergeResult, setActiveStream, invalidateProject]);

  const handleDiffStream = useCallback(
    async (streamName: string) => {
      const result = await adapter.diffStream(ws, project.id, streamName);
      setDiffResult(result);
    },
    [ws, adapter, project.id],
  );

  const [archiveStreamName, setArchiveStreamName] = useState<string | null>(null);
  const handleDeleteStream = useCallback((streamName: string) => {
    setArchiveStreamName(streamName);
  }, []);
  const confirmArchiveStream = useCallback(async () => {
    if (!archiveStreamName) return;
    await adapter.deleteStream(ws, project.id, archiveStreamName);
    setArchiveStreamName(null);
    setActiveStream("main");
    invalidateProject();
  }, [ws, adapter, project.id, archiveStreamName, setActiveStream, invalidateProject]);

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
      <ProjectView
        project={project}
        onBack={() => navigate({ to: "/$workspace", params: { workspace: workspace ?? ws } })}
        onOpenFile={(itemId) =>
          navigate({
            to: "/$workspace/p/$projectId/s/$stream/$itemId/translate",
            params: {
              workspace: workspace ?? ws,
              projectId: project.id,
              stream: activeStream,
              itemId,
            },
          })
        }
        onUploadFiles={handleUploadFiles}
        onRemoveFile={handleRemoveFile}
        onOpenDashboard={() =>
          navigate({
            to: "/$workspace/p/$projectId/s/$stream/dashboard",
            params: {
              workspace: workspace ?? ws,
              projectId: project.id,
              stream: activeStream || "main",
            },
          })
        }
        onOpenTM={() =>
          navigate({ to: "/$workspace/memory", params: { workspace: workspace ?? ws } })
        }
        onOpenTerms={() =>
          navigate({ to: "/$workspace/termbase", params: { workspace: workspace ?? ws } })
        }
        serverMode={ws ? { serverURL: window.location.origin, workspaceSlug: ws } : undefined}
        // Project actions
        onManageMembers={() => setShowMembers(true)}
        onEditProject={() => setShowEditProject(true)}
        onArchiveProject={() => setShowArchiveProject(true)}
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
      />

      {/* Create Stream Dialog */}
      <StreamCreateDialog
        streams={project.streams ?? []}
        open={showCreateStream}
        onClose={() => setShowCreateStream(false)}
        onSubmit={handleCreateStream}
      />

      {/* Edit Stream Dialog */}
      <StreamEditDialog
        stream={editingStream}
        open={editingStream !== null}
        onClose={() => setEditingStream(null)}
        onSubmit={handleEditStreamSubmit}
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
