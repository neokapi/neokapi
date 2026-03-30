import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, useCallback } from "react";
import { Plus, Trash2, TestTube, KeyRound, Loader2, CheckCircle2, XCircle } from "lucide-react";

const PROVIDER_TYPES = ["anthropic", "openai", "ollama", "azureopenai"] as const;

interface Provider {
  id: string;
  name: string;
  provider_type: string;
  model?: string;
  testResult?: "success" | "error" | "testing";
}

function SimulatedCredentials() {
  const [providers, setProviders] = useState<Provider[]>([
    {
      id: "1",
      name: "Production Anthropic",
      provider_type: "anthropic",
      model: "claude-sonnet-4-5-20241022",
    },
  ]);
  const [editing, setEditing] = useState<Provider | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);

  const handleAdd = () => {
    setEditing({ id: "", name: "", provider_type: "anthropic" });
    setApiKey("");
  };

  const handleSave = useCallback(async () => {
    if (!editing) return;
    setSaving(true);
    await new Promise((r) => setTimeout(r, 800));
    const saved: Provider = {
      ...editing,
      id: editing.id || crypto.randomUUID().slice(0, 8),
    };
    setProviders((prev) => {
      const idx = prev.findIndex((p) => p.id === saved.id);
      if (idx >= 0) {
        const next = [...prev];
        next[idx] = saved;
        return next;
      }
      return [...prev, saved];
    });
    setSaving(false);
    setEditing(null);
    setApiKey("");
  }, [editing]);

  const handleDelete = useCallback((id: string) => {
    setProviders((prev) => prev.filter((p) => p.id !== id));
  }, []);

  const handleTest = useCallback(async (id: string) => {
    setProviders((prev) =>
      prev.map((p) => (p.id === id ? { ...p, testResult: "testing" as const } : p)),
    );
    await new Promise((r) => setTimeout(r, 1200));
    const success = Math.random() > 0.3;
    setProviders((prev) =>
      prev.map((p) =>
        p.id === id ? { ...p, testResult: success ? ("success" as const) : ("error" as const) } : p,
      ),
    );
  }, []);

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
        >
          <Plus size={12} />
          Add Provider
        </button>
      </div>

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
                {provider.testResult === "success" && (
                  <CheckCircle2 size={14} className="text-green-500" />
                )}
                {provider.testResult === "error" && (
                  <XCircle size={14} className="text-destructive" />
                )}
              </div>
              {provider.model && (
                <p className="mt-0.5 text-xs text-muted-foreground">Model: {provider.model}</p>
              )}
            </div>
            <button
              onClick={() => handleTest(provider.id)}
              disabled={provider.testResult === "testing"}
              className="flex items-center gap-1 rounded p-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
              aria-label={`Test ${provider.name}`}
            >
              {provider.testResult === "testing" ? (
                <Loader2 size={14} className="animate-spin" />
              ) : (
                <TestTube size={14} />
              )}
              {provider.testResult === "testing" ? "Testing..." : "Test"}
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
      </div>

      {editing && (
        <div className="mt-4 rounded-lg border border-border p-4">
          <h3 className="mb-3 text-sm font-medium">New Provider</h3>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1 block text-xs text-muted-foreground">Name</label>
              <input
                type="text"
                value={editing.name}
                onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                placeholder="My OpenAI Key"
                className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-muted-foreground">Provider</label>
              <select
                value={editing.provider_type}
                onChange={(e) => setEditing({ ...editing, provider_type: e.target.value })}
                className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none"
              >
                {PROVIDER_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="mb-1 block text-xs text-muted-foreground">Model</label>
              <input
                type="text"
                value={editing.model ?? ""}
                onChange={(e) => setEditing({ ...editing, model: e.target.value })}
                placeholder="gpt-4o"
                className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-muted-foreground">API Key</label>
              <input
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
              disabled={!editing.name || saving}
              className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {saving && <Loader2 size={12} className="animate-spin" />}
              {saving ? "Saving..." : "Save to Keychain"}
            </button>
            <button
              onClick={() => setEditing(null)}
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

const meta: Meta<typeof SimulatedCredentials> = {
  title: "Interactions/Credential Management",
  component: SimulatedCredentials,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Demonstrates adding AI providers, saving API keys to the OS keychain with a spinner, testing connections with success/failure states, and deletion.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SimulatedCredentials>;

export const Default: Story = {};
