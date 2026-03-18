import { useState } from "react";
import { cn } from "../../lib/utils";
import type { BravoConfig, BravoToolInfo, BravoUsageSummary } from "../../types/api";
import { Button } from "../ui/button";
import { Switch } from "../ui/switch";

export interface BravoConfigPanelProps {
  config: BravoConfig;
  tools: BravoToolInfo[];
  usage?: BravoUsageSummary;
  onSave: (config: Partial<BravoConfig>) => void;
  saving?: boolean;
}

export function BravoConfigPanel({
  config,
  tools,
  usage,
  onSave,
  saving,
}: BravoConfigPanelProps) {
  const [enabled, setEnabled] = useState(config.enabled);
  const [codeExec, setCodeExec] = useState(config.code_exec_enabled);
  const [maxConcurrent, setMaxConcurrent] = useState(config.max_concurrent);

  const handleSave = () => {
    onSave({
      enabled,
      code_exec_enabled: codeExec,
      max_concurrent: maxConcurrent,
    });
  };

  const dirty =
    enabled !== config.enabled ||
    codeExec !== config.code_exec_enabled ||
    maxConcurrent !== config.max_concurrent;

  return (
    <div className="space-y-6 p-4">
      <div>
        <h3 className="text-sm font-semibold mb-4">@bravo Configuration</h3>

        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm font-medium">Enable @bravo</div>
              <div className="text-xs text-muted-foreground">
                Allow the AI agent to operate in this workspace
              </div>
            </div>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </div>

          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm font-medium">Code execution</div>
              <div className="text-xs text-muted-foreground">
                Allow @bravo to run scripts in sandboxed containers
              </div>
            </div>
            <Switch checked={codeExec} onCheckedChange={setCodeExec} />
          </div>

          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm font-medium">Max concurrent</div>
              <div className="text-xs text-muted-foreground">
                Maximum simultaneous agent conversations
              </div>
            </div>
            <input
              type="number"
              min={1}
              max={20}
              value={maxConcurrent}
              onChange={(e) => setMaxConcurrent(parseInt(e.target.value) || 1)}
              className="w-16 rounded-md border px-2 py-1 text-sm text-center"
            />
          </div>
        </div>
      </div>

      {tools.length > 0 && (
        <div>
          <h4 className="text-xs uppercase text-muted-foreground font-medium mb-2">
            Available tools ({tools.length})
          </h4>
          <div className="grid gap-1 max-h-48 overflow-y-auto">
            {tools.map((tool) => (
              <div
                key={tool.name}
                className="flex items-center justify-between rounded px-2 py-1 text-xs hover:bg-muted"
              >
                <span className="font-mono">{tool.name}</span>
                {tool.require_approval && (
                  <span className="text-amber-600 dark:text-amber-400 text-[10px]">
                    approval
                  </span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {usage && (
        <div>
          <h4 className="text-xs uppercase text-muted-foreground font-medium mb-2">
            Usage this month
          </h4>
          <div className="grid grid-cols-2 gap-2">
            <div className="rounded-md border p-2">
              <div className="text-lg font-semibold">
                {((usage.total_input_tokens + usage.total_output_tokens) / 1000).toFixed(1)}k
              </div>
              <div className="text-[10px] text-muted-foreground">Total tokens</div>
            </div>
            <div className="rounded-md border p-2">
              <div className="text-lg font-semibold">
                {Math.ceil(usage.total_container_sec / 60)}m
              </div>
              <div className="text-[10px] text-muted-foreground">Container time</div>
            </div>
            <div className="rounded-md border p-2">
              <div className="text-lg font-semibold">{usage.message_count}</div>
              <div className="text-[10px] text-muted-foreground">Messages</div>
            </div>
            <div className="rounded-md border p-2">
              <div className="text-lg font-semibold">
                {usage.total_input_tokens.toLocaleString()}
              </div>
              <div className="text-[10px] text-muted-foreground">Input tokens</div>
            </div>
          </div>
        </div>
      )}

      {dirty && (
        <Button onClick={handleSave} disabled={saving} size="sm" className="w-full">
          {saving ? "Saving..." : "Save changes"}
        </Button>
      )}
    </div>
  );
}
