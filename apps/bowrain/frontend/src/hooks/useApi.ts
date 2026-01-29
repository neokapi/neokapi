import { useState, useEffect, useCallback } from "react";
import type {
  FormatInfo,
  ToolInfo,
  FlowInfo,
  HealthResponse,
} from "../types/api";

const API_BASE = "/api/v1";

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export function useFormats() {
  const [formats, setFormats] = useState<FormatInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchJSON<FormatInfo[]>(`${API_BASE}/formats`)
      .then(setFormats)
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
    fetchJSON<ToolInfo[]>(`${API_BASE}/tools`)
      .then(setTools)
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
    fetchJSON<FlowInfo[]>(`${API_BASE}/flows`)
      .then(setFlows)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return { flows, loading, error };
}

export function useHealth() {
  const [health, setHealth] = useState<HealthResponse | null>(null);

  const refresh = useCallback(() => {
    fetchJSON<HealthResponse>(`${API_BASE}/health`)
      .then(setHealth)
      .catch(() => setHealth(null));
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 30000);
    return () => clearInterval(interval);
  }, [refresh]);

  return { health, refresh };
}
