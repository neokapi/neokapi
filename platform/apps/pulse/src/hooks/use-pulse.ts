import { useQuery } from "@tanstack/react-query";
import { fetchOverview, fetchProjects, fetchProjectDetail, fetchActivity, fetchLeaderboard, fetchTerms } from "../api";

export function usePulseOverview(workspace: string) {
  return useQuery({
    queryKey: ["pulse", workspace, "overview"],
    queryFn: () => fetchOverview(workspace),
    staleTime: 5 * 60_000,
  });
}

export function usePulseProjects(workspace: string) {
  return useQuery({
    queryKey: ["pulse", workspace, "projects"],
    queryFn: () => fetchProjects(workspace),
    staleTime: 2 * 60_000,
  });
}

export function usePulseProjectDetail(workspace: string, pid: string) {
  return useQuery({
    queryKey: ["pulse", workspace, "project", pid],
    queryFn: () => fetchProjectDetail(workspace, pid),
    staleTime: 2 * 60_000,
    enabled: !!pid,
  });
}

export function usePulseActivity(workspace: string, params?: URLSearchParams) {
  return useQuery({
    queryKey: ["pulse", workspace, "activity", params?.toString()],
    queryFn: () => fetchActivity(workspace, params),
    staleTime: 60_000,
  });
}

export function usePulseLeaderboard(workspace: string, params?: URLSearchParams) {
  return useQuery({
    queryKey: ["pulse", workspace, "leaderboard", params?.toString()],
    queryFn: () => fetchLeaderboard(workspace, params),
    staleTime: 10 * 60_000,
  });
}

export function usePulseTerms(workspace: string, params?: URLSearchParams) {
  return useQuery({
    queryKey: ["pulse", workspace, "terms", params?.toString()],
    queryFn: () => fetchTerms(workspace, params),
    staleTime: 15 * 60_000,
  });
}
