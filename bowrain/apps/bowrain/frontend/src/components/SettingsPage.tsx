import { useState } from "react";
import { useTheme, type Theme, cn, Button, Input, Label, Badge, Card, CardContent, Tabs, TabsList, TabsTrigger, TabsContent, Select, SelectTrigger, SelectValue, SelectContent, SelectItem, Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@gokapi/ui";
import { useFormats, useTools, useFlows, usePlugins, useVersion, useProviderConfigs, useProviderApi } from "../hooks/useApi";
import type { FormatInfo, ToolInfo, FlowInfo, PluginInfo, ProviderConfig, ProviderConfigWithKey } from "../types/api";

type SettingsTab = "general" | "ai-providers" | "plugins" | "system-info";

const tabs: { id: SettingsTab; label: string }[] = [
  { id: "general", label: "General" },
  { id: "ai-providers", label: "AI Providers" },
  { id: "plugins", label: "Plugins" },
  { id: "system-info", label: "System Info" },
];

export function SettingsPage() {
  return (
    <div className="flex flex-col flex-1">
      <h1 className="mb-4">Settings</h1>
      <Tabs defaultValue="general" className="flex flex-col flex-1">
        <TabsList className="mb-6">
          {tabs.map((tab) => (
            <TabsTrigger key={tab.id} value={tab.id} data-testid={`settings-tab-${tab.id}`}>
              {tab.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <div className="flex-1 overflow-y-auto">
          <TabsContent value="general"><GeneralTab /></TabsContent>
          <TabsContent value="ai-providers"><AIProvidersTab /></TabsContent>
          <TabsContent value="plugins"><PluginsTab /></TabsContent>
          <TabsContent value="system-info"><SystemInfoTab /></TabsContent>
        </div>
      </Tabs>
    </div>
  );
}

/* ── General ── */

function GeneralTab() {
  const { theme, setTheme } = useTheme();
  const themeOptions: { value: Theme; label: string }[] = [
    { value: "light", label: "Light" },
    { value: "dark", label: "Dark" },
    { value: "system", label: "System" },
  ];

  return (
    <div data-testid="settings-general" className="flex flex-col gap-6">
      <section>
        <h3 className="mb-2 text-[15px]">Appearance</h3>
        <p className="text-muted-foreground text-[13px] mb-3">
          Choose your preferred color theme.
        </p>
        <div className="flex gap-2">
          {themeOptions.map((opt) => (
            <Button
              key={opt.value}
              data-testid={`theme-${opt.value}`}
              onClick={() => setTheme(opt.value)}
              variant={theme === opt.value ? "default" : "outline"}
              size="sm"
            >
              {opt.label}
            </Button>
          ))}
        </div>
      </section>
    </div>
  );
}

/* ── AI Providers ── */

const providerTypeDefaults: Record<string, { model: string; baseUrl: string }> = {
  anthropic: { model: "claude-sonnet-4-20250514", baseUrl: "https://api.anthropic.com" },
  openai: { model: "gpt-4o", baseUrl: "https://api.openai.com" },
  ollama: { model: "llama3", baseUrl: "http://localhost:11434" },
};

function AIProvidersTab() {
  const { configs, loading, error, refresh } = useProviderConfigs();
  const { saveProviderConfig, deleteProviderConfig, testProviderConfig } = useProviderApi();
  const [editing, setEditing] = useState<ProviderConfigWithKey | null>(null);
  const [testStatus, setTestStatus] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  if (loading) return <p className="text-muted-foreground">Loading providers...</p>;
  if (error) return <p className="text-destructive">Error: {error}</p>;

  const handleAdd = () => {
    setEditing({
      id: "",
      name: "",
      provider_type: "anthropic",
      model: "",
      base_url: "",
      api_key: "",
    });
    setTestStatus(null);
    setSubmitted(false);
  };

  const handleEdit = (cfg: ProviderConfig) => {
    setEditing({ ...cfg, api_key: "" });
    setTestStatus(null);
    setSubmitted(false);
  };

  const handleDelete = async (id: string) => {
    try {
      await deleteProviderConfig(id);
      refresh();
    } catch (e) {
      setTestStatus(e instanceof Error ? e.message : "Delete failed");
    }
  };

  const needsApiKey = editing ? editing.provider_type !== "ollama" && !editing.id : false;
  const missingName = editing ? !editing.name.trim() : false;
  const missingApiKey = editing ? needsApiKey && !editing.api_key.trim() : false;
  const hasErrors = missingName || missingApiKey;

  const handleSave = async () => {
    if (!editing) return;
    setSubmitted(true);
    if (missingName || missingApiKey) return;
    setSaving(true);
    try {
      await saveProviderConfig(editing);
      setEditing(null);
      refresh();
    } catch (e) {
      setTestStatus(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    if (!editing) return;
    setTestStatus("Testing...");
    try {
      await testProviderConfig(editing);
      setTestStatus("Connection successful");
    } catch (e) {
      setTestStatus(e instanceof Error ? e.message : "Test failed");
    }
  };

  const handleTypeChange = (type: string) => {
    if (!editing) return;
    setEditing({ ...editing, provider_type: type });
  };

  const handleDialogClose = (open: boolean) => {
    if (!open) {
      setEditing(null);
      setTestStatus(null);
      setSubmitted(false);
    }
  };

  const defaults = editing ? (providerTypeDefaults[editing.provider_type] || { model: "", baseUrl: "" }) : { model: "", baseUrl: "" };

  return (
    <div data-testid="settings-ai-providers">
      <Dialog open={!!editing} onOpenChange={handleDialogClose}>
        <DialogContent size="md" data-testid="provider-form" onInteractOutside={(e: Event) => e.preventDefault()}>
          <DialogHeader>
            <DialogTitle>{editing?.id ? "Edit Provider" : "Add Provider"}</DialogTitle>
          </DialogHeader>
          {editing && (
            <div className="flex flex-col gap-4 py-2">
              <div className="flex flex-col gap-1">
                <Label className="text-muted-foreground">
                  Name {submitted && missingName && <span className="text-destructive text-xs ml-1">Required</span>}
                </Label>
                <Input
                  type="text"
                  value={editing.name}
                  onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                  className={cn(submitted && missingName && "border-destructive")}
                  data-testid="provider-name"
                  placeholder="My Provider"
                  autoFocus
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label className="text-muted-foreground">Type</Label>
                <Select value={editing.provider_type} onValueChange={handleTypeChange}>
                  <SelectTrigger data-testid="provider-type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="anthropic">Anthropic</SelectItem>
                    <SelectItem value="openai">OpenAI</SelectItem>
                    <SelectItem value="ollama">Ollama</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-1">
                <Label className="text-muted-foreground">
                  API Key {submitted && missingApiKey && <span className="text-destructive text-xs ml-1">Required</span>}
                </Label>
                <Input
                  type="password"
                  value={editing.api_key}
                  onChange={(e) => setEditing({ ...editing, api_key: e.target.value })}
                  className={cn(submitted && missingApiKey && "border-destructive")}
                  data-testid="provider-api-key"
                  placeholder={editing.id ? "Enter new key or leave blank to keep current" : "Enter API key"}
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label className="text-muted-foreground">Model</Label>
                <Input
                  type="text"
                  value={editing.model}
                  onChange={(e) => setEditing({ ...editing, model: e.target.value })}
                  data-testid="provider-model"
                  placeholder={defaults.model}
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label className="text-muted-foreground">Base URL (optional)</Label>
                <Input
                  type="text"
                  value={editing.base_url}
                  onChange={(e) => setEditing({ ...editing, base_url: e.target.value })}
                  data-testid="provider-base-url"
                  placeholder={defaults.baseUrl}
                />
              </div>
              {testStatus && (
                <div
                  data-testid="provider-test-status"
                  className={cn(
                    "px-3 py-2 rounded-md text-[13px]",
                    testStatus.includes("successful") && "bg-green-500/10 text-green-600 dark:text-green-400",
                    testStatus === "Testing..." && "bg-blue-500/10 text-blue-600 dark:text-blue-400",
                    !testStatus.includes("successful") && testStatus !== "Testing..." && "bg-destructive/10 text-destructive",
                  )}
                >
                  {testStatus}
                </div>
              )}
              {submitted && hasErrors && (
                <div className="text-[13px] text-destructive" data-testid="provider-validation-error">
                  Please fill in all required fields.
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={handleTest} data-testid="provider-test-btn">
              Test Connection
            </Button>
            <Button variant="outline" size="sm" onClick={() => handleDialogClose(false)} data-testid="provider-cancel-btn">
              Cancel
            </Button>
            <Button size="sm" onClick={handleSave} disabled={saving} data-testid="provider-save-btn">
              {saving ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <div className="flex justify-between items-center mb-4">
        <p className="text-muted-foreground text-[13px]">
          Manage AI provider credentials. API keys are stored securely in the OS keychain.
        </p>
        <Button onClick={handleAdd} data-testid="add-provider-btn">
          Add Provider
        </Button>
      </div>
      {configs.length === 0 ? (
        <Card data-testid="providers-empty">
          <CardContent className="py-6 text-center text-muted-foreground">
            <p className="mb-2">No AI providers configured.</p>
            <p className="text-[13px]">
              Add a provider to use AI translation features with saved credentials.
            </p>
          </CardContent>
        </Card>
      ) : (
        <table className="w-full border-collapse bg-card rounded-lg overflow-hidden">
          <thead>
            <tr>
              <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Name</th>
              <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Type</th>
              <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Model</th>
              <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Actions</th>
            </tr>
          </thead>
          <tbody>
            {configs.map((cfg) => (
              <tr key={cfg.id} data-testid={`provider-row-${cfg.id}`} className="border-b border-border transition-colors hover:bg-accent/50">
                <td className="px-3 py-2.5 text-sm">{cfg.name}</td>
                <td className="px-3 py-2.5 text-sm">
                  <Badge variant="secondary">{cfg.provider_type}</Badge>
                </td>
                <td className="px-3 py-2.5 text-sm">{cfg.model || "-"}</td>
                <td className="px-3 py-2.5 text-sm">
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" onClick={() => handleEdit(cfg)} data-testid={`edit-provider-${cfg.id}`}>
                      Edit
                    </Button>
                    <Button variant="outline" size="sm" className="text-destructive" onClick={() => handleDelete(cfg.id)} data-testid={`delete-provider-${cfg.id}`}>
                      Delete
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

/* ── Plugins ── */

function PluginsTab() {
  const { plugins, pluginDir, loading, error } = usePlugins();

  if (loading) return <p className="text-muted-foreground">Loading plugins...</p>;
  if (error) return <p className="text-destructive">Error: {error}</p>;

  return (
    <div data-testid="settings-plugins">
      <p className="text-muted-foreground mb-4 text-[13px]">
        Plugin directory: <code className="text-xs">{pluginDir || "(not configured)"}</code>
      </p>
      {plugins.length === 0 ? <PluginsEmpty /> : <PluginsTable plugins={plugins} />}
    </div>
  );
}

function PluginsEmpty() {
  return (
    <Card data-testid="plugins-empty">
      <CardContent className="py-6 text-center text-muted-foreground">
        <p className="mb-2">No plugins loaded.</p>
        <p className="text-[13px]">
          Place plugin binaries or bridge descriptors in the plugin directory to extend
          available formats and tools.
        </p>
      </CardContent>
    </Card>
  );
}

function PluginsTable({ plugins }: { plugins: PluginInfo[] }) {
  return (
    <table className="w-full border-collapse bg-card rounded-lg overflow-hidden">
      <thead>
        <tr>
          <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Name</th>
          <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Type</th>
          <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Formats</th>
        </tr>
      </thead>
      <tbody>
        {plugins.map((p) => (
          <tr key={p.name} className="border-b border-border transition-colors hover:bg-accent/50">
            <td className="px-3 py-2.5 text-sm">{p.name}</td>
            <td className="px-3 py-2.5 text-sm">
              <Badge variant="secondary">{p.type}</Badge>
            </td>
            <td className="px-3 py-2.5 text-sm">{p.formats.length > 0 ? p.formats.join(", ") : "-"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/* ── System Info ── */

function SystemInfoTab() {
  const { version } = useVersion();
  const { formats, loading: fmtLoading, error: fmtError } = useFormats();
  const { tools, loading: toolLoading, error: toolError } = useTools();
  const { flows, loading: flowLoading, error: flowError } = useFlows();

  const loading = fmtLoading || toolLoading || flowLoading;
  const error = fmtError || toolError || flowError;

  if (loading) return <p className="text-muted-foreground">Loading...</p>;
  if (error) return <p className="text-destructive">Error: {error}</p>;

  return (
    <div data-testid="settings-system-info" className="flex flex-col gap-8">
      {version && (
        <section data-testid="version-info">
          <h2 className="mb-2">Version</h2>
          <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
            <span className="text-muted-foreground">Version</span>
            <span>{version.version}</span>
            <span className="text-muted-foreground">Commit</span>
            <span className="font-mono text-xs">{version.commit}</span>
            <span className="text-muted-foreground">Build Date</span>
            <span>{version.build_date}</span>
          </div>
        </section>
      )}
      <FormatsSection formats={formats} />
      <ToolsSection tools={tools} />
      <FlowsSection flows={flows} />
    </div>
  );
}

function FormatsSection({ formats }: { formats: FormatInfo[] }) {
  return (
    <section>
      <h2 className="mb-2">Formats</h2>
      <p className="text-muted-foreground mb-3">
        {formats.length} format(s) registered
      </p>
      <table className="w-full border-collapse bg-card rounded-lg overflow-hidden">
        <thead>
          <tr>
            <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Format</th>
            <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Read</th>
            <th className="px-3 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Write</th>
          </tr>
        </thead>
        <tbody>
          {formats.map((f) => (
            <tr key={f.name} className="border-b border-border">
              <td className="px-3 py-2.5 text-sm">{f.name}</td>
              <td className="px-3 py-2.5 text-sm"><StatusBadge ok={f.has_reader} /></td>
              <td className="px-3 py-2.5 text-sm"><StatusBadge ok={f.has_writer} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function ToolsSection({ tools }: { tools: ToolInfo[] }) {
  return (
    <section>
      <h2 className="mb-2">Tools</h2>
      <p className="text-muted-foreground mb-3">
        {tools.length} tool(s) available
      </p>
      <div className="flex flex-col gap-2">
        {tools.map((t) => (
          <Card key={t.name}>
            <CardContent className="py-3">
              <div className="font-semibold mb-1">{t.name}</div>
              <div className="text-[13px] text-muted-foreground">{t.description}</div>
            </CardContent>
          </Card>
        ))}
      </div>
    </section>
  );
}

function FlowsSection({ flows }: { flows: FlowInfo[] }) {
  return (
    <section>
      <h2 className="mb-2">Flows</h2>
      <p className="text-muted-foreground mb-3">
        {flows.length} flow(s) available
      </p>
      <div className="flex flex-col gap-2">
        {flows.map((f) => (
          <Card key={f.name}>
            <CardContent className="py-3">
              <div className="font-semibold mb-1">{f.name}</div>
              <div className="text-[13px] text-muted-foreground">{f.description}</div>
            </CardContent>
          </Card>
        ))}
      </div>
    </section>
  );
}

/* ── Shared ── */

function StatusBadge({ ok }: { ok: boolean }) {
  return (
    <Badge variant={ok ? "default" : "destructive"} className="text-xs">
      {ok ? "Yes" : "No"}
    </Badge>
  );
}
