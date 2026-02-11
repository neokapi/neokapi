import { useState } from "react";
import { useTheme, type Theme } from "@gokapi/ui";
import { useFormats, useTools, useFlows, usePlugins, useProviderConfigs, useProviderApi } from "../hooks/useApi";
import type { FormatInfo, ToolInfo, FlowInfo, PluginInfo, ProviderConfig, ProviderConfigWithKey } from "../types/api";

type SettingsTab = "general" | "ai-providers" | "plugins" | "system-info";

const tabs: { id: SettingsTab; label: string }[] = [
  { id: "general", label: "General" },
  { id: "ai-providers", label: "AI Providers" },
  { id: "plugins", label: "Plugins" },
  { id: "system-info", label: "System Info" },
];

export function SettingsPage() {
  const [activeTab, setActiveTab] = useState<SettingsTab>("general");

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 0, flex: 1 }}>
      <h1 style={{ margin: "0 0 16px" }}>Settings</h1>
      <div
        role="tablist"
        style={{
          display: "flex",
          gap: 0,
          borderBottom: "1px solid var(--border)",
          marginBottom: 24,
        }}
      >
        {tabs.map((tab) => (
          <button
            key={tab.id}
            role="tab"
            aria-selected={activeTab === tab.id}
            data-testid={`settings-tab-${tab.id}`}
            onClick={() => setActiveTab(tab.id)}
            style={{
              padding: "10px 20px",
              background: "none",
              border: "none",
              borderBottom: activeTab === tab.id
                ? "2px solid var(--accent)"
                : "2px solid transparent",
              color: activeTab === tab.id
                ? "var(--text-primary)"
                : "var(--text-secondary)",
              fontWeight: activeTab === tab.id ? 600 : 400,
              fontSize: 14,
              cursor: "pointer",
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div style={{ flex: 1, overflowY: "auto" }}>
        {activeTab === "general" && <GeneralTab />}
        {activeTab === "ai-providers" && <AIProvidersTab />}
        {activeTab === "plugins" && <PluginsTab />}
        {activeTab === "system-info" && <SystemInfoTab />}
      </div>
    </div>
  );
}

/* ── General ── */

function GeneralTab() {
  const { theme, setTheme } = useTheme();
  const themeOptions: { value: Theme; label: string }[] = [
    { value: "system", label: "System" },
    { value: "light", label: "Light" },
    { value: "dark", label: "Dark" },
  ];

  return (
    <div data-testid="settings-general" style={{ display: "flex", flexDirection: "column", gap: 24 }}>
      <section>
        <h3 style={{ margin: "0 0 8px", fontSize: 15 }}>Appearance</h3>
        <p style={{ color: "var(--text-secondary)", fontSize: 13, marginBottom: 12 }}>
          Choose your preferred color theme.
        </p>
        <div style={{ display: "flex", gap: 8 }}>
          {themeOptions.map((opt) => (
            <button
              key={opt.value}
              data-testid={`theme-${opt.value}`}
              onClick={() => setTheme(opt.value)}
              style={{
                padding: "6px 16px",
                fontSize: 13,
                fontWeight: theme === opt.value ? 600 : 400,
                backgroundColor: theme === opt.value ? "var(--accent)" : "var(--bg-secondary)",
                color: theme === opt.value ? "#fff" : "var(--text-primary)",
                border: theme === opt.value ? "none" : "1px solid var(--border)",
                borderRadius: 6,
                cursor: "pointer",
              }}
            >
              {opt.label}
            </button>
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

  if (loading) return <p style={{ color: "var(--text-secondary)" }}>Loading providers...</p>;
  if (error) return <p style={{ color: "var(--error)" }}>Error: {error}</p>;

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

  if (editing) {
    const defaults = providerTypeDefaults[editing.provider_type] || { model: "", baseUrl: "" };
    return (
      <div data-testid="settings-ai-providers">
        <div data-testid="provider-form" style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 500 }}>
          <h3 style={{ margin: 0 }}>{editing.id ? "Edit Provider" : "Add Provider"}</h3>
          <label style={labelStyle}>
            Name {submitted && missingName && <span style={fieldErrorStyle}>Required</span>}
            <input
              type="text"
              value={editing.name}
              onChange={(e) => setEditing({ ...editing, name: e.target.value })}
              style={{ ...inputStyle, ...(submitted && missingName ? fieldErrorBorder : {}) }}
              data-testid="provider-name"
              placeholder="My Provider"
            />
          </label>
          <label style={labelStyle}>
            Type
            <select
              value={editing.provider_type}
              onChange={(e) => handleTypeChange(e.target.value)}
              style={inputStyle}
              data-testid="provider-type"
            >
              <option value="anthropic">Anthropic</option>
              <option value="openai">OpenAI</option>
              <option value="ollama">Ollama</option>
            </select>
          </label>
          <label style={labelStyle}>
            API Key {submitted && missingApiKey && <span style={fieldErrorStyle}>Required</span>}
            <input
              type="password"
              value={editing.api_key}
              onChange={(e) => setEditing({ ...editing, api_key: e.target.value })}
              style={{ ...inputStyle, ...(submitted && missingApiKey ? fieldErrorBorder : {}) }}
              data-testid="provider-api-key"
              placeholder={editing.id ? "Enter new key or leave blank to keep current" : "Enter API key"}
            />
          </label>
          <label style={labelStyle}>
            Model
            <input
              type="text"
              value={editing.model}
              onChange={(e) => setEditing({ ...editing, model: e.target.value })}
              style={inputStyle}
              data-testid="provider-model"
              placeholder={defaults.model}
            />
          </label>
          <label style={labelStyle}>
            Base URL (optional)
            <input
              type="text"
              value={editing.base_url}
              onChange={(e) => setEditing({ ...editing, base_url: e.target.value })}
              style={inputStyle}
              data-testid="provider-base-url"
              placeholder={defaults.baseUrl}
            />
          </label>
          {testStatus && (
            <div
              data-testid="provider-test-status"
              style={{
                padding: "8px 12px",
                borderRadius: 6,
                fontSize: 13,
                backgroundColor: testStatus.includes("successful")
                  ? "rgba(34,197,94,0.1)"
                  : testStatus === "Testing..."
                    ? "rgba(96,165,250,0.1)"
                    : "rgba(239,68,68,0.1)",
                color: testStatus.includes("successful")
                  ? "var(--success)"
                  : testStatus === "Testing..."
                    ? "var(--accent)"
                    : "var(--error)",
              }}
            >
              {testStatus}
            </div>
          )}
          {submitted && hasErrors && (
            <div style={{ fontSize: 13, color: "var(--error)" }} data-testid="provider-validation-error">
              Please fill in all required fields.
            </div>
          )}
          <div style={{ display: "flex", gap: 8 }}>
            <button onClick={handleTest} style={toolBtnStyle} data-testid="provider-test-btn">
              Test Connection
            </button>
            <button onClick={handleSave} disabled={saving} style={saveBtnStyle} data-testid="provider-save-btn">
              {saving ? "Saving..." : "Save"}
            </button>
            <button onClick={() => setEditing(null)} style={toolBtnStyle} data-testid="provider-cancel-btn">
              Cancel
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div data-testid="settings-ai-providers">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
        <p style={{ color: "var(--text-secondary)", margin: 0, fontSize: 13 }}>
          Manage AI provider credentials. API keys are stored securely in the OS keychain.
        </p>
        <button onClick={handleAdd} style={saveBtnStyle} data-testid="add-provider-btn">
          Add Provider
        </button>
      </div>
      {configs.length === 0 ? (
        <div
          data-testid="providers-empty"
          style={{
            padding: "24px 16px",
            backgroundColor: "var(--bg-secondary)",
            borderRadius: 8,
            border: "1px solid var(--border)",
            textAlign: "center",
            color: "var(--text-secondary)",
          }}
        >
          <p style={{ marginBottom: 8 }}>No AI providers configured.</p>
          <p style={{ fontSize: 13 }}>
            Add a provider to use AI translation features with saved credentials.
          </p>
        </div>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid var(--border)" }}>
              <th style={thStyle}>Name</th>
              <th style={thStyle}>Type</th>
              <th style={thStyle}>Model</th>
              <th style={thStyle}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {configs.map((cfg) => (
              <tr key={cfg.id} data-testid={`provider-row-${cfg.id}`} style={{ borderBottom: "1px solid var(--border)" }}>
                <td style={tdStyle}>{cfg.name}</td>
                <td style={tdStyle}>
                  <span
                    style={{
                      display: "inline-block",
                      padding: "2px 8px",
                      borderRadius: 4,
                      fontSize: 12,
                      backgroundColor: "rgba(96,165,250,0.15)",
                      color: "var(--accent)",
                    }}
                  >
                    {cfg.provider_type}
                  </span>
                </td>
                <td style={tdStyle}>{cfg.model || "-"}</td>
                <td style={tdStyle}>
                  <div style={{ display: "flex", gap: 8 }}>
                    <button onClick={() => handleEdit(cfg)} style={smallBtnStyle} data-testid={`edit-provider-${cfg.id}`}>
                      Edit
                    </button>
                    <button onClick={() => handleDelete(cfg.id)} style={{ ...smallBtnStyle, color: "var(--error)" }} data-testid={`delete-provider-${cfg.id}`}>
                      Delete
                    </button>
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

const labelStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 4,
  fontSize: 13,
  color: "var(--text-secondary)",
};

const inputStyle: React.CSSProperties = {
  padding: "8px 12px",
  backgroundColor: "var(--bg-tertiary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  color: "var(--text-primary)",
  fontSize: 14,
  outline: "none",
};

const toolBtnStyle: React.CSSProperties = {
  padding: "6px 12px",
  backgroundColor: "var(--bg-secondary)",
  color: "var(--text-primary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  fontSize: 12,
  cursor: "pointer",
  fontWeight: 500,
};

const saveBtnStyle: React.CSSProperties = {
  padding: "6px 16px",
  backgroundColor: "var(--accent)",
  color: "#fff",
  border: "none",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
  fontWeight: 600,
};

const fieldErrorStyle: React.CSSProperties = {
  color: "var(--error)",
  fontSize: 12,
  fontWeight: 400,
  marginLeft: 4,
};

const fieldErrorBorder: React.CSSProperties = {
  borderColor: "var(--error)",
};

const smallBtnStyle: React.CSSProperties = {
  padding: "4px 8px",
  background: "none",
  border: "1px solid var(--border)",
  borderRadius: 4,
  fontSize: 12,
  cursor: "pointer",
  color: "var(--text-primary)",
};

/* ── Plugins ── */

function PluginsTab() {
  const { plugins, pluginDir, loading, error } = usePlugins();

  if (loading) return <p style={{ color: "var(--text-secondary)" }}>Loading plugins...</p>;
  if (error) return <p style={{ color: "var(--error)" }}>Error: {error}</p>;

  return (
    <div data-testid="settings-plugins">
      <p style={{ color: "var(--text-secondary)", marginBottom: 16, fontSize: 13 }}>
        Plugin directory: <code style={{ fontSize: 12 }}>{pluginDir || "(not configured)"}</code>
      </p>
      {plugins.length === 0 ? <PluginsEmpty /> : <PluginsTable plugins={plugins} />}
    </div>
  );
}

function PluginsEmpty() {
  return (
    <div
      data-testid="plugins-empty"
      style={{
        padding: "24px 16px",
        backgroundColor: "var(--bg-secondary)",
        borderRadius: 8,
        border: "1px solid var(--border)",
        textAlign: "center",
        color: "var(--text-secondary)",
      }}
    >
      <p style={{ marginBottom: 8 }}>No plugins loaded.</p>
      <p style={{ fontSize: 13 }}>
        Place plugin binaries or bridge descriptors in the plugin directory to extend
        available formats and tools.
      </p>
    </div>
  );
}

function PluginsTable({ plugins }: { plugins: PluginInfo[] }) {
  return (
    <table style={{ width: "100%", borderCollapse: "collapse" }}>
      <thead>
        <tr style={{ borderBottom: "1px solid var(--border)" }}>
          <th style={thStyle}>Name</th>
          <th style={thStyle}>Type</th>
          <th style={thStyle}>Formats</th>
        </tr>
      </thead>
      <tbody>
        {plugins.map((p) => (
          <tr key={p.name} style={{ borderBottom: "1px solid var(--border)" }}>
            <td style={tdStyle}>{p.name}</td>
            <td style={tdStyle}>
              <span
                style={{
                  display: "inline-block",
                  padding: "2px 8px",
                  borderRadius: 4,
                  fontSize: 12,
                  backgroundColor: "rgba(96,165,250,0.15)",
                  color: "var(--accent)",
                }}
              >
                {p.type}
              </span>
            </td>
            <td style={tdStyle}>{p.formats.length > 0 ? p.formats.join(", ") : "-"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/* ── System Info ── */

function SystemInfoTab() {
  const { formats, loading: fmtLoading, error: fmtError } = useFormats();
  const { tools, loading: toolLoading, error: toolError } = useTools();
  const { flows, loading: flowLoading, error: flowError } = useFlows();

  const loading = fmtLoading || toolLoading || flowLoading;
  const error = fmtError || toolError || flowError;

  if (loading) return <p style={{ color: "var(--text-secondary)" }}>Loading...</p>;
  if (error) return <p style={{ color: "var(--error)" }}>Error: {error}</p>;

  return (
    <div data-testid="settings-system-info" style={{ display: "flex", flexDirection: "column", gap: 32 }}>
      <FormatsSection formats={formats} />
      <ToolsSection tools={tools} />
      <FlowsSection flows={flows} />
    </div>
  );
}

function FormatsSection({ formats }: { formats: FormatInfo[] }) {
  return (
    <section>
      <h2 style={{ marginBottom: 8 }}>Formats</h2>
      <p style={{ color: "var(--text-secondary)", marginBottom: 12 }}>
        {formats.length} format(s) registered
      </p>
      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr style={{ borderBottom: "1px solid var(--border)" }}>
            <th style={thStyle}>Format</th>
            <th style={thStyle}>Read</th>
            <th style={thStyle}>Write</th>
          </tr>
        </thead>
        <tbody>
          {formats.map((f) => (
            <tr key={f.name} style={{ borderBottom: "1px solid var(--border)" }}>
              <td style={tdStyle}>{f.name}</td>
              <td style={tdStyle}><Badge ok={f.has_reader} /></td>
              <td style={tdStyle}><Badge ok={f.has_writer} /></td>
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
      <h2 style={{ marginBottom: 8 }}>Tools</h2>
      <p style={{ color: "var(--text-secondary)", marginBottom: 12 }}>
        {tools.length} tool(s) available
      </p>
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        {tools.map((t) => (
          <div
            key={t.name}
            style={{
              padding: "12px 16px",
              backgroundColor: "var(--bg-secondary)",
              borderRadius: 8,
              border: "1px solid var(--border)",
            }}
          >
            <div style={{ fontWeight: 600, marginBottom: 4 }}>{t.name}</div>
            <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
              {t.description}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function FlowsSection({ flows }: { flows: FlowInfo[] }) {
  return (
    <section>
      <h2 style={{ marginBottom: 8 }}>Flows</h2>
      <p style={{ color: "var(--text-secondary)", marginBottom: 12 }}>
        {flows.length} flow(s) available
      </p>
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        {flows.map((f) => (
          <div
            key={f.name}
            style={{
              padding: "12px 16px",
              backgroundColor: "var(--bg-secondary)",
              borderRadius: 8,
              border: "1px solid var(--border)",
            }}
          >
            <div style={{ fontWeight: 600, marginBottom: 4 }}>{f.name}</div>
            <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
              {f.description}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

/* ── Shared ── */

function Badge({ ok }: { ok: boolean }) {
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: 4,
        fontSize: 12,
        backgroundColor: ok ? "rgba(34,197,94,0.15)" : "rgba(239,68,68,0.15)",
        color: ok ? "var(--success)" : "var(--error)",
      }}
    >
      {ok ? "Yes" : "No"}
    </span>
  );
}

const thStyle: React.CSSProperties = {
  textAlign: "left",
  padding: "10px 12px",
  color: "var(--text-secondary)",
  fontSize: 12,
  textTransform: "uppercase",
  letterSpacing: 0.5,
};

const tdStyle: React.CSSProperties = {
  padding: "10px 12px",
  fontSize: 14,
};
