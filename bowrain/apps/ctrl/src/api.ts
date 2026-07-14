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
  ImpersonateResponse,
} from "./types";

function resolveApiBaseUrl(): string {
  if (import.meta.env.VITE_ADMIN_API_URL) {
    return import.meta.env.VITE_ADMIN_API_URL;
  }
  // Same-origin: every deployment fronts ctrl with a proxy that forwards
  // /api/* to bowrain-server (Caddy in prod, traefik/the vite dev proxy
  // locally). A cross-origin absolute fallback (e.g. the apex domain) would
  // have no DNS record in prod and be blocked by the server's CORS allowlist
  // anyway — it only allows the OIDC public URL origin.
  return "/api/admin";
}

const BASE_URL = resolveApiBaseUrl();

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
      ...(options?.headers as Record<string, string>),
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
            ...(options?.headers as Record<string, string>),
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

export async function listWorkspaces(params?: {
  q?: string;
  plan?: string;
  status?: string;
}): Promise<AdminWorkspace[]> {
  const query = new URLSearchParams();
  if (params?.q) query.set("q", params.q);
  if (params?.plan) query.set("plan", params.plan);
  if (params?.status) query.set("status", params.status);
  const qs = query.toString();
  const resp = await request<{ workspaces: AdminWorkspace[]; total: number }>(
    `/workspaces${qs ? `?${qs}` : ""}`,
  );
  return resp.workspaces ?? [];
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

export async function getFeatureOverrides(workspaceId: string): Promise<FeatureOverride[]> {
  const resp = await request<{ overrides: FeatureOverride[] }>(
    `/workspaces/${encodeURIComponent(workspaceId)}/feature-overrides`,
  );
  return resp.overrides ?? [];
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

export async function listAllOverrides(): Promise<FeatureOverride[]> {
  const resp = await request<{ overrides: FeatureOverride[] }>("/overrides");
  return resp.overrides ?? [];
}

// ---------------------------------------------------------------------------
// Notes
// ---------------------------------------------------------------------------

export async function getNotes(workspaceId: string): Promise<WorkspaceNote[]> {
  const resp = await request<{ notes: WorkspaceNote[] }>(
    `/workspaces/${encodeURIComponent(workspaceId)}/notes`,
  );
  return resp.notes ?? [];
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

export async function listUsers(params?: { q?: string }): Promise<AdminUser[]> {
  const query = new URLSearchParams();
  if (params?.q) query.set("q", params.q);
  const qs = query.toString();
  const resp = await request<{ users: AdminUser[]; total: number }>(`/users${qs ? `?${qs}` : ""}`);
  return resp.users ?? [];
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

export async function listEvents(params?: {
  type?: string;
  from?: string;
  to?: string;
}): Promise<BillingEvent[]> {
  const query = new URLSearchParams();
  if (params?.type) query.set("type", params.type);
  if (params?.from) query.set("from", params.from);
  if (params?.to) query.set("to", params.to);
  const qs = query.toString();
  const resp = await request<{ events: BillingEvent[] }>(`/events${qs ? `?${qs}` : ""}`);
  return resp.events ?? [];
}

export async function getUpsells(): Promise<UpsellOpportunity[]> {
  const resp = await request<{ upsells: UpsellOpportunity[] }>("/upsells");
  return resp.upsells ?? [];
}

// ---------------------------------------------------------------------------
// Ledger
// ---------------------------------------------------------------------------

export function getLedger(workspaceId: string): Promise<LedgerEntry[]> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/ledger`);
}

// ---------------------------------------------------------------------------
// Model Usage
// ---------------------------------------------------------------------------

export interface ModelUsageEntry {
  model: string;
  operation: string;
  prompt_tokens: number;
  output_tokens: number;
  total_tokens: number;
  call_count: number;
}

export interface RunnerUsageEntry {
  operation: string;
  total_seconds: number;
  count: number;
}

export function getModelUsage(
  workspaceId: string,
): Promise<{ model_usage: ModelUsageEntry[]; runner_usage?: RunnerUsageEntry[] }> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/model-usage`);
}

// ---------------------------------------------------------------------------
// Impersonation
// ---------------------------------------------------------------------------

export function impersonateWorkspace(workspaceId: string): Promise<ImpersonateResponse> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/impersonate`, {
    method: "POST",
  });
}

// ---------------------------------------------------------------------------
// Member management
// ---------------------------------------------------------------------------

export function addMemberToWorkspace(
  workspaceId: string,
  userId: string,
  role: string,
): Promise<void> {
  return request(`/workspaces/${encodeURIComponent(workspaceId)}/members`, {
    method: "POST",
    body: JSON.stringify({ user_id: userId, role }),
  });
}

// ---------------------------------------------------------------------------
// Platform configuration (instance-wide runtime settings)
// ---------------------------------------------------------------------------

export type ModelTier = "fast" | "balanced" | "flagship";

export interface CatalogModel {
  id: string;
  name: string;
  tier: ModelTier;
  requires_marketplace?: boolean;
}

export interface AISettings {
  provider: string;
  default_model: string;
  base_url?: string;
  enabled_models: string[];
  customer_choice: boolean;
}

export interface MaintenanceSettings {
  enabled: boolean;
  message?: string;
}

export interface WorkspaceDefaults {
  plan: string;
  trial_days: number;
}

export interface PlatformSnapshot {
  ai: AISettings;
  signups: { open: boolean };
  maintenance: MaintenanceSettings;
  workspace_defaults: WorkspaceDefaults;
  features: Record<string, boolean>;
  model_catalog: CatalogModel[];
}

export interface PlatformDefaults {
  ai_provider: string;
  ai_default_model: string;
  ai_base_url: string;
  signups_open: boolean;
  default_plan: string;
  trial_days: number;
}

export interface PlatformConfigResponse {
  snapshot: PlatformSnapshot;
  persistent: boolean;
  defaults: PlatformDefaults;
}

// PlatformConfigUpdate is a PATCH-style body: every section/field is optional, so
// a section can be saved without touching the rest.
export interface PlatformConfigUpdate {
  ai?: {
    provider?: string;
    default_model?: string;
    base_url?: string;
    enabled_models?: string[];
    customer_choice?: boolean;
  };
  signups?: { open?: boolean };
  maintenance?: { enabled?: boolean; message?: string };
  workspace_defaults?: { plan?: string; trial_days?: number };
  features?: Record<string, boolean>;
}

export function getPlatformConfig(): Promise<PlatformConfigResponse> {
  return request("/platform");
}

export function updatePlatformConfig(
  update: PlatformConfigUpdate,
): Promise<PlatformConfigResponse> {
  return request("/platform", {
    method: "PUT",
    body: JSON.stringify(update),
  });
}

// ---------------------------------------------------------------------------
// Workspace slug reservations (rename grace period)
// ---------------------------------------------------------------------------

export interface SlugReservation {
  slug: string;
  workspace_id: string;
  reserved_until: string;
  created_at: string;
}

export function listSlugReservations(): Promise<SlugReservation[]> {
  return request<SlugReservation[]>("/slug-reservations");
}

export function releaseSlugReservation(slug: string): Promise<void> {
  return request("/slug-reservations/release", {
    method: "POST",
    body: JSON.stringify({ slug }),
  });
}
