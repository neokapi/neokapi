import { useState, useEffect } from "react";
import { useApi } from "../context/ApiContext";
import type { ToolInfo } from "../types/api";

export function useTools() {
  const api = useApi();
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.listTools()
      .then((r) => setTools(r))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [api]);

  return { tools, loading, error };
}
