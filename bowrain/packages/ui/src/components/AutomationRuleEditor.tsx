import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
} from "@neokapi/ui-primitives";
import { useState, useEffect, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import type {
  AutomationCondition,
  AutomationAction,
  AutomationRule,
  FlowDefinitionInfo,
} from "../types/api";

const OPERATORS = ["equals", "contains", "exists"] as const;
const ACTION_TYPES = [
  "auto_translate",
  "create_review_tasks",
  "create_source_review",
  "run_flow",
  "webhook",
  "notify",
] as const;

// ---------------------------------------------------------------------------
// Condition row
// ---------------------------------------------------------------------------

function ConditionRow({
  condition,
  onChange,
  onRemove,
}: {
  condition: AutomationCondition;
  onChange: (c: AutomationCondition) => void;
  onRemove: () => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Input
        className="flex-1"
        placeholder="Field"
        value={condition.Field}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
          onChange({ ...condition, Field: e.target.value })
        }
      />
      <Select
        value={condition.Operator}
        onValueChange={(v: string) => onChange({ ...condition, Operator: v })}
      >
        <SelectTrigger className="w-[130px]">
          <SelectValue placeholder="Operator" />
        </SelectTrigger>
        <SelectContent>
          {OPERATORS.map((op) => (
            <SelectItem key={op} value={op}>
              {op}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        className="flex-1"
        placeholder="Value"
        value={condition.Value}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
          onChange({ ...condition, Value: e.target.value })
        }
        disabled={condition.Operator === "exists"}
      />
      <Button variant="ghost" size="sm" onClick={onRemove}>
        Remove
      </Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Action row
// ---------------------------------------------------------------------------

// Default config fields for workflow action types.
const ACTION_DEFAULTS: Record<string, Record<string, string>> = {
  create_review_tasks: { mode: "review" },
  create_source_review: { reviewer: "" },
};

function ActionRow({
  action,
  onChange,
  onRemove,
  flows,
}: {
  action: AutomationAction;
  onChange: (a: AutomationAction) => void;
  onRemove: () => void;
  flows: FlowDefinitionInfo[];
}) {
  const configEntries = Object.entries(action.Config);

  const updateConfig = useCallback(
    (key: string, value: string) => {
      onChange({ ...action, Config: { ...action.Config, [key]: value } });
    },
    [action, onChange],
  );

  const addConfigField = useCallback(() => {
    const key = `param_${Object.keys(action.Config).length + 1}`;
    onChange({ ...action, Config: { ...action.Config, [key]: "" } });
  }, [action, onChange]);

  const removeConfigField = useCallback(
    (key: string) => {
      const next = { ...action.Config };
      delete next[key];
      onChange({ ...action, Config: next });
    },
    [action, onChange],
  );

  const isRunFlow = action.Type === "run_flow";

  return (
    <div className="border rounded-md p-3 space-y-2">
      <div className="flex items-center gap-2">
        <Select
          value={action.Type}
          onValueChange={(v: string) => {
            const defaults = ACTION_DEFAULTS[v];
            onChange({ ...action, Type: v, Config: defaults ? { ...defaults } : action.Config });
          }}
        >
          <SelectTrigger className="w-[200px]">
            <SelectValue placeholder="Action type" />
          </SelectTrigger>
          <SelectContent>
            {ACTION_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {t.replace(/_/g, " ")}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="flex-1" />
        <Button variant="ghost" size="sm" onClick={onRemove}>
          Remove
        </Button>
      </div>
      {isRunFlow && (
        <div className="flex items-center gap-2 pl-2">
          <span className="text-xs text-muted-foreground w-24 shrink-0 font-mono">flow</span>
          <Select
            value={action.Config.flow ?? ""}
            onValueChange={(v: string) => updateConfig("flow", v)}
          >
            <SelectTrigger className="flex-1">
              <SelectValue placeholder="Select a flow" />
            </SelectTrigger>
            <SelectContent>
              {flows.length === 0 ? (
                <SelectItem value="_none" disabled>
                  No flows available
                </SelectItem>
              ) : (
                flows.map((f) => (
                  <SelectItem key={f.id} value={f.id}>
                    {f.name}
                    {f.source === "built-in" ? " (built-in)" : ""}
                  </SelectItem>
                ))
              )}
            </SelectContent>
          </Select>
        </div>
      )}
      {configEntries
        .filter(([key]) => !(isRunFlow && key === "flow"))
        .map(([key, val]) => (
          <div key={key} className="flex items-center gap-2 pl-2">
            <span className="text-xs text-muted-foreground w-24 shrink-0 font-mono">{key}</span>
            <Input
              className="flex-1"
              value={val}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                updateConfig(key, e.target.value)
              }
            />
            <Button variant="ghost" size="sm" onClick={() => removeConfigField(key)}>
              x
            </Button>
          </div>
        ))}
      <Button variant="outline" size="sm" onClick={addConfigField} className="ml-2">
        + Add parameter
      </Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Editor dialog
// ---------------------------------------------------------------------------

interface AutomationRuleEditorProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceSlug: string;
  projectId: string;
  rule?: AutomationRule;
  onSave: (data: {
    name: string;
    trigger: string;
    conditions: AutomationCondition[];
    actions: AutomationAction[];
    enabled: boolean;
  }) => void;
  saving?: boolean;
}

export function AutomationRuleEditor({
  open,
  onOpenChange,
  workspaceSlug,
  projectId,
  rule,
  onSave,
  saving,
}: AutomationRuleEditorProps) {
  const api = useApi();
  const [name, setName] = useState("");
  const [trigger, setTrigger] = useState("");
  const [conditions, setConditions] = useState<AutomationCondition[]>([]);
  const [actions, setActions] = useState<AutomationAction[]>([]);
  const [enabled, setEnabled] = useState(true);

  useEffect(() => {
    if (open) {
      setName(rule?.name ?? "");
      setTrigger(rule?.trigger ?? "");
      setConditions(rule?.conditions ?? []);
      setActions(rule?.actions ?? []);
      setEnabled(rule?.enabled ?? true);
    }
  }, [open, rule]);

  const { data: events } = useQuery({
    queryKey: ["automations", "events", workspaceSlug, projectId],
    queryFn: () => api.listAutomationEvents(workspaceSlug, projectId),
    staleTime: 60_000,
    enabled: open,
  });

  // Flow registry for run_flow actions — built-in flows merged with the
  // project's server-stored flows. Connector-agnostic: a flow runs server-side
  // on content from any connector.
  const { data: flows } = useQuery({
    queryKey: ["flows", workspaceSlug, projectId],
    queryFn: () => api.listFlowDefinitions(workspaceSlug, projectId),
    staleTime: 60_000,
    enabled: open,
  });

  const handleSubmit = useCallback(() => {
    onSave({ name, trigger, conditions, actions, enabled });
  }, [name, trigger, conditions, actions, enabled, onSave]);

  const addCondition = useCallback(() => {
    setConditions((prev) => [...prev, { Field: "", Operator: "equals", Value: "" }]);
  }, []);

  const updateCondition = useCallback((idx: number, c: AutomationCondition) => {
    setConditions((prev) => prev.map((item, i) => (i === idx ? c : item)));
  }, []);

  const removeCondition = useCallback((idx: number) => {
    setConditions((prev) => prev.filter((_, i) => i !== idx));
  }, []);

  const addAction = useCallback(() => {
    setActions((prev) => [...prev, { Type: "auto_translate", Config: {} }]);
  }, []);

  const updateAction = useCallback((idx: number, a: AutomationAction) => {
    setActions((prev) => prev.map((item, i) => (i === idx ? a : item)));
  }, []);

  const removeAction = useCallback((idx: number) => {
    setActions((prev) => prev.filter((_, i) => i !== idx));
  }, []);

  const isValid = name.trim() !== "" && trigger !== "" && actions.length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{rule ? "Edit Rule" : "New Automation Rule"}</DialogTitle>
        </DialogHeader>

        <div className="space-y-5 py-2">
          {/* Name */}
          <div className="space-y-1.5">
            <Label htmlFor="rule-name">Name</Label>
            <Input
              id="rule-name"
              value={name}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setName(e.target.value)}
              placeholder="e.g. Auto-translate on upload"
            />
          </div>

          {/* Trigger */}
          <div className="space-y-1.5">
            <Label>Trigger</Label>
            <Select value={trigger} onValueChange={setTrigger}>
              <SelectTrigger>
                <SelectValue placeholder="Select a trigger event" />
              </SelectTrigger>
              <SelectContent>
                {events?.map((ev) => (
                  <SelectItem key={ev.type} value={ev.type}>
                    {ev.description || ev.type}
                  </SelectItem>
                )) ?? (
                  <SelectItem value="_loading" disabled>
                    Loading events...
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
          </div>

          {/* Conditions */}
          <div className="space-y-2">
            <Label>Conditions</Label>
            {conditions.map((c, i) => (
              <ConditionRow
                key={i}
                condition={c}
                onChange={(updated) => updateCondition(i, updated)}
                onRemove={() => removeCondition(i)}
              />
            ))}
            <Button variant="outline" size="sm" onClick={addCondition}>
              + Add condition
            </Button>
          </div>

          {/* Actions */}
          <div className="space-y-2">
            <Label>Actions</Label>
            {actions.map((a, i) => (
              <ActionRow
                key={i}
                action={a}
                onChange={(updated) => updateAction(i, updated)}
                onRemove={() => removeAction(i)}
                flows={flows ?? []}
              />
            ))}
            <Button variant="outline" size="sm" onClick={addAction}>
              + Add action
            </Button>
          </div>

          {/* Enabled toggle */}
          <div className="flex items-center gap-3">
            <Switch checked={enabled} onCheckedChange={setEnabled} id="rule-enabled" />
            <Label htmlFor="rule-enabled">Enabled</Label>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!isValid || saving}>
            {saving ? "Saving..." : rule ? "Update" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
