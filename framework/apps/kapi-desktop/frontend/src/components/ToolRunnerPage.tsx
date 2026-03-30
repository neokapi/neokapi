import { useState, useEffect } from "react";
import { Play, FileInput, Loader2 } from "lucide-react";
import type { ToolInfo } from "../types/api";
import { api } from "../hooks/useApi";

export function ToolRunnerPage() {
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedTool, setSelectedTool] = useState<string | null>(null);
  const [targetLang, setTargetLang] = useState("");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.listTools().then((result) => {
      if (result) setTools(result);
      setLoading(false);
    });
  }, []);

  const handleRun = async () => {
    if (!selectedTool || !targetLang) return;
    setRunning(true);
    setError(null);
    try {
      await api.runFlow(selectedTool, [], targetLang);
    } catch (e) {
      setError(String(e));
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="flex h-full">
      {/* Tool list */}
      <div className="w-56 shrink-0 border-r border-border p-3">
        <h2 className="mb-3 text-sm font-medium">Tools</h2>
        {loading ? (
          <div className="flex items-center gap-2 px-2 py-4 text-sm text-muted-foreground">
            <Loader2 size={14} className="animate-spin" />
            Loading tools...
          </div>
        ) : (
          <div className="space-y-0.5">
            {tools.map((tool) => (
              <button
                key={tool.name}
                onClick={() => setSelectedTool(tool.name)}
                className={`w-full rounded px-2 py-1.5 text-left text-sm ${
                  selectedTool === tool.name
                    ? "bg-accent text-accent-foreground"
                    : "text-muted-foreground hover:bg-accent/50"
                }`}
                aria-label={`Select tool ${tool.name}`}
              >
                <div className="truncate">{tool.name}</div>
                <div className="truncate text-xs opacity-60">{tool.category}</div>
              </button>
            ))}
            {tools.length === 0 && (
              <p className="px-2 text-xs text-muted-foreground">No tools available</p>
            )}
          </div>
        )}
      </div>

      {/* Tool runner */}
      <div className="flex-1 p-6">
        {selectedTool ? (
          <div className="space-y-4">
            <h2 className="text-lg font-medium">{selectedTool}</h2>
            <p className="text-sm text-muted-foreground">
              {tools.find((t) => t.name === selectedTool)?.description}
            </p>

            <div>
              <label className="mb-1 block text-sm font-medium" htmlFor="tool-files">
                Input Files
              </label>
              <button
                id="tool-files"
                className="flex items-center gap-2 rounded-md border border-dashed border-border px-4 py-3 text-sm text-muted-foreground hover:border-primary hover:text-primary"
                aria-label="Select input files"
              >
                <FileInput size={16} />
                Select files...
              </button>
            </div>

            <div>
              <label className="mb-1 block text-sm font-medium" htmlFor="tool-target-lang">
                Target Language
              </label>
              <input
                id="tool-target-lang"
                type="text"
                value={targetLang}
                onChange={(e) => setTargetLang(e.target.value)}
                placeholder="e.g. fr-FR"
                className="w-48 rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              />
            </div>

            {error && (
              <p className="text-sm text-destructive" role="alert">
                {error}
              </p>
            )}

            <button
              onClick={handleRun}
              disabled={!targetLang || running}
              className="flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              aria-label={`Run ${selectedTool}`}
            >
              {running ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
              {running ? "Running..." : `Run ${selectedTool}`}
            </button>
          </div>
        ) : (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            <p className="text-sm">Select a tool to configure and run</p>
          </div>
        )}
      </div>
    </div>
  );
}
