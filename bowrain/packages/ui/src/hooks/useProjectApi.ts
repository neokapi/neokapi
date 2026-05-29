import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { ProjectInfo } from "../types/api";

/** useProjects is a cached query of the active workspace's projects, for pickers
 *  and dashboards that need the list reactively (the callback API below is for
 *  imperative flows). */
export function useProjects() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  return useQuery({
    queryKey: ["projects", ws],
    queryFn: () => api.listProjects(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });
}

export function useProjectApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const listProjects = useCallback(
    async (): Promise<ProjectInfo[]> => api.listProjects(ws),
    [api, ws],
  );

  const createProject = useCallback(
    async (
      name: string,
      defaultSourceLanguage: string,
      targetLanguages: string[],
    ): Promise<ProjectInfo> => api.createProject(ws, name, defaultSourceLanguage, targetLanguages),
    [api, ws],
  );

  const getProject = useCallback(
    async (projectId: string): Promise<ProjectInfo> => api.getProject(ws, projectId),
    [api, ws],
  );

  const deleteProject = useCallback(
    async (projectId: string): Promise<void> => api.deleteProject(ws, projectId),
    [api, ws],
  );

  const uploadFiles = useCallback(
    async (projectId: string, files: File[]): Promise<ProjectInfo> =>
      api.uploadFiles(ws, projectId, files),
    [api, ws],
  );

  const removeFile = useCallback(
    async (projectId: string, fileName: string): Promise<ProjectInfo> =>
      api.removeFile(ws, projectId, fileName),
    [api, ws],
  );

  return { listProjects, createProject, getProject, deleteProject, uploadFiles, removeFile };
}
