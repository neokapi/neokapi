import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Switch,
  useSetBreadcrumb,
} from "@neokapi/ui";
import { AlertTriangle, Check, Star } from "lucide-react";
import {
  getPlatformConfig,
  updatePlatformConfig,
  type CatalogModel,
  type PlatformConfigResponse,
  type PlatformConfigUpdate,
} from "../api";

const PLANS = ["free", "pro", "team", "enterprise"] as const;

// Known gated features (mirrors billing.Feature). An instance-wide flag layers
// OVER the plan matrix but UNDER per-workspace overrides.
const KNOWN_FEATURES: { key: string; label: string; hint: string }[] = [
  { key: "bravo", label: "@bravo agent", hint: "The @bravo chat agent surface (dark by default)." },
  { key: "bravo-code-exec", label: "@bravo code execution", hint: "Lets @bravo run code." },
  { key: "connectors-git", label: "Git connector", hint: "Source content from a git repository." },
  { key: "connectors-custom", label: "Custom connectors", hint: "Customer-defined connectors." },
  { key: "api-access", label: "API access", hint: "Programmatic API tokens." },
  { key: "sso-saml", label: "SSO / SAML", hint: "Enterprise single sign-on." },
];

type FeatureState = "default" | "on" | "off";

function tierVariant(tier: string): "default" | "secondary" | "outline" {
  if (tier === "flagship") return "default";
  if (tier === "balanced") return "secondary";
  return "outline";
}

/**
 * useSectionState resets local editing state whenever the server snapshot
 * changes (after a save or a background refetch), so the form always tracks the
 * source of truth without stomping an in-progress edit mid-render.
 */
function useSectionState<T>(
  deps: unknown,
  init: () => T,
): [T, React.Dispatch<React.SetStateAction<T>>] {
  const [state, setState] = useState<T>(init);
  useEffect(() => {
    setState(init());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deps]);
  return [state, setState];
}

export function PlatformRoute() {
  useSetBreadcrumb("Platform");
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "platform"],
    queryFn: getPlatformConfig,
  });

  if (isLoading || !data) {
    return (
      <div className="mx-auto w-full max-w-3xl">
        <p className="text-sm text-muted-foreground">Loading platform settings…</p>
      </div>
    );
  }

  return (
    <PlatformSettings
      data={data}
      onSaved={() => queryClient.invalidateQueries({ queryKey: ["admin", "platform"] })}
    />
  );
}

function PlatformSettings({
  data,
  onSaved,
}: {
  data: PlatformConfigResponse;
  onSaved: () => void;
}) {
  const { snapshot, persistent } = data;
  const readOnly = !persistent;

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <div className="flex flex-col gap-1">
        <p className="text-sm text-muted-foreground">
          Instance-wide runtime settings. Changes apply to every server and worker within seconds,
          with no redeploy required.
        </p>
      </div>

      {readOnly && (
        <Alert>
          <AlertTriangle />
          <AlertTitle>Read-only</AlertTitle>
          <AlertDescription>
            This instance has no database configured, so it serves the environment bootstrap
            defaults and cannot persist changes. Settings below are shown for reference only.
          </AlertDescription>
        </Alert>
      )}

      <AICard snapshot={snapshot} readOnly={readOnly} onSaved={onSaved} />
      <ModelSweepsCard snapshot={snapshot} readOnly={readOnly} onSaved={onSaved} />
      <SignupsCard snapshot={snapshot} readOnly={readOnly} onSaved={onSaved} />
      <MaintenanceCard snapshot={snapshot} readOnly={readOnly} onSaved={onSaved} />
      <WorkspaceDefaultsCard snapshot={snapshot} readOnly={readOnly} onSaved={onSaved} />
      <FeaturesCard snapshot={snapshot} readOnly={readOnly} onSaved={onSaved} />
    </div>
  );
}

// useSave centralizes the PUT mutation + cache invalidation for a section.
function useSave(onSaved: () => void) {
  return useMutation({
    mutationFn: (update: PlatformConfigUpdate) => updatePlatformConfig(update),
    onSuccess: () => onSaved(),
  });
}

function SaveError({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <p className="text-sm text-destructive">
      {error instanceof Error ? error.message : "Failed to save"}
    </p>
  );
}

// ---------------------------------------------------------------------------
// AI provider & models
// ---------------------------------------------------------------------------

function AICard({
  snapshot,
  readOnly,
  onSaved,
}: {
  snapshot: PlatformConfigResponse["snapshot"];
  readOnly: boolean;
  onSaved: () => void;
}) {
  const save = useSave(onSaved);
  const ai = snapshot.ai;

  const [provider, setProvider] = useSectionState(ai.provider, () => ai.provider);
  const [baseURL, setBaseURL] = useSectionState(ai.base_url, () => ai.base_url ?? "");
  const [customerChoice, setCustomerChoice] = useSectionState(
    ai.customer_choice,
    () => ai.customer_choice,
  );
  const [enabled, setEnabled] = useSectionState(
    ai.enabled_models,
    () => new Set(ai.enabled_models),
  );
  const [defaultModel, setDefaultModel] = useSectionState(ai.default_model, () => ai.default_model);
  const [customId, setCustomId] = useState("");

  // Merge the catalog with any enabled ids not present in it (custom models), so
  // both onboard/offboard from the static catalog and free-text ids are managed
  // in one list.
  const rows: CatalogModel[] = useMemo(() => {
    const byId = new Map<string, CatalogModel>();
    for (const m of snapshot.model_catalog) byId.set(m.id, m);
    const extras: CatalogModel[] = [];
    for (const id of ai.enabled_models) {
      if (!byId.has(id)) extras.push({ id, name: id, tier: "balanced" });
    }
    return [...snapshot.model_catalog, ...extras];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [snapshot.model_catalog, ai.enabled_models]);

  const toggleEnabled = (id: string, on: boolean) => {
    setEnabled((prev) => {
      const next = new Set(prev);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
    // If the current default was just disabled, clear it.
    if (!on && defaultModel === id) setDefaultModel("");
  };

  const addCustom = () => {
    const id = customId.trim();
    if (!id) return;
    setEnabled((prev) => new Set(prev).add(id));
    setCustomId("");
  };

  const enabledList = [...enabled];
  const defaultInvalid = !enabledList.includes(defaultModel);
  const canSave = !readOnly && !save.isPending && enabledList.length > 0 && !defaultInvalid;

  const onSave = () => {
    const update: PlatformConfigUpdate = {
      ai: {
        provider: provider.trim(),
        base_url: baseURL.trim(),
        customer_choice: customerChoice,
        enabled_models: enabledList,
        default_model: defaultModel,
      },
    };
    save.mutate(update);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>AI provider &amp; models</CardTitle>
        <CardDescription>
          The platform translation backend. Onboard the models you want available, pick the default,
          and optionally let workspaces choose their own from the enabled set.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="flex flex-col gap-2">
            <Label htmlFor="ai-provider">Provider</Label>
            <Input
              id="ai-provider"
              value={provider}
              onChange={(e) => setProvider(e.target.value)}
              disabled={readOnly}
              placeholder="bedrock"
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="ai-base-url">Base URL (optional)</Label>
            <Input
              id="ai-base-url"
              value={baseURL}
              onChange={(e) => setBaseURL(e.target.value)}
              disabled={readOnly}
              placeholder="provider default"
            />
          </div>
        </div>

        <Separator />

        <div className="flex flex-col gap-3">
          <div className="flex items-center justify-between">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium">Model catalog</span>
              <span className="text-xs text-muted-foreground">
                Toggle a model on to onboard it. The starred model is the platform default.
              </span>
            </div>
          </div>

          <div className="flex flex-col divide-y rounded-lg border">
            {rows.map((m) => {
              const isOn = enabled.has(m.id);
              const isDefault = defaultModel === m.id;
              return (
                <div key={m.id} className="flex items-center gap-3 px-3 py-2.5">
                  <Switch
                    checked={isOn}
                    onCheckedChange={(v) => toggleEnabled(m.id, v)}
                    disabled={readOnly}
                    aria-label={`Enable ${m.name}`}
                  />
                  <div className="flex min-w-0 flex-1 flex-col">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">{m.name}</span>
                      <Badge variant={tierVariant(m.tier)} className="capitalize">
                        {m.tier}
                      </Badge>
                      {m.requires_marketplace && (
                        <Badge variant="outline" className="text-amber-600">
                          Marketplace
                        </Badge>
                      )}
                    </div>
                    <span className="truncate font-mono text-xs text-muted-foreground">{m.id}</span>
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant={isDefault ? "default" : "outline"}
                    disabled={readOnly || !isOn}
                    onClick={() => setDefaultModel(m.id)}
                  >
                    {isDefault ? (
                      <Check data-icon="inline-start" />
                    ) : (
                      <Star data-icon="inline-start" />
                    )}
                    {isDefault ? "Default" : "Set default"}
                  </Button>
                </div>
              );
            })}
          </div>

          <div className="flex items-end gap-2">
            <div className="flex flex-1 flex-col gap-2">
              <Label htmlFor="ai-custom-id">Add a custom model id</Label>
              <Input
                id="ai-custom-id"
                value={customId}
                onChange={(e) => setCustomId(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addCustom();
                  }
                }}
                disabled={readOnly}
                placeholder="e.g. eu.anthropic.claude-…"
              />
            </div>
            <Button
              type="button"
              variant="outline"
              onClick={addCustom}
              disabled={readOnly || !customId.trim()}
            >
              Add
            </Button>
          </div>

          {defaultInvalid && enabledList.length > 0 && (
            <p className="text-sm text-destructive">Pick a default model from the enabled set.</p>
          )}
          {enabledList.length === 0 && (
            <p className="text-sm text-destructive">At least one model must be enabled.</p>
          )}
        </div>

        <Separator />

        <div className="flex items-center justify-between gap-4">
          <div className="flex flex-col gap-0.5">
            <span className="text-sm font-medium">Let workspaces choose their model</span>
            <span className="text-xs text-muted-foreground">
              When on, workspaces may pick any enabled model; when off, everyone uses the default.
            </span>
          </div>
          <Switch
            checked={customerChoice}
            onCheckedChange={setCustomerChoice}
            disabled={readOnly}
            aria-label="Customer model choice"
          />
        </div>

        <SaveError error={save.error} />
      </CardContent>
      <CardFooter className="justify-end">
        <Button onClick={onSave} disabled={!canSave}>
          Save AI settings
        </Button>
      </CardFooter>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Model sweeps (measured steerability)
// ---------------------------------------------------------------------------

function ModelSweepsCard({
  snapshot,
  readOnly,
  onSaved,
}: {
  snapshot: PlatformConfigResponse["snapshot"];
  readOnly: boolean;
  onSaved: () => void;
}) {
  const save = useSave(onSaved);
  const [enabled, setEnabled] = useSectionState(
    snapshot.model_sweeps.enabled,
    () => snapshot.model_sweeps.enabled,
  );
  const dirty = enabled !== snapshot.model_sweeps.enabled;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Model sweeps</CardTitle>
        <CardDescription>
          Measured steerability: periodically translate a small fixture set from each project's own
          content, with and without its brand context, on every enabled model, and recommend the
          model with the highest measured lift. Sweeps run on the platform provider and never deduct
          customer credits; the recommendation also steers platform model resolution for workspaces
          without a pinned model.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">Sweeps enabled</span>
            <Badge variant={enabled ? "default" : "secondary"}>{enabled ? "On" : "Off"}</Badge>
          </div>
          <Switch
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={readOnly}
            aria-label="Model sweeps enabled"
          />
        </div>
        <SaveError error={save.error} />
      </CardContent>
      <CardFooter className="justify-end">
        <Button
          onClick={() => save.mutate({ model_sweeps: { enabled } })}
          disabled={readOnly || save.isPending || !dirty}
        >
          Save
        </Button>
      </CardFooter>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Signups
// ---------------------------------------------------------------------------

function SignupsCard({
  snapshot,
  readOnly,
  onSaved,
}: {
  snapshot: PlatformConfigResponse["snapshot"];
  readOnly: boolean;
  onSaved: () => void;
}) {
  const save = useSave(onSaved);
  const [open, setOpen] = useSectionState(snapshot.signups.open, () => snapshot.signups.open);
  const dirty = open !== snapshot.signups.open;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Signups</CardTitle>
        <CardDescription>
          When closed, new users cannot finish onboarding or create a self-serve workspace. Existing
          accounts are unaffected.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">Signups open</span>
            <Badge variant={open ? "default" : "secondary"}>{open ? "Open" : "Closed"}</Badge>
          </div>
          <Switch
            checked={open}
            onCheckedChange={setOpen}
            disabled={readOnly}
            aria-label="Signups open"
          />
        </div>
        <SaveError error={save.error} />
      </CardContent>
      <CardFooter className="justify-end">
        <Button
          onClick={() => save.mutate({ signups: { open } })}
          disabled={readOnly || save.isPending || !dirty}
        >
          Save
        </Button>
      </CardFooter>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Maintenance
// ---------------------------------------------------------------------------

function MaintenanceCard({
  snapshot,
  readOnly,
  onSaved,
}: {
  snapshot: PlatformConfigResponse["snapshot"];
  readOnly: boolean;
  onSaved: () => void;
}) {
  const save = useSave(onSaved);
  const [enabled, setEnabled] = useSectionState(
    snapshot.maintenance.enabled,
    () => snapshot.maintenance.enabled,
  );
  const [message, setMessage] = useSectionState(
    snapshot.maintenance.message,
    () => snapshot.maintenance.message ?? "",
  );
  const dirty =
    enabled !== snapshot.maintenance.enabled || message !== (snapshot.maintenance.message ?? "");

  return (
    <Card>
      <CardHeader>
        <CardTitle>Maintenance banner</CardTitle>
        <CardDescription>
          Show an instance-wide banner to every user, e.g. to announce scheduled work.
          Informational; it does not block access.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center justify-between gap-4">
          <span className="text-sm font-medium">Banner active</span>
          <Switch
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={readOnly}
            aria-label="Maintenance banner active"
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="maintenance-message">Message</Label>
          <Input
            id="maintenance-message"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            disabled={readOnly}
            placeholder="Scheduled maintenance at 22:00 UTC…"
          />
        </div>
        <SaveError error={save.error} />
      </CardContent>
      <CardFooter className="justify-end">
        <Button
          onClick={() => save.mutate({ maintenance: { enabled, message } })}
          disabled={readOnly || save.isPending || !dirty}
        >
          Save
        </Button>
      </CardFooter>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// New-workspace defaults
// ---------------------------------------------------------------------------

function WorkspaceDefaultsCard({
  snapshot,
  readOnly,
  onSaved,
}: {
  snapshot: PlatformConfigResponse["snapshot"];
  readOnly: boolean;
  onSaved: () => void;
}) {
  const save = useSave(onSaved);
  const wd = snapshot.workspace_defaults;
  const [plan, setPlan] = useSectionState(wd.plan, () => wd.plan);
  const [trialDays, setTrialDays] = useSectionState(wd.trial_days, () => String(wd.trial_days));

  const trialNum = Number(trialDays);
  const trialValid = Number.isInteger(trialNum) && trialNum > 0 && trialNum <= 365;
  const dirty = plan !== wd.plan || trialNum !== wd.trial_days;

  return (
    <Card>
      <CardHeader>
        <CardTitle>New-workspace defaults</CardTitle>
        <CardDescription>
          The plan and trial length applied when a new workspace is created.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-2">
          <Label htmlFor="default-plan">Default plan</Label>
          <Select value={plan} onValueChange={setPlan} disabled={readOnly}>
            <SelectTrigger id="default-plan">
              <SelectValue placeholder="Select plan" />
            </SelectTrigger>
            <SelectContent>
              {PLANS.map((p) => (
                <SelectItem key={p} value={p} className="capitalize">
                  {p}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="trial-days">Trial length (days)</Label>
          <Input
            id="trial-days"
            type="number"
            min={1}
            max={365}
            value={trialDays}
            onChange={(e) => setTrialDays(e.target.value)}
            disabled={readOnly}
            aria-invalid={!trialValid}
          />
          {!trialValid && <p className="text-sm text-destructive">Enter 1–365 days.</p>}
        </div>
        <div className="sm:col-span-2">
          <SaveError error={save.error} />
        </div>
      </CardContent>
      <CardFooter className="justify-end">
        <Button
          onClick={() => save.mutate({ workspace_defaults: { plan, trial_days: trialNum } })}
          disabled={readOnly || save.isPending || !dirty || !trialValid}
        >
          Save
        </Button>
      </CardFooter>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Global feature flags
// ---------------------------------------------------------------------------

function featureStateOf(features: Record<string, boolean>, key: string): FeatureState {
  if (!(key in features)) return "default";
  return features[key] ? "on" : "off";
}

function FeaturesCard({
  snapshot,
  readOnly,
  onSaved,
}: {
  snapshot: PlatformConfigResponse["snapshot"];
  readOnly: boolean;
  onSaved: () => void;
}) {
  const save = useSave(onSaved);
  const [states, setStates] = useSectionState(snapshot.features, () => {
    const m: Record<string, FeatureState> = {};
    for (const f of KNOWN_FEATURES) m[f.key] = featureStateOf(snapshot.features, f.key);
    return m;
  });

  const setFeature = (key: string, v: FeatureState) => setStates((prev) => ({ ...prev, [key]: v }));

  const dirty = KNOWN_FEATURES.some(
    (f) => states[f.key] !== featureStateOf(snapshot.features, f.key),
  );

  const onSave = () => {
    // "default" removes the key (falls through to the plan matrix); on/off write a
    // boolean. The map fully replaces the stored feature flags.
    const features: Record<string, boolean> = {};
    for (const f of KNOWN_FEATURES) {
      const s = states[f.key];
      if (s === "on") features[f.key] = true;
      else if (s === "off") features[f.key] = false;
    }
    save.mutate({ features });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Global feature flags</CardTitle>
        <CardDescription>
          Instance-wide overrides that layer over each plan&apos;s entitlements. A per-workspace
          override still wins over these. Leave a feature on{" "}
          <span className="font-medium">Default</span> to defer to the plan matrix.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {KNOWN_FEATURES.map((f) => (
          <div key={f.key} className="flex items-center justify-between gap-4">
            <div className="flex min-w-0 flex-col gap-0.5">
              <span className="text-sm font-medium">{f.label}</span>
              <span className="truncate text-xs text-muted-foreground">{f.hint}</span>
            </div>
            <Select
              value={states[f.key]}
              onValueChange={(v) => setFeature(f.key, v as FeatureState)}
              disabled={readOnly}
            >
              <SelectTrigger className="w-32 shrink-0">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="default">Default</SelectItem>
                <SelectItem value="on">Enabled</SelectItem>
                <SelectItem value="off">Disabled</SelectItem>
              </SelectContent>
            </Select>
          </div>
        ))}
        <SaveError error={save.error} />
      </CardContent>
      <CardFooter className="justify-end">
        <Button onClick={onSave} disabled={readOnly || save.isPending || !dirty}>
          Save feature flags
        </Button>
      </CardFooter>
    </Card>
  );
}
