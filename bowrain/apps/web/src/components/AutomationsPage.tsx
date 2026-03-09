import { useState, useCallback, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button, GlassCard, Switch, Badge } from "@gokapi/ui";
import { AutomationRuleEditor, type AutomationRule, type AutomationCondition, type AutomationAction } from "./AutomationRuleEditor";
import { AutomationHistory } from "./AutomationHistory";

// ---------------------------------------------------------------------------
// API helpers
// ---------------------------------------------------------------------------

function automationsUrl(ws: string, projectId: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(ws)}/projects/${encodeURIComponent(projectId)}/automations`;
}

async function fetchRules(ws: string, projectId: string): Promise<AutomationRule[]> {
  const resp = await fetch(automationsUrl(ws, projectId), { credentials: "same-origin" });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status}: ${body}`);
  }
  return resp.json();
}

async function createRule(
  ws: string,
  projectId: string,
  data: { name: string; trigger: string; conditions: AutomationCondition[]; actions: AutomationAction[]; enabled: boolean },
): Promise<AutomationRule> {
  const resp = await fetch(automationsUrl(ws, projectId), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(data),
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status}: ${body}`);
  }
  return resp.json();
}

async function updateRule(
  ws: string,
  projectId: string,
  ruleId: string,
  data: { name: string; trigger: string; conditions: AutomationCondition[]; actions: AutomationAction[]; enabled: boolean },
): Promise<AutomationRule> {
  const resp = await fetch(`${automationsUrl(ws, projectId)}/${encodeURIComponent(ruleId)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(data),
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status}: ${body}`);
  }
  return resp.json();
}

async function deleteRule(ws: string, projectId: string, ruleId: string): Promise<void> {
  const resp = await fetch(`${automationsUrl(ws, projectId)}/${encodeURIComponent(ruleId)}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status}: ${body}`);
  }
}

async function toggleRule(ws: string, projectId: string, ruleId: string): Promise<AutomationRule> {
  const resp = await fetch(`${automationsUrl(ws, projectId)}/${encodeURIComponent(ruleId)}/toggle`, {
    method: "PATCH",
    credentials: "same-origin",
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status}: ${body}`);
  }
  return resp.json();
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface AutomationsPageProps {
  workspaceSlug: string;
  projectId: string;
}

export function AutomationsPage({ workspaceSlug, projectId }: AutomationsPageProps) {
  const queryClient = useQueryClient();
  const rulesQueryKey = ["automations", "rules", workspaceSlug, projectId];

  // ---- Data fetching ----
  const { data: rules, isLoading, error } = useQuery({
    queryKey: rulesQueryKey,
    queryFn: () => fetchRules(workspaceSlug, projectId),
    staleTime: 15_000,
  });

  // ---- Editor state ----
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<AutomationRule | undefined>();

  // ---- Mutations ----
  const createMutation = useMutation({
    mutationFn: (data: { name: string; trigger: string; conditions: AutomationCondition[]; actions: AutomationAction[]; enabled: boolean }) =>
      createRule(workspaceSlug, projectId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: rulesQueryKey });
      setEditorOpen(false);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ ruleId, data }: { ruleId: string; data: { name: string; trigger: string; conditions: AutomationCondition[]; actions: AutomationAction[]; enabled: boolean } }) =>
      updateRule(workspaceSlug, projectId, ruleId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: rulesQueryKey });
      setEditorOpen(false);
      setEditingRule(undefined);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (ruleId: string) => deleteRule(workspaceSlug, projectId, ruleId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: rulesQueryKey });
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (ruleId: string) => toggleRule(workspaceSlug, projectId, ruleId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: rulesQueryKey });
    },
  });

  // ---- Handlers ----
  const handleNewRule = useCallback(() => {
    setEditingRule(undefined);
    setEditorOpen(true);
  }, []);

  const handleEditRule = useCallback((rule: AutomationRule) => {
    setEditingRule(rule);
    setEditorOpen(true);
  }, []);

  const handleSave = useCallback(
    (data: { name: string; trigger: string; conditions: AutomationCondition[]; actions: AutomationAction[]; enabled: boolean }) => {
      if (editingRule) {
        updateMutation.mutate({ ruleId: editingRule.id, data });
      } else {
        createMutation.mutate(data);
      }
    },
    [editingRule, createMutation, updateMutation],
  );

  const handleDelete = useCallback(
    (ruleId: string) => {
      if (window.confirm("Delete this automation rule?")) {
        deleteMutation.mutate(ruleId);
      }
    },
    [deleteMutation],
  );

  // Build rule name map for history display.
  const ruleNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const r of rules ?? []) {
      map[r.id] = r.name;
    }
    return map;
  }, [rules]);

  // ---- Render ----
  return (
    <div className="space-y-6 max-w-[720px]">
      {/* Active Rules */}
      <GlassCard intensity="subtle" className="p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-xl font-semibold">Automation Rules</h2>
            <p className="mt-1 text-[13px] text-muted-foreground">
              Automate actions when events occur in this project
            </p>
          </div>
          <Button size="sm" onClick={handleNewRule}>
            New Rule
          </Button>
        </div>

        {isLoading && (
          <div className="py-8 text-center text-sm text-muted-foreground">
            Loading rules...
          </div>
        )}

        {error && (
          <div className="py-8 text-center text-sm text-destructive">
            Failed to load rules: {(error as Error).message}
          </div>
        )}

        {rules && rules.length === 0 && (
          <div className="py-8 text-center text-sm text-muted-foreground">
            No automation rules yet. Create one to get started.
          </div>
        )}

        {rules && rules.length > 0 && (
          <div className="space-y-2">
            {rules.map((rule) => (
              <div
                key={rule.id}
                className="flex items-center gap-4 rounded-md border px-4 py-3"
              >
                <Switch
                  checked={rule.enabled}
                  onCheckedChange={() => toggleMutation.mutate(rule.id)}
                  disabled={toggleMutation.isPending}
                />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium truncate">{rule.name}</span>
                    {rule.builtin && (
                      <Badge variant="secondary">built-in</Badge>
                    )}
                  </div>
                  <div className="text-xs text-muted-foreground mt-0.5">
                    Trigger: {rule.trigger}
                    {rule.actions.length > 0 && (
                      <span className="ml-2">
                        &middot; {rule.actions.length} action{rule.actions.length !== 1 ? "s" : ""}
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleEditRule(rule)}
                  >
                    Edit
                  </Button>
                  {!rule.builtin && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleDelete(rule.id)}
                      disabled={deleteMutation.isPending}
                      className="text-destructive hover:text-destructive"
                    >
                      Delete
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </GlassCard>

      {/* Execution History */}
      <GlassCard intensity="subtle" className="p-6">
        <div className="mb-4">
          <h2 className="text-xl font-semibold">Execution History</h2>
          <p className="mt-1 text-[13px] text-muted-foreground">
            Recent automation executions
          </p>
        </div>
        <AutomationHistory
          workspaceSlug={workspaceSlug}
          projectId={projectId}
          ruleNames={ruleNames}
        />
      </GlassCard>

      {/* Rule editor dialog */}
      <AutomationRuleEditor
        open={editorOpen}
        onOpenChange={(open) => {
          setEditorOpen(open);
          if (!open) setEditingRule(undefined);
        }}
        workspaceSlug={workspaceSlug}
        projectId={projectId}
        rule={editingRule}
        onSave={handleSave}
        saving={createMutation.isPending || updateMutation.isPending}
      />
    </div>
  );
}
