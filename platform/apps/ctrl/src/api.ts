// ---------------------------------------------------------------------------
// Admin API client wrapping /api/admin/* endpoints
// ---------------------------------------------------------------------------

import { getToken, refreshToken, login } from "./auth";
import type {
  AdminWorkspace,
  AdminWorkspaceDetail,
  FeatureOverride,
  WorkspaceNote,
  AdminUser,
  AdminUserDetail,
  PlatformMetrics,
  BillingEvent,
  UpsellOpportunity,
  LedgerEntry,
} from "./types";

const BASE_URL = "/api/admin";

async function getValidToken(): Promise<string> {
  let token = getToken();
  if (token) return token;

  // Try refreshing
  const refreshed = await refreshToken();
  if (refreshed) {
    token = getToken();
    if (token) return token;
  }

  // No valid token — redirect to login
  await login();
  // Will not reach here (page navigates away)
  throw new Error("Redirecting to login");
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = await getValidToken();

  const response = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...options?.headers,
    },
  });

  if (response.status === 401) {
    // Token expired mid-request — try refresh once
    const refreshed = await refreshToken();
    if (refreshed) {
      const retryToken = getToken();
      if (retryToken) {
        const retryResponse = await fetch(`${BASE_URL}${path}`, {
          ...options,
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${retryToken}`,
            ...options?.headers,
          },
        });
        if (!retryResponse.ok) {
          throw new Error(`API error: ${retryResponse.status}`);
        }
        return retryResponse.json() as Promise<T>;
      }
    }
    await login();
    throw new Error("Redirecting to login");
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}

// ---------------------------------------------------------------------------
// Workspaces
// ---------------------------------------------------------------------------

export function listWorkspaces(params?: {
  q?: string;
  plan?: string;
  status?: string;
}): Promise<AdminWorkspace[]> {
  const query = new URLSearchParams();
  if (params?.q) query.set("q", params.q);
  if (params?.plan) query.set("plan", params.plan);
  if (params?.status) query.set("status", params.status);
  const qs = query.toString();
  return request(`/workspaces${qs ? `?${qs}` : ""}`);
}

export function getWorkspace(id: string): Promise<AdminWorkspaceDetail> {
  return request(`/workspaces/${encodeURIComponent(id)}`);
}

export function updatePlan(id: string, plan: string): Promise<void> {
  return request(`/workspaces/${encodeURIComponent(id)}/plan`, {
    method: "PUT",
    body: JSON.stringify({ plan }),
  });
}

export function grantCredits(id: string, amount: number, reason: string): Promise<void> {
  return request(`/workspaces/${encodeURIComponent(id)}/credits`, {
    method: "POST",
    body: JSON.stringify({ amount, reason }),
  });
}

// ---------------------------------------------------------------------------
// Feature overrides
// ---------------------------------------------------------------------------

export function getFeatureOverrides(workspaceId: string): Promise<FeatureOverride[]> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/feature-overrides`);
}

export function setFeatureOverride(
  workspaceId: string,
  feature: string,
  enabled: boolean,
  reason: string,
  expiresAt?: string,
): Promise<void> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/feature-overrides`, {
    method: "PUT",
    body: JSON.stringify({ feature, enabled, reason, expires_at: expiresAt }),
  });
}

export function listAllOverrides(): Promise<FeatureOverride[]> {
  return request("/overrides");
}

// ---------------------------------------------------------------------------
// Notes
// ---------------------------------------------------------------------------

export function getNotes(workspaceId: string): Promise<WorkspaceNote[]> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/notes`);
}

export function addNote(workspaceId: string, content: string): Promise<void> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/notes`, {
    method: "POST",
    body: JSON.stringify({ content }),
  });
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

export function listUsers(params?: { q?: string }): Promise<AdminUser[]> {
  const query = new URLSearchParams();
  if (params?.q) query.set("q", params.q);
  const qs = query.toString();
  return request(`/users${qs ? `?${qs}` : ""}`);
}

export function getUser(id: string): Promise<AdminUserDetail> {
  return request(`/users/${encodeURIComponent(id)}`);
}

// ---------------------------------------------------------------------------
// Platform
// ---------------------------------------------------------------------------

export function getMetrics(): Promise<PlatformMetrics> {
  return request("/metrics");
}

export function listEvents(params?: {
  type?: string;
  from?: string;
  to?: string;
}): Promise<BillingEvent[]> {
  const query = new URLSearchParams();
  if (params?.type) query.set("type", params.type);
  if (params?.from) query.set("from", params.from);
  if (params?.to) query.set("to", params.to);
  const qs = query.toString();
  return request(`/events${qs ? `?${qs}` : ""}`);
}

export function getUpsells(): Promise<UpsellOpportunity[]> {
  return request("/upsells");
}

// ---------------------------------------------------------------------------
// Ledger
// ---------------------------------------------------------------------------

export function getLedger(workspaceId: string): Promise<LedgerEntry[]> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/ledger`);
}
