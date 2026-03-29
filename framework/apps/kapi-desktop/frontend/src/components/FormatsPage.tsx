import { useState, useEffect } from "react";
import { FileText, Loader2 } from "lucide-react";
import type { FormatInfo } from "../types/api";
import { api } from "../hooks/useApi";

export function FormatsPage() {
  const [formats, setFormats] = useState<FormatInfo[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.listFormats().then((f) => {
      if (f) setFormats(f);
      setLoading(false);
    });
  }, []);

  return (
    <div className="p-6">
      <h1 className="mb-6 text-xl font-semibold">Formats</h1>

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 size={14} className="animate-spin" />
          Loading formats...
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {formats.map((f) => (
            <div key={f.name} className="rounded-lg border border-border p-3">
              <div className="flex items-center gap-2">
                <FileText size={14} className="text-primary" />
                <span className="text-sm font-medium">{f.name}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
