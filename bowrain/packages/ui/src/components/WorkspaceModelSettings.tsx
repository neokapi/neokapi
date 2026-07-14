import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import { useCallback, useEffect, useState } from "react";
import { useApi } from "../context/ApiContext";
import { ErrorNotice } from "../errors";
import type { PublicPlatformConfig, Workspace } from "../types/api";

// Sentinel Select value for "use the platform default" — Radix Select cannot use
// an empty string as an item value, so we map it to/from workspace.preferred_model === "".
const PLATFORM_DEFAULT = "__platform_default__";

interface WorkspaceModelSettingsProps {
  workspace: Workspace;
  onUpdate?: (preferredModel: string) => void;
}

/**
 * WorkspaceModelSettings lets a workspace admin choose which platform AI model
 * this workspace uses, from the set the platform admin has enabled. It renders
 * nothing unless the platform has opened model choice to customers
 * (`ai_customer_choice`), so on a locked-down instance the card is simply absent.
 */
export function WorkspaceModelSettings({ workspace, onUpdate }: WorkspaceModelSettingsProps) {
  const api = useApi();
  const [config, setConfig] = useState<PublicPlatformConfig | null>(null);
  const [selected, setSelected] = useState<string>(workspace.preferred_model ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const canEdit = workspace.role === "owner" || workspace.role === "admin";

  useEffect(() => {
    let cancelled = false;
    void api
      .getPublicPlatformConfig()
      .then((c) => {
        if (!cancelled) setConfig(c);
      })
      .catch(() => {
        // A missing/unreachable config endpoint just means no picker — stay silent.
        if (!cancelled) setConfig(null);
      });
    return () => {
      cancelled = true;
    };
  }, [api]);

  useEffect(() => {
    setSelected(workspace.preferred_model ?? "");
  }, [workspace.preferred_model]);

  const save = useCallback(
    async (next: string) => {
      const prev = selected;
      setSelected(next);
      setSaving(true);
      setError(null);
      try {
        await api.updateWorkspace(workspace.slug, { preferred_model: next } as Partial<Workspace>);
        onUpdate?.(next);
      } catch (e) {
        setSelected(prev);
        setError(e);
      } finally {
        setSaving(false);
      }
    },
    [api, workspace.slug, selected, onUpdate],
  );

  // Feature off (or not yet loaded): render nothing.
  if (!config?.ai_customer_choice) return null;

  const models = config.ai_enabled_models ?? [];
  const defaultModel = models.find((m) => m.id === config.ai_default_model);
  const defaultLabel = defaultModel
    ? `Platform default (${defaultModel.name})`
    : "Platform default";

  return (
    <Card>
      <CardHeader>
        <CardTitle>AI model</CardTitle>
        <CardDescription>
          Choose the AI model used for platform-powered translation in this workspace.
          Bring-your-own providers below always use their own configured model.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <Label className="text-muted-foreground">Model</Label>
        <Select
          value={selected === "" ? PLATFORM_DEFAULT : selected}
          onValueChange={(v: string) => void save(v === PLATFORM_DEFAULT ? "" : v)}
          disabled={!canEdit || saving}
        >
          <SelectTrigger className="max-w-md">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={PLATFORM_DEFAULT}>{defaultLabel}</SelectItem>
            {models.map((m) => (
              <SelectItem key={m.id} value={m.id}>
                {m.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {!canEdit && (
          <p className="text-xs text-muted-foreground">
            Only workspace owners and admins can change the model.
          </p>
        )}
        {error != null && (
          <ErrorNotice title="Couldn't update the model" error={error} variant="inline" />
        )}
      </CardContent>
    </Card>
  );
}
