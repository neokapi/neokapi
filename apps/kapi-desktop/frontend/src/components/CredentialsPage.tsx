import { useState, useEffect, useCallback, useMemo } from "react";
import {
  Plus,
  Trash2,
  TestTube,
  KeyRound,
  Loader2,
  CheckCircle2,
  Cpu,
  Cloud,
  Sparkles,
  Star,
} from "lucide-react";
import {
  Button,
  Badge,
  Label,
  Input,
  PageHeader,
  LoadingSpinner,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  cn,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  ProviderConfig,
  AIModelOption,
  DefaultModelInfo,
  AIDetectionResult,
} from "../types/api";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { ActiveModelBadge } from "./ActiveModelBadge";
import { AIModelList } from "./AIModelList";
import { useError } from "./ErrorBanner";

interface ProviderTypeOption {
  name: string;
  label: string;
  /** On-device providers (Ollama, Gemma, Demo) need no API key. */
  local?: boolean;
  /** Needs no API key at all — local, or subscription-backed (claude-code). */
  keyless?: boolean;
  /** Bills a personal subscription (claude-code) instead of metered usage. */
  subscription?: boolean;
}

/** Models + saved keys for one provider, the unit the page is grouped by. */
interface ProviderGroup {
  provider: string;
  label: string;
  local: boolean;
  keyless: boolean;
  subscription: boolean;
  models: AIModelOption[];
  creds: ProviderConfig[];
}

export interface CredentialsPageProps {
  /** Pre-loaded providers for Storybook — skips api.listProviders(). */
  providers?: ProviderConfig[];
  /** Pre-loaded provider types for Storybook. */
  providerTypes?: ProviderTypeOption[];
  /** Pre-loaded model catalog for Storybook — skips api.listAIModels(). */
  models?: AIModelOption[];
  /** Pre-loaded machine detection for Storybook — skips api.detectAIProviders(). */
  detection?: AIDetectionResult;
}

export function CredentialsPage({
  providers: propProviders,
  providerTypes: propProviderTypes,
  models: propModels,
  detection: propDetection,
}: CredentialsPageProps = {}) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<ProviderConfig | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [testResult, setTestResult] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);
  // Optimistic default-model selection; cleared once the server confirms.
  const [defaultOverride, setDefaultOverride] = useState<DefaultModelInfo | null>(null);

  const { showError } = useError();

  // All AI-model data (providers, types, catalog, default, machine detection)
  // loads in a single query fn — the Wails bindings are the source, react-query
  // owns caching. Mutations invalidate this key via reload().
  const dataQuery = useQuery({
    queryKey: qk.aiModelData(),
    enabled: !propProviders,
    queryFn: async () => {
      const [result, types, modelList, def, det] = await Promise.all([
        api.listProviders(),
        api.listProviderTypes(),
        api.listAIModels(),
        api.getDefaultModel(),
        propDetection ? Promise.resolve(null) : api.detectAIProviders(),
      ]);
      return {
        providers: result ?? [],
        providerTypes: types ?? [],
        models: modelList ?? [],
        defaultModel: def ?? { provider: "", model: "" },
        detection: det ?? null,
      };
    },
  });

  const providers: ProviderConfig[] = propProviders ?? dataQuery.data?.providers ?? [];
  const providerTypes: ProviderTypeOption[] =
    propProviderTypes ?? dataQuery.data?.providerTypes ?? [];
  const models: AIModelOption[] = propModels ?? dataQuery.data?.models ?? [];
  const detection: AIDetectionResult | null = propDetection ?? dataQuery.data?.detection ?? null;
  const defaultModel: DefaultModelInfo = defaultOverride ??
    dataQuery.data?.defaultModel ?? { provider: "", model: "" };
  const loading = !propProviders && dataQuery.isLoading;

  useEffect(() => {
    if (dataQuery.error) showError("Failed to load AI models", dataQuery.error);
  }, [dataQuery.error, showError]);

  // Clear the optimistic default once fresh server data arrives.
  useEffect(() => {
    setDefaultOverride(null);
  }, [dataQuery.data]);

  const load = useCallback(() => {
    void qc.invalidateQueries({ queryKey: qk.aiModelData() });
    // Also refresh the raw providers list the schema-form credential picker reads.
    void qc.invalidateQueries({ queryKey: qk.providers() });
  }, [qc]);

  // One group per provider (in catalog order: local first, then cloud), each
  // carrying that provider's models and its saved keys — so a credential is
  // shown with the provider it belongs to rather than in a separate list.
  const groups = useMemo<ProviderGroup[]>(() => {
    const typeByName = new Map(providerTypes.map((pt) => [pt.name, pt]));
    const byProvider = new Map<string, ProviderGroup>();
    for (const m of models) {
      let g = byProvider.get(m.provider);
      if (!g) {
        const pt = typeByName.get(m.provider);
        g = {
          provider: m.provider,
          label: m.label,
          local: m.local,
          keyless: !!(pt?.keyless ?? (m.local || m.subscription)),
          subscription: !!(pt?.subscription ?? m.subscription),
          models: [],
          creds: [],
        };
        byProvider.set(m.provider, g);
      }
      g.models.push(m);
    }
    for (const c of providers) {
      const g = byProvider.get(c.provider_type);
      if (g) g.creds.push(c);
    }
    return [...byProvider.values()];
  }, [models, providers, providerTypes]);

  // One-click select of a detected keyless provider (Claude Code, Ollama).
  const handleSelectDetected = async (provider: string, model: string) => {
    setError(null);
    try {
      await api.selectAIProvider(provider, model);
      setDefaultOverride({ provider, model });
      load();
    } catch (e) {
      setError(String(e));
    }
  };

  // Choosing a model persists the shared default (ai.provider/ai.model); the
  // provider follows from the model.
  const handleSelectModel = async (m: AIModelOption) => {
    setDefaultOverride({ provider: m.provider, model: m.model });
    try {
      await api.setDefaultModel(m.model, m.provider);
      load();
    } catch (e) {
      setError(String(e));
      load();
    }
  };

  // Mark a key as the default for its provider (when several are saved).
  const handleSetKeyDefault = async (id: string) => {
    try {
      await api.setProviderDefault(id);
      load();
    } catch (e) {
      setError(String(e));
    }
  };

  // Add a key for a specific provider (from its group header).
  const handleAddKey = (providerType: string, label: string) => {
    setEditing({ id: "", name: `${label} key`, provider_type: providerType });
    setApiKey("");
    setError(null);
  };

  const handleSave = async () => {
    if (!editing) return;
    setSaving(true);
    setError(null);
    try {
      await api.saveProvider({ ...editing, api_key: apiKey });
      setEditing(null);
      setApiKey("");
      load();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    setError(null);
    try {
      await api.deleteProvider(id);
      load();
    } catch (e) {
      setError(String(e));
    }
  };

  const handleTest = async (id: string) => {
    try {
      const result = await api.testProvider(id);
      setTestResult((prev) => ({ ...prev, [id]: !!result }));
    } catch {
      setTestResult((prev) => ({ ...prev, [id]: false }));
    }
  };

  // The provider being edited (fixed by the group the form was opened from).
  const editingType = editing
    ? providerTypes.find((pt) => pt.name === editing.provider_type)
    : undefined;
  const editingLabel = editingType?.label ?? editing?.provider_type ?? "";
  const editingIsLocal = !!(editingType?.keyless ?? editingType?.local);

  return (
    <div className="p-6">
      <PageHeader
        title="AI Models"
        subtitle="Pick the default model for translation and checks. The provider follows from the model. API keys are stored in your OS keychain."
      />

      {/* What a run will actually use, resolved by the same shared precedence
          the CLI applies — so this page states the outcome, not just the keys
          it edits. */}
      <div className="mb-4 flex items-center gap-2 text-xs text-muted-foreground">
        <span>{t("Active for runs")}</span>
        <ActiveModelBadge />
      </div>

      {error && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          {error}
        </p>
      )}

      {loading ? (
        <LoadingSpinner text="Loading AI models..." className="py-8" />
      ) : (
        <div className="space-y-6">
          {/* Detected — keyless providers found on this machine, one click to select. */}
          {(detection?.detected?.length ?? 0) > 0 && (
            <section data-testid="detected-providers">
              <div className="mb-2 flex items-center gap-2 border-b border-border pb-2">
                <Sparkles size={16} className="text-primary" />
                <h2 className="text-sm font-semibold">{t("Detected on this machine")}</h2>
                <Badge variant="secondary">{t("no API key needed")}</Badge>
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {(detection?.detected ?? []).map((d) => {
                  const isCurrent = defaultModel.provider === d.provider;
                  return (
                    <div
                      key={d.provider}
                      data-testid={`detected-${d.provider}`}
                      className={cn(
                        "flex items-center justify-between gap-3 rounded-lg border p-3",
                        isCurrent ? "border-primary bg-primary/10" : "border-border",
                      )}
                    >
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 text-sm font-medium">
                          <span translate="no">{d.label}</span>
                          {isCurrent && <CheckCircle2 size={13} className="text-primary" />}
                        </div>
                        <p className="mt-0.5 text-xs text-muted-foreground">{d.detail}</p>
                      </div>
                      <Button
                        size="sm"
                        variant={isCurrent ? "secondary" : "default"}
                        disabled={isCurrent}
                        onClick={() => void handleSelectDetected(d.provider, d.model)}
                        aria-label={t("Use {name} as the default AI provider", { name: d.label })}
                      >
                        {isCurrent ? t("Selected") : t("Select")}
                      </Button>
                    </div>
                  );
                })}
              </div>
            </section>
          )}

          {groups.map((g) => (
            <section key={g.provider}>
              {/* Provider header — label + key status/management for this provider */}
              <div className="mb-2 flex items-center justify-between gap-2 border-b border-border pb-2">
                <div className="flex items-center gap-2">
                  {g.local ? (
                    <Cpu size={16} className="text-primary" />
                  ) : (
                    <Cloud size={16} className="text-muted-foreground" />
                  )}
                  <h2 className="text-sm font-semibold" translate="no">
                    {g.label}
                  </h2>
                  {g.local && <Badge variant="secondary">{t("on-device")}</Badge>}
                  {g.subscription && (
                    <Badge variant="secondary">{t("uses your Claude subscription")}</Badge>
                  )}
                </div>

                {!g.keyless && (
                  <div className="flex flex-wrap items-center gap-2">
                    {g.creds.map((c) => {
                      // With several keys the chip is a default-selector (star);
                      // a lone key is implicitly the one used, so it's a plain chip.
                      const multiple = g.creds.length > 1;
                      return (
                        <span key={c.id} className="flex items-center gap-0.5">
                          {multiple ? (
                            <button
                              type="button"
                              onClick={() => void handleSetKeyDefault(c.id)}
                              aria-pressed={!!c.default}
                              aria-label={
                                c.default
                                  ? t("{name} is the default key", { name: c.name })
                                  : t("Use {name} as the default key", { name: c.name })
                              }
                              className={cn(
                                "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors",
                                c.default
                                  ? "border-primary bg-primary/10 text-primary"
                                  : "border-border text-muted-foreground hover:border-primary/40",
                              )}
                            >
                              <Star size={11} className={c.default ? "fill-current" : ""} />
                              {c.name}
                            </button>
                          ) : (
                            <Badge variant="outline" className="gap-1">
                              <KeyRound size={10} />
                              {c.name}
                            </Badge>
                          )}
                          {testResult[c.id] && (
                            <CheckCircle2 size={12} className="text-green-500" />
                          )}
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => handleTest(c.id)}
                            aria-label={t("Test connection for {name}", { name: c.name })}
                          >
                            <TestTube size={13} />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => handleDelete(c.id)}
                            className="hover:bg-destructive/10 hover:text-destructive"
                            aria-label={t("Delete {name}", { name: c.name })}
                          >
                            <Trash2 size={13} />
                          </Button>
                        </span>
                      );
                    })}
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleAddKey(g.provider, g.label)}
                      aria-label={t("Add credentials for {name}", { name: g.label })}
                    >
                      <Plus size={12} />
                      {t("Add Credentials")}
                    </Button>
                  </div>
                )}
              </div>

              {/* Models for this provider — model-first selectable rows */}
              <AIModelList
                models={g.models}
                selected={defaultModel.model ? defaultModel : undefined}
                showProvider={false}
                onSelect={(m) => void handleSelectModel(m)}
              />
            </section>
          ))}
        </div>
      )}

      <Dialog
        open={editing !== null}
        onOpenChange={(o) => {
          if (!o) {
            setEditing(null);
            setApiKey("");
            setError(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editing?.id
                ? t("Edit {provider} credentials", { provider: editingLabel })
                : t("Add {provider} credentials", { provider: editingLabel })}
            </DialogTitle>
          </DialogHeader>
          {error && (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          )}
          {editing && (
            // The provider is fixed by the group this was opened from — no
            // provider chooser; the form is native to that provider.
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label htmlFor="cred-name" className="mb-1 block text-xs text-muted-foreground">
                    Name
                  </Label>
                  <Input
                    id="cred-name"
                    type="text"
                    value={editing.name}
                    onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                    placeholder={t("{provider} key", { provider: editingLabel })}
                  />
                </div>
                <div>
                  <Label htmlFor="cred-model" className="mb-1 block text-xs text-muted-foreground">
                    Model (optional)
                  </Label>
                  <Input
                    id="cred-model"
                    type="text"
                    value={editing.model ?? ""}
                    onChange={(e) => setEditing({ ...editing, model: e.target.value })}
                    placeholder="claude-sonnet-4-5-20241022"
                  />
                </div>
              </div>
              {editingIsLocal ? (
                <Badge variant="secondary">{t("Runs on-device, no API key needed")}</Badge>
              ) : (
                <div>
                  <Label htmlFor="cred-apikey" className="mb-1 block text-xs text-muted-foreground">
                    API Key
                  </Label>
                  <Input
                    id="cred-apikey"
                    type="password"
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    placeholder="sk-..."
                  />
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setEditing(null);
                setApiKey("");
                setError(null);
              }}
              disabled={saving}
            >
              {t("Cancel")}
            </Button>
            <Button
              onClick={handleSave}
              disabled={!editing?.name || !editing?.provider_type || saving}
            >
              {saving && <Loader2 size={12} className="animate-spin" />}
              {saving ? t("Saving...") : t("Save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
