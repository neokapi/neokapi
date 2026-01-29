import { useState, useEffect, useCallback } from "react";
import type {
  FormatInfo,
  ToolInfo,
  FlowInfo,
} from "../types/api";

// Wails v2 generates bindings at runtime. In dev mode we fall back to fetch.
// The Go backend methods are available as window.go.backend.App.*
interface WailsBackend {
  ListFormats(): Promise<FormatInfo[]>;
  ListTools(): Promise<ToolInfo[]>;
  ListFlows(): Promise<FlowInfo[]>;
}

function getBackend(): WailsBackend | null {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const w = window as any;
  return w?.go?.backend?.App ?? null;
}

// Fallback fetch for non-Wails dev mode
const API_BASE = "/api/v1";
async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`);
  return res.json() as Promise<T>;
}

export function useFormats() {
  const [formats, setFormats] = useState<FormatInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const be = getBackend();
    const p = be
      ? be.ListFormats()
      : fetchJSON<FormatInfo[]>(`${API_BASE}/formats`);

    p.then(setFormats)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return { formats, loading, error };
}

export function useTools() {
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const be = getBackend();
    const p = be
      ? be.ListTools()
      : fetchJSON<ToolInfo[]>(`${API_BASE}/tools`);

    p.then(setTools)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return { tools, loading, error };
}

export function useFlows() {
  const [flows, setFlows] = useState<FlowInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const be = getBackend();
    const p = be
      ? be.ListFlows()
      : fetchJSON<FlowInfo[]>(`${API_BASE}/flows`);

    p.then(setFlows)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return { flows, loading, error };
}

export function useHealth() {
  const [connected, setConnected] = useState(false);

  const refresh = useCallback(() => {
    const be = getBackend();
    if (be) {
      // Wails is available - we're connected
      setConnected(true);
    } else {
      // Fallback: check REST API
      fetch(`${API_BASE}/health`)
        .then((r) => setConnected(r.ok))
        .catch(() => setConnected(false));
    }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 30000);
    return () => clearInterval(interval);
  }, [refresh]);

  return { connected, refresh };
}
