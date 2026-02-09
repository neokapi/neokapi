import { useState, useEffect, useCallback } from "react";
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../../bindings/github.com/gokapi/gokapi/apps/bowrain/backend/app.js";

interface ConnectorInfo {
  id: string;
  name: string;
  type: string;
  category: string;
}

interface ContentItem {
  id: string;
  path: string;
  title: string;
  block_count: number;
}

interface SyncStatus {
  connector_id: string;
  last_sync: string;
  item_count: number;
  status: string;
}

export function ConnectorPanel() {
  const [connectorTypes, setConnectorTypes] = useState<string[]>([]);
  const [activeConnectors, setActiveConnectors] = useState<ConnectorInfo[]>([]);
  const [selectedType, setSelectedType] = useState("");
  const [configPath, setConfigPath] = useState("");
  const [configFormat, setConfigFormat] = useState("");
  const [contentItems, setContentItems] = useState<ContentItem[]>([]);
  const [selectedConnector, setSelectedConnector] = useState<string | null>(null);
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  const loadConnectorTypes = useCallback(async () => {
    try {
      const types = await Backend.ListConnectorTypes();
      setConnectorTypes(types || []);
    } catch (e) {
      console.error("Failed to load connector types:", e);
    }
  }, []);

  const loadActiveConnectors = useCallback(async () => {
    try {
      const connectors = await Backend.ListConnectors();
      setActiveConnectors(connectors || []);
    } catch (e) {
      console.error("Failed to load connectors:", e);
    }
  }, []);

  useEffect(() => {
    loadConnectorTypes();
    loadActiveConnectors();
  }, [loadConnectorTypes, loadActiveConnectors]);

  const handleAddConnector = async () => {
    if (!selectedType) return;
    setError(null);
    try {
      const config: Record<string, string> = {};
      if (configPath) config.path = configPath;
      if (configFormat) config.format = configFormat;
      await Backend.ConfigureConnector(selectedType, config);
      setSelectedType("");
      setConfigPath("");
      setConfigFormat("");
      loadActiveConnectors();
    } catch (e) {
      setError(String(e));
    }
  };

  const handleRemoveConnector = async (id: string) => {
    try {
      await Backend.RemoveConnector(id);
      if (selectedConnector === id) {
        setSelectedConnector(null);
        setContentItems([]);
        setSyncStatus(null);
      }
      loadActiveConnectors();
    } catch (e) {
      setError(String(e));
    }
  };

  const handleSelectConnector = async (id: string) => {
    setSelectedConnector(id);
    setError(null);
    try {
      const items = await Backend.ListContentItems(id);
      setContentItems(items || []);
    } catch (e) {
      setContentItems([]);
    }
    try {
      const status = await Backend.GetSyncStatus(id);
      setSyncStatus(status);
    } catch {
      setSyncStatus(null);
    }
  };

  return (
    <div style={{ padding: 24, maxWidth: 800 }}>
      <h2 style={{ color: "var(--text-primary)", marginBottom: 24 }}>Connectors</h2>

      {error && (
        <div style={{ padding: 12, background: "#ff00001a", borderRadius: 8, marginBottom: 16, color: "#ff4444" }}>
          {error}
        </div>
      )}

      {/* Add connector form */}
      <div style={{ padding: 16, background: "var(--bg-secondary)", borderRadius: 8, marginBottom: 24 }}>
        <h3 style={{ color: "var(--text-primary)", marginBottom: 12 }}>Add Connector</h3>
        <div style={{ display: "flex", gap: 8, alignItems: "flex-end", flexWrap: "wrap" }}>
          <div>
            <label style={{ color: "var(--text-secondary)", fontSize: 12, display: "block", marginBottom: 4 }}>Type</label>
            <select
              value={selectedType}
              onChange={(e) => setSelectedType(e.target.value)}
              style={{ padding: "8px 12px", borderRadius: 6, border: "1px solid var(--border)", background: "var(--bg-primary)", color: "var(--text-primary)" }}
            >
              <option value="">Select type...</option>
              {connectorTypes.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>
          <div>
            <label style={{ color: "var(--text-secondary)", fontSize: 12, display: "block", marginBottom: 4 }}>Path</label>
            <input
              type="text"
              value={configPath}
              onChange={(e) => setConfigPath(e.target.value)}
              placeholder="/path/to/content"
              style={{ padding: "8px 12px", borderRadius: 6, border: "1px solid var(--border)", background: "var(--bg-primary)", color: "var(--text-primary)", width: 200 }}
            />
          </div>
          <div>
            <label style={{ color: "var(--text-secondary)", fontSize: 12, display: "block", marginBottom: 4 }}>Format</label>
            <input
              type="text"
              value={configFormat}
              onChange={(e) => setConfigFormat(e.target.value)}
              placeholder="json, html..."
              style={{ padding: "8px 12px", borderRadius: 6, border: "1px solid var(--border)", background: "var(--bg-primary)", color: "var(--text-primary)", width: 120 }}
            />
          </div>
          <button
            onClick={handleAddConnector}
            disabled={!selectedType}
            style={{ padding: "8px 16px", borderRadius: 6, border: "none", background: "var(--accent)", color: "white", cursor: selectedType ? "pointer" : "not-allowed", opacity: selectedType ? 1 : 0.5 }}
          >
            Add
          </button>
        </div>
      </div>

      {/* Active connectors */}
      <div style={{ display: "flex", gap: 16 }}>
        <div style={{ flex: 1 }}>
          <h3 style={{ color: "var(--text-primary)", marginBottom: 12 }}>Active Connectors</h3>
          {activeConnectors.length === 0 ? (
            <p style={{ color: "var(--text-secondary)" }}>No active connectors. Add one above.</p>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {activeConnectors.map((c) => (
                <div
                  key={c.id}
                  onClick={() => handleSelectConnector(c.id)}
                  style={{
                    padding: 12,
                    background: selectedConnector === c.id ? "var(--bg-tertiary)" : "var(--bg-secondary)",
                    borderRadius: 8,
                    cursor: "pointer",
                    border: selectedConnector === c.id ? "1px solid var(--accent)" : "1px solid var(--border)",
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                  }}
                >
                  <div>
                    <div style={{ color: "var(--text-primary)", fontWeight: 500 }}>{c.name}</div>
                    <div style={{ color: "var(--text-secondary)", fontSize: 12 }}>{c.category}</div>
                  </div>
                  <button
                    onClick={(e) => { e.stopPropagation(); handleRemoveConnector(c.id); }}
                    style={{ padding: "4px 8px", borderRadius: 4, border: "1px solid var(--border)", background: "transparent", color: "var(--text-secondary)", cursor: "pointer", fontSize: 12 }}
                  >
                    Remove
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Content browser / sync status */}
        {selectedConnector && (
          <div style={{ flex: 1 }}>
            {syncStatus && (
              <div style={{ padding: 12, background: "var(--bg-secondary)", borderRadius: 8, marginBottom: 12 }}>
                <h4 style={{ color: "var(--text-primary)", marginBottom: 8 }}>Sync Status</h4>
                <div style={{ color: "var(--text-secondary)", fontSize: 13 }}>
                  <div>Status: <span style={{ color: syncStatus.status === "synced" ? "#4caf50" : "#ff9800" }}>{syncStatus.status}</span></div>
                  <div>Items: {syncStatus.item_count}</div>
                  {syncStatus.last_sync && <div>Last sync: {new Date(syncStatus.last_sync).toLocaleString()}</div>}
                </div>
              </div>
            )}
            <h4 style={{ color: "var(--text-primary)", marginBottom: 8 }}>Content Items</h4>
            {contentItems.length === 0 ? (
              <p style={{ color: "var(--text-secondary)", fontSize: 13 }}>No content items found.</p>
            ) : (
              <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                {contentItems.map((item) => (
                  <div key={item.id} style={{ padding: 8, background: "var(--bg-secondary)", borderRadius: 6, fontSize: 13 }}>
                    <div style={{ color: "var(--text-primary)" }}>{item.title || item.path}</div>
                    <div style={{ color: "var(--text-secondary)", fontSize: 11 }}>{item.block_count} blocks</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
