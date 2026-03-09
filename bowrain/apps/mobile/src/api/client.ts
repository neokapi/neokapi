/** Lightweight REST client for the Bowrain API. */

export interface ApiClientConfig {
  baseUrl: string;
  getToken: () => Promise<string | null>;
  onUnauthorized?: () => void;
}

export class ApiClient {
  private config: ApiClientConfig;

  constructor(config: ApiClientConfig) {
    this.config = config;
  }

  private async headers(): Promise<Record<string, string>> {
    const h: Record<string, string> = { "Content-Type": "application/json" };
    const token = await this.config.getToken();
    if (token) h["Authorization"] = `Bearer ${token}`;
    return h;
  }

  async get<T>(path: string): Promise<T> {
    return this.request("GET", path);
  }

  async post<T>(path: string, body?: unknown): Promise<T> {
    return this.request("POST", path, body);
  }

  async put<T>(path: string, body?: unknown): Promise<T> {
    return this.request("PUT", path, body);
  }

  async delete(path: string): Promise<void> {
    await this.request("DELETE", path);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const resp = await fetch(`${this.config.baseUrl}${path}`, {
      method,
      headers: await this.headers(),
      body: body ? JSON.stringify(body) : undefined,
    });

    if (resp.status === 401) {
      this.config.onUnauthorized?.();
      throw new Error("Unauthorized");
    }

    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(`${resp.status}: ${text}`);
    }

    if (resp.status === 204) return undefined as T;
    return resp.json();
  }
}

// API response types (subset of platform types needed for mobile)

export interface Workspace {
  id: string;
  name: string;
  slug: string;
  type: string;
  role: string;
}

export interface ProjectInfo {
  id: string;
  name: string;
  source_locale: string;
  target_locales: string[];
}

export interface ReviewItem {
  id: string;
  project_id: string;
  type: string;
  status: string;
  data: Record<string, unknown>;
  occurrences: Array<{ block_id: string; context: string }>;
  confidence: number;
  locale: string;
  assigned_to: string;
  created_at: string;
}

export interface ReviewDecision {
  item_id: string;
  status: "approved" | "rejected" | "skipped";
  notes?: string;
}

export interface SyncRequest {
  decisions: ReviewDecision[];
  last_sync_at?: string;
}

export interface SyncResponse {
  synced: number;
  new_items: ReviewItem[];
}

export interface NotificationInfo {
  id: string;
  type: string;
  title: string;
  body: string;
  read: boolean;
  created_at: string;
}
