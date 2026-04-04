import { Button, Switch } from "@neokapi/ui-primitives";
import { useState, useCallback } from "react";
import type { BravoConfig, BravoToolInfo } from "../../types/api";

export interface BravoConfigPanelProps {
  config: BravoConfig;
  tools: BravoToolInfo[];
  onSave: (config: Partial<BravoConfig>) => void;
  saving?: boolean;
}

/** Per-tool policy state. */
type ToolPolicy = "allow" | "deny" | "approve";

function resolvePolicy(toolName: string, config: BravoConfig): ToolPolicy {
  if (config.denied_tools?.includes(toolName)) return "deny";
  if (config.require_approval?.includes(toolName)) return "approve";
  return "allow";
}

function policyLabel(policy: ToolPolicy): string {
  switch (policy) {
    case "allow":
      return "Allow";
    case "deny":
      return "Deny";
    case "approve":
      return "Approve";
  }
}

function policyColor(policy: ToolPolicy): string {
  switch (policy) {
    case "allow":
      return "text-success dark:text-success";
    case "deny":
      return "text-destructive";
    case "approve":
      return "text-warning dark:text-warning";
  }
}

export function BravoConfigPanel({ config, tools, onSave, saving }: BravoConfigPanelProps) {
  const [enabled, setEnabled] = useState(config.enabled);
  const [codeExec, setCodeExec] = useState(config.code_exec_enabled);
  const [maxConcurrent, setMaxConcurrent] = useState(config.max_concurrent);

  // Per-tool policy overrides. Initialized from config arrays.
  const [toolPolicies, setToolPolicies] = useState<Record<string, ToolPolicy>>(() => {
    const policies: Record<string, ToolPolicy> = {};
    for (const tool of tools) {
      policies[tool.name] = resolvePolicy(tool.name, config);
    }
    return policies;
  });

  const setToolPolicy = useCallback((toolName: string, policy: ToolPolicy) => {
    setToolPolicies((prev) => ({ ...prev, [toolName]: policy }));
  }, []);

  const cyclePolicy = useCallback((toolName: string) => {
    setToolPolicies((prev) => {
      const current = prev[toolName] ?? "allow";
      const next: ToolPolicy =
        current === "allow" ? "approve" : current === "approve" ? "deny" : "allow";
      return { ...prev, [toolName]: next };
    });
  }, []);

  const handleSave = () => {
    // Build tool policy arrays from per-tool state.
    const denied_tools: string[] = [];
    const require_approval: string[] = [];

    for (const [name, policy] of Object.entries(toolPolicies)) {
      if (policy === "deny") denied_tools.push(name);
      else if (policy === "approve") require_approval.push(name);
    }

    onSave({
      enabled,
      code_exec_enabled: codeExec,
      max_concurrent: maxConcurrent,
      denied_tools,
      require_approval,
    });
  };

  // Check if anything changed from the original config.
  const toolPoliciesDirty = tools.some(
    (t) => (toolPolicies[t.name] ?? "allow") !== resolvePolicy(t.name, config),
  );

  const dirty =
    enabled !== config.enabled ||
    codeExec !== config.code_exec_enabled ||
    maxConcurrent !== config.max_concurrent ||
    toolPoliciesDirty;

  return (
    <div className="space-y-6">
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

      {tools.length > 0 && (
        <div>
          <h4 className="text-xs uppercase text-muted-foreground font-medium mb-2">
            Tool policies ({tools.length})
          </h4>
          <div className="text-xs text-muted-foreground mb-3">
            Click the policy badge to cycle: Allow → Approve → Deny
          </div>
          <div className="grid gap-1 max-h-64 overflow-y-auto">
            {tools.map((tool) => {
              const policy = toolPolicies[tool.name] ?? "allow";
              return (
                <div
                  key={tool.name}
                  className="flex items-center justify-between rounded px-2 py-1.5 text-xs hover:bg-muted group"
                >
                  <span className="font-mono">{tool.name}</span>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => cyclePolicy(tool.name)}
                      className={`px-2 py-0.5 rounded text-[10px] font-medium transition-colors cursor-pointer hover:opacity-80 ${policyColor(policy)}`}
                      title={`Click to change policy (current: ${policyLabel(policy)})`}
                    >
                      {policyLabel(policy)}
                    </button>
                    <select
                      value={policy}
                      onChange={(e) => setToolPolicy(tool.name, e.target.value as ToolPolicy)}
                      className="text-[10px] bg-transparent border rounded px-1 py-0.5 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
                    >
                      <option value="allow">Allow</option>
                      <option value="approve">Approve</option>
                      <option value="deny">Deny</option>
                    </select>
                  </div>
                </div>
              );
            })}
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
