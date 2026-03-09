import { useState, useEffect, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
} from "@gokapi/ui";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface AutomationCondition {
  Field: string;
  Operator: string;
  Value: string;
}

export interface AutomationAction {
  Type: string;
  Config: Record<string, string>;
}

export interface AutomationRule {
  id: string;
  project_id: string;
  name: string;
  trigger: string;
  conditions: AutomationCondition[];
  actions: AutomationAction[];
  enabled: boolean;
  builtin: boolean;
  created_at: string;
  updated_at: string;
}

export interface AutomationEvent {
  type: string;
  description: string;
}

const OPERATORS = ["equals", "contains", "exists"] as const;
const ACTION_TYPES = ["auto_translate", "run_flow", "webhook", "notify"] as const;

// ---------------------------------------------------------------------------
// API helpers
// ---------------------------------------------------------------------------

async function fetchEvents(ws: string, projectId: string): Promise<AutomationEvent[]> {
  const resp = await fetch(
    `/api/v1/workspaces/${encodeURIComponent(ws)}/projects/${encodeURIComponent(projectId)}/automations/events`,
    { credentials: "same-origin" },
  );
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status}: ${body}`);
  }
  return resp.json();
}

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
        onChange={(e) => onChange({ ...condition, Field: e.target.value })}
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
        onChange={(e) => onChange({ ...condition, Value: e.target.value })}
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

function ActionRow({
  action,
  onChange,
  onRemove,
}: {
  action: AutomationAction;
  onChange: (a: AutomationAction) => void;
  onRemove: () => void;
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

  return (
    <div className="border rounded-md p-3 space-y-2">
      <div className="flex items-center gap-2">
        <Select
          value={action.Type}
          onValueChange={(v: string) => onChange({ ...action, Type: v })}
        >
          <SelectTrigger className="w-[180px]">
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
      {configEntries.map(([key, val]) => (
        <div key={key} className="flex items-center gap-2 pl-2">
          <span className="text-xs text-muted-foreground w-24 shrink-0 font-mono">{key}</span>
          <Input
            className="flex-1"
            value={val}
            onChange={(e) => updateConfig(key, e.target.value)}
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
  /** If provided, the dialog is in edit mode. */
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
  const [name, setName] = useState("");
  const [trigger, setTrigger] = useState("");
  const [conditions, setConditions] = useState<AutomationCondition[]>([]);
  const [actions, setActions] = useState<AutomationAction[]>([]);
  const [enabled, setEnabled] = useState(true);

  // Reset form when rule or open state changes.
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
    queryFn: () => fetchEvents(workspaceSlug, projectId),
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
              onChange={(e) => setName(e.target.value)}
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
