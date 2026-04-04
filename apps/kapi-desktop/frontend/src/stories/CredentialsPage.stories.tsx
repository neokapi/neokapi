import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, useCallback } from "react";
import { Plus, Trash2, TestTube, KeyRound, Loader2, CheckCircle2, XCircle } from "lucide-react";
import { Button, Badge, Card, Label, Input } from "@neokapi/ui-primitives";
import { CredentialsPage } from "../components/CredentialsPage";

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
        <Button size="sm" onClick={handleAdd}>
          <Plus size={12} />
          Add Provider
        </Button>
      </div>

      <div className="space-y-2">
        {providers.map((provider) => (
          <Card key={provider.id} className="flex items-center gap-3 p-4">
            <KeyRound size={18} className="shrink-0 text-primary" />
            <div className="flex-1">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium">{provider.name}</span>
                <Badge variant="secondary">{provider.provider_type}</Badge>
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
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => handleTest(provider.id)}
              disabled={provider.testResult === "testing"}
              aria-label={`Test ${provider.name}`}
            >
              {provider.testResult === "testing" ? (
                <Loader2 size={14} className="animate-spin" />
              ) : (
                <TestTube size={14} />
              )}
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => handleDelete(provider.id)}
              className="hover:bg-destructive/10 hover:text-destructive"
              aria-label={`Delete ${provider.name}`}
            >
              <Trash2 size={14} />
            </Button>
          </Card>
        ))}
      </div>

      {editing && (
        <Card className="mt-4 p-4">
          <h3 className="mb-3 text-sm font-medium">New Provider</h3>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">Name</Label>
              <Input
                type="text"
                value={editing.name}
                onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                placeholder="My OpenAI Key"
              />
            </div>
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">Provider</Label>
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
              <Label className="mb-1 block text-xs text-muted-foreground">Model</Label>
              <Input
                type="text"
                value={editing.model ?? ""}
                onChange={(e) => setEditing({ ...editing, model: e.target.value })}
                placeholder="gpt-4o"
              />
            </div>
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">API Key</Label>
              <Input
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder="sk-..."
              />
            </div>
          </div>
          <div className="mt-3 flex gap-2">
            <Button size="sm" onClick={handleSave} disabled={!editing.name || saving}>
              {saving && <Loader2 size={12} className="animate-spin" />}
              {saving ? "Saving..." : "Save to Keychain"}
            </Button>
            <Button variant="outline" size="sm" onClick={() => setEditing(null)}>
              Cancel
            </Button>
          </div>
        </Card>
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

/**
 * Real component with pre-loaded providers (no Wails API calls).
 */
export const WithProviders: StoryObj<typeof CredentialsPage> = {
  render: () => (
    <CredentialsPage
      providers={[
        {
          id: "1",
          name: "Production Anthropic",
          provider_type: "anthropic",
          model: "claude-sonnet-4-5-20241022",
        },
        {
          id: "2",
          name: "OpenAI GPT-4o",
          provider_type: "openai",
          model: "gpt-4o",
        },
      ]}
    />
  ),
};

/**
 * Real component with empty providers list.
 */
export const Empty: StoryObj<typeof CredentialsPage> = {
  render: () => <CredentialsPage providers={[]} />,
};

