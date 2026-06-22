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
import { t } from "@neokapi/kapi-react/runtime";
import type { ProviderConfig, AIModelOption, DefaultModelInfo } from "../types/api";
import { api } from "../hooks/useApi";
import { AIModelList } from "./AIModelList";
import { useError } from "./ErrorBanner";

interface ProviderTypeOption {
  name: string;
  label: string;
  /** On-device providers (Ollama, Gemma, Demo) need no API key. */
  local?: boolean;
}

/** Models + saved keys for one provider, the unit the page is grouped by. */
interface ProviderGroup {
  provider: string;
  label: string;
  local: boolean;
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
}

export function CredentialsPage({
  providers: propProviders,
  providerTypes: propProviderTypes,
  models: propModels,
}: CredentialsPageProps = {}) {
  const [providers, setProviders] = useState<ProviderConfig[]>(propProviders ?? []);
  const [providerTypes, setProviderTypes] = useState<ProviderTypeOption[]>(propProviderTypes ?? []);
  const [models, setModels] = useState<AIModelOption[]>(propModels ?? []);
  const [defaultModel, setDefaultModel] = useState<DefaultModelInfo>({ provider: "", model: "" });
  const [loading, setLoading] = useState(!propProviders);
  const [editing, setEditing] = useState<ProviderConfig | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [testResult, setTestResult] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);

  const { showError } = useError();

  const load = useCallback(async () => {
    if (propProviders) return;
    try {
      const [result, types, modelList, def] = await Promise.all([
        api.listProviders(),
        api.listProviderTypes(),
        api.listAIModels(),
        api.getDefaultModel(),
      ]);
      if (result) setProviders(result);
      if (types) setProviderTypes(types);
      if (modelList) setModels(modelList);
      if (def) setDefaultModel(def);
    } catch (err) {
      showError("Failed to load AI models", err);
    } finally {
      setLoading(false);
    }
  }, [showError, propProviders]);

  useEffect(() => {
    void load();
  }, [load]);

  // One group per provider (in catalog order: local first, then cloud), each
  // carrying that provider's models and its saved keys — so a credential is
  // shown with the provider it belongs to rather than in a separate list.
  const groups = useMemo<ProviderGroup[]>(() => {
    const byProvider = new Map<string, ProviderGroup>();
    for (const m of models) {
      let g = byProvider.get(m.provider);
      if (!g) {
        g = { provider: m.provider, label: m.label, local: m.local, models: [], creds: [] };
        byProvider.set(m.provider, g);
      }
      g.models.push(m);
    }
    for (const c of providers) {
      const g = byProvider.get(c.provider_type);
      if (g) g.creds.push(c);
    }
    return [...byProvider.values()];
  }, [models, providers]);

  // Choosing a model persists the shared default (ai.provider/ai.model); the
  // provider follows from the model.
  const handleSelectModel = async (m: AIModelOption) => {
    setDefaultModel({ provider: m.provider, model: m.model });
    try {
      await api.setDefaultModel(m.model, m.provider);
      await load();
    } catch (e) {
      setError(String(e));
      void load();
    }
  };

  // Mark a key as the default for its provider (when several are saved).
  const handleSetKeyDefault = async (id: string) => {
    try {
      await api.setProviderDefault(id);
      await load();
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
      await load();
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
      await load();
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
  const editingIsLocal = !!editingType?.local;

  return (
    <div className="p-6">
      <PageHeader
        title="AI Models"
        subtitle="Pick the default model for translation and QA — the provider follows from the model. API keys are stored in your OS keychain."
      />

      {error && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          {error}
        </p>
      )}

      {loading ? (
        <LoadingSpinner text="Loading AI models..." className="py-8" />
      ) : (
        <div className="space-y-6">
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
                </div>

                {!g.local && (
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
                <Badge variant="secondary">{t("Runs on-device — no API key needed")}</Badge>
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
