import { useEffect, useCallback, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  RecycleBinView,
  ConfirmDialog,
  useWorkspace,
  useApi,
  Card,
  PageContainer,
} from "@neokapi/ui";

export function BinRoute() {
  const { activeWorkspace } = useWorkspace();
  const adapter = useApi();
  const queryClient = useQueryClient();
  const ws = activeWorkspace?.slug ?? "";

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Recycle Bin · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const { data: projects, isFetching } = useQuery({
    queryKey: ["archived-projects", ws],
    queryFn: () => adapter.listArchivedProjects(ws),
    enabled: !!ws,
    staleTime: 10_000,
  });

  const handleRestore = useCallback(
    async (id: string) => {
      await adapter.restoreProject(ws, id);
      void queryClient.invalidateQueries({ queryKey: ["archived-projects", ws] });
      void queryClient.invalidateQueries({ queryKey: ["projects", ws] });
    },
    [ws, adapter, queryClient],
  );

  const [deleteProjectId, setDeleteProjectId] = useState<string | null>(null);
  const confirmPermanentlyDelete = useCallback(async () => {
    if (!deleteProjectId) return;
    await adapter.permanentlyDeleteProject(ws, deleteProjectId);
    setDeleteProjectId(null);
    void queryClient.invalidateQueries({ queryKey: ["archived-projects", ws] });
  }, [ws, adapter, deleteProjectId, queryClient]);

  if (!activeWorkspace) {
    return (
      <PageContainer width="narrow">
        <Card className="mt-8 p-8 text-center text-muted-foreground text-sm">
          Select a workspace
        </Card>
      </PageContainer>
    );
  }

  return (
    <PageContainer width="wide">
      {/* No server-side auto-purge of archived projects exists today, so we don't
          claim a retention window: projects are kept until manually deleted.
          Pass a positive retentionDays here once a purge policy is enforced. */}
      <RecycleBinView
        projects={projects ?? []}
        loading={isFetching}
        onRestoreProject={handleRestore}
        onPermanentlyDelete={setDeleteProjectId}
      />

      <ConfirmDialog
        open={deleteProjectId !== null}
        onOpenChange={(v) => {
          if (!v) setDeleteProjectId(null);
        }}
        title="Permanently delete project"
        description="This project and all its data will be permanently deleted. This action cannot be undone."
        confirmLabel="Delete permanently"
        variant="destructive"
        onConfirm={confirmPermanentlyDelete}
      />
    </PageContainer>
  );
}
