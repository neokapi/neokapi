import { useState, useEffect, useCallback } from "react";
import { Plus, Trash2, TestTube, KeyRound, Loader2, CheckCircle2 } from "lucide-react";
import {
  Button,
  Badge,
  Card,
  CardContent,
  Label,
  Input,
  PageHeader,
  EmptyState,
  LoadingSpinner,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { ProviderConfig } from "../types/api";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";

interface ProviderTypeOption {
  name: string;
  label: string;
  /** On-device providers (Ollama, Gemma, Demo) need no API key. */
  local?: boolean;
}

export interface CredentialsPageProps {
  /** Pre-loaded providers for Storybook — skips api.listProviders(). */
  providers?: ProviderConfig[];
  /** Pre-loaded provider types for Storybook. */
  providerTypes?: ProviderTypeOption[];
}

export function CredentialsPage({
  providers: propProviders,
  providerTypes: propProviderTypes,
}: CredentialsPageProps = {}) {
  const [providers, setProviders] = useState<ProviderConfig[]>(propProviders ?? []);
  const [providerTypes, setProviderTypes] = useState<ProviderTypeOption[]>(propProviderTypes ?? []);
  const [loading, setLoading] = useState(!propProviders);
  const [editing, setEditing] = useState<ProviderConfig | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [testResult, setTestResult] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);

  const { showError } = useError();

  const loadProviders = useCallback(async () => {
    if (propProviders) return;
    try {
      const [result, types] = await Promise.all([api.listProviders(), api.listProviderTypes()]);
      if (result) setProviders(result);
      if (types) setProviderTypes(types);
    } catch (err) {
      showError("Failed to load AI providers", err);
    } finally {
      setLoading(false);
    }
  }, [showError, propProviders]);

  useEffect(() => {
    void loadProviders();
  }, [loadProviders]);

  const handleAdd = () => {
    setEditing({
      id: "",
      name: "",
      provider_type: providerTypes[0]?.name ?? "anthropic",
    });
    setApiKey("");
    setError(null);
  };

  const handleSave = async () => {
    if (!editing) return;
    setSaving(true);
    setError(null);
    try {
      await api.saveProvider({
        ...editing,
        api_key: apiKey,
      });
      setEditing(null);
      setApiKey("");
      await loadProviders();
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
      await loadProviders();
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

  return (
    <div className="p-6">
      <PageHeader
        title="AI Credentials"
        subtitle="API keys are stored in your OS keychain"
        actions={
          <Button size="sm" onClick={handleAdd} aria-label="Add AI provider">
            <Plus size={12} />
            Add Provider
          </Button>
        }
      />

      {error && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          {error}
        </p>
      )}

      {loading ? (
        <LoadingSpinner text="Loading providers..." className="py-8" />
      ) : (
        <div className="space-y-2">
          {providers.map((provider) => (
            <Card key={provider.id} className="!flex-row items-center gap-3 p-4">
              <KeyRound size={18} className="shrink-0 text-primary" />
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{provider.name}</span>
                  <Badge variant="secondary">{provider.provider_type}</Badge>
                  {testResult[provider.id] && <CheckCircle2 size={14} className="text-green-500" />}
                </div>
                {provider.model && (
                  <p className="mt-0.5 text-xs text-muted-foreground">Model: {provider.model}</p>
                )}
              </div>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => handleTest(provider.id)}
                aria-label={t("Test connection for {name}", { name: provider.name })}
              >
                <TestTube size={14} />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => handleDelete(provider.id)}
                className="hover:bg-destructive/10 hover:text-destructive"
                aria-label={t("Delete {name}", { name: provider.name })}
              >
                <Trash2 size={14} />
              </Button>
            </Card>
          ))}
          {providers.length === 0 && !editing && (
            <EmptyState
              icon={<KeyRound size={24} />}
              title="No AI providers configured. Add one to use AI translation and QA tools."
            />
          )}
        </div>
      )}

      {editing && (
        <Card className="mt-4">
          <CardContent className="p-4">
            <h3 className="mb-3 text-sm font-medium">
              {editing.id ? t("Edit Provider") : t("New Provider")}
            </h3>
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
                  placeholder="My Anthropic Key"
                />
              </div>
              <div>
                <Label htmlFor="cred-type" className="mb-1 block text-xs text-muted-foreground">
                  Provider
                </Label>
                <select
                  id="cred-type"
                  value={editing.provider_type}
                  onChange={(e) => setEditing({ ...editing, provider_type: e.target.value })}
                  className="h-8 w-full rounded-lg border border-input bg-transparent px-2 py-1 text-base md:text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
                >
                  {providerTypes.map((t) => (
                    <option key={t.name} value={t.name} translate="no">
                      {t.label}
                    </option>
                  ))}
                </select>
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
              {providerTypes.find((t) => t.name === editing.provider_type)?.local ? (
                <div>
                  <Label className="mb-1 block text-xs text-muted-foreground">API Key</Label>
                  <Badge variant="secondary">{t("Runs on-device — no API key needed")}</Badge>
                </div>
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
            <div className="mt-3 flex gap-2">
              <Button
                size="sm"
                onClick={handleSave}
                disabled={!editing.name || !editing.provider_type || saving}
              >
                {saving && <Loader2 size={12} className="animate-spin" />}
                {saving ? t("Saving...") : t("Save")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setEditing(null);
                  setApiKey("");
                  setError(null);
                }}
              >
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
