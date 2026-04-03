import { useState, useEffect } from "react";
import { useApi } from "../context/ApiContext";
import type { FormatInfo } from "../types/api";

export function useFormats() {
  const api = useApi();
  const [formats, setFormats] = useState<FormatInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .listFormats()
      .then((r) => setFormats(r))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [api]);

  return { formats, loading, error };
}
