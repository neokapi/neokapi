import { useState, useEffect } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { TranslationEditor, useApi, useWorkspace, type ProjectInfo } from "@gokapi/ui";

export function TranslateRoute() {
  const navigate = useNavigate();
  const { workspace, projectId, fileName } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [project, setProject] = useState<ProjectInfo | null>(null);

  useEffect(() => {
    if (!ws || !projectId) return;
    adapter.getProject(ws, projectId).then(setProject).catch(() => setProject(null));
  }, [ws, projectId, adapter]);

  if (!project || !fileName) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        Loading editor...
      </div>
    );
  }

  return (
    <TranslationEditor
      project={project}
      fileName={fileName}
      onBack={() =>
        navigate({
          to: "/$workspace/project/$projectId",
          params: { workspace: workspace ?? ws, projectId: project.id },
        })
      }
    />
  );
}
