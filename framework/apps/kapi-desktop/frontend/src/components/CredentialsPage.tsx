import { useState, useEffect, useCallback } from "react";
import { Plus, Trash2, TestTube, KeyRound, Loader2, CheckCircle2 } from "lucide-react";
import type { ProviderConfig } from "../types/api";
import { api } from "../hooks/useApi";

const PROVIDER_TYPES = ["anthropic", "openai", "ollama", "azureopenai"] as const;

export function CredentialsPage() {
  const [providers, setProviders] = useState<ProviderConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<ProviderConfig | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [testResult, setTestResult] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);

  const loadProviders = useCallback(async () => {
    const result = await api.listProviders();
    if (result) setProviders(result);
    setLoading(false);
  }, []);

  useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  const handleAdd = () => {
    setEditing({
      id: "",
      name: "",
      provider_type: "anthropic",
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
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">AI Credentials</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            API keys are stored in your OS keychain
          </p>
        </div>
        <button
          onClick={handleAdd}
          className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          aria-label="Add AI provider"
        >
          <Plus size={12} />
          Add Provider
        </button>
      </div>

      {error && (
        <p className="mb-4 text-sm text-destructive" role="alert">{error}</p>
      )}

      {loading ? (
        <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Loader2 size={16} className="animate-spin" />
          Loading providers...
        </div>
      ) : (
        <div className="space-y-2">
          {providers.map((provider) => (
            <div
              key={provider.id}
              className="flex items-center gap-3 rounded-lg border border-border p-4"
            >
              <KeyRound size={18} className="shrink-0 text-primary" />
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{provider.name}</span>
                  <span className="rounded bg-accent px-1.5 py-0.5 text-xs">
                    {provider.provider_type}
                  </span>
                  {testResult[provider.id] && (
                    <CheckCircle2 size={14} className="text-green-500" />
                  )}
                </div>
                {provider.model && (
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    Model: {provider.model}
                  </p>
                )}
              </div>
              <button
                onClick={() => handleTest(provider.id)}
                className="rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                aria-label={`Test connection for ${provider.name}`}
              >
                <TestTube size={14} />
              </button>
              <button
                onClick={() => handleDelete(provider.id)}
                className="rounded p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                aria-label={`Delete ${provider.name}`}
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
          {providers.length === 0 && !editing && (
            <div className="rounded-lg border border-dashed border-border p-8 text-center">
              <KeyRound size={24} className="mx-auto mb-2 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">
                No AI providers configured. Add one to use AI translation and QA tools.
              </p>
            </div>
          )}
        </div>
      )}

      {editing && (
        <div className="mt-4 rounded-lg border border-border p-4">
          <h3 className="mb-3 text-sm font-medium">
            {editing.id ? "Edit Provider" : "New Provider"}
          </h3>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1 block text-xs text-muted-foreground" htmlFor="cred-name">
                Name
              </label>
              <input
                id="cred-name"
                type="text"
                value={editing.name}
                onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                placeholder="My Anthropic Key"
                className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-muted-foreground" htmlFor="cred-type">
                Provider
              </label>
              <select
                id="cred-type"
                value={editing.provider_type}
                onChange={(e) => setEditing({ ...editing, provider_type: e.target.value })}
                className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              >
                {PROVIDER_TYPES.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="mb-1 block text-xs text-muted-foreground" htmlFor="cred-model">
                Model (optional)
              </label>
              <input
                id="cred-model"
                type="text"
                value={editing.model ?? ""}
                onChange={(e) => setEditing({ ...editing, model: e.target.value })}
                placeholder="claude-sonnet-4-5-20241022"
                className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-muted-foreground" htmlFor="cred-apikey">
                API Key
              </label>
              <input
                id="cred-apikey"
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder="sk-..."
                className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>
          <div className="mt-3 flex gap-2">
            <button
              onClick={handleSave}
              disabled={!editing.name || !editing.provider_type || saving}
              className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {saving && <Loader2 size={12} className="animate-spin" />}
              {saving ? "Saving..." : "Save"}
            </button>
            <button
              onClick={() => { setEditing(null); setApiKey(""); setError(null); }}
              className="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
