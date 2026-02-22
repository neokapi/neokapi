import { useState, useEffect, useCallback } from "react";
import { Button, Input, Badge, cn } from "@gokapi/ui";
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../../bindings/github.com/gokapi/gokapi/bowrain/apps/bowrain/backend/app.js";

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
      const status = await Backend.GetConnectorStatus(id);
      setSyncStatus(status);
    } catch {
      setSyncStatus(null);
    }
  };

  return (
    <div className="p-6 max-w-3xl">
      <h2 className="text-foreground text-xl font-semibold mb-6">Connectors</h2>

      {error && (
        <div className="p-3 bg-destructive/10 rounded-lg mb-4 text-destructive text-sm">
          {error}
        </div>
      )}

      {/* Add connector form */}
      <div className="p-4 bg-card rounded-lg border border-border mb-6">
        <h3 className="text-foreground font-medium mb-3">Add Connector</h3>
        <div className="flex gap-2 items-end flex-wrap">
          <div>
            <label className="text-xs text-muted-foreground block mb-1">Type</label>
            <select
              value={selectedType}
              onChange={(e) => setSelectedType(e.target.value)}
              className="px-3 py-2 rounded-md border border-border bg-background text-foreground text-sm"
            >
              <option value="">Select type...</option>
              {connectorTypes.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="text-xs text-muted-foreground block mb-1">Path</label>
            <Input
              type="text"
              value={configPath}
              onChange={(e) => setConfigPath(e.target.value)}
              placeholder="/path/to/content"
              className="w-[200px]"
            />
          </div>
          <div>
            <label className="text-xs text-muted-foreground block mb-1">Format</label>
            <Input
              type="text"
              value={configFormat}
              onChange={(e) => setConfigFormat(e.target.value)}
              placeholder="json, html..."
              className="w-[120px]"
            />
          </div>
          <Button
            onClick={handleAddConnector}
            disabled={!selectedType}
            size="sm"
          >
            Add
          </Button>
        </div>
      </div>

      {/* Active connectors */}
      <div className="flex gap-4">
        <div className="flex-1">
          <h3 className="text-foreground font-medium mb-3">Active Connectors</h3>
          {activeConnectors.length === 0 ? (
            <p className="text-muted-foreground text-sm">No active connectors. Add one above.</p>
          ) : (
            <div className="flex flex-col gap-2">
              {activeConnectors.map((c) => (
                <div
                  key={c.id}
                  onClick={() => handleSelectConnector(c.id)}
                  className={cn(
                    "p-3 rounded-lg cursor-pointer border flex justify-between items-center",
                    selectedConnector === c.id
                      ? "bg-accent border-primary"
                      : "bg-card border-border hover:bg-accent/50",
                  )}
                >
                  <div>
                    <div className="text-foreground font-medium text-sm">{c.name}</div>
                    <div className="text-muted-foreground text-xs">{c.category}</div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={(e: React.MouseEvent) => { e.stopPropagation(); handleRemoveConnector(c.id); }}
                  >
                    Remove
                  </Button>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Content browser / sync status */}
        {selectedConnector && (
          <div className="flex-1">
            {syncStatus && (
              <div className="p-3 bg-card rounded-lg border border-border mb-3">
                <h4 className="text-foreground font-medium text-sm mb-2">Sync Status</h4>
                <div className="text-muted-foreground text-xs space-y-0.5">
                  <div>Status: <Badge variant={syncStatus.status === "synced" ? "default" : "secondary"}>{syncStatus.status}</Badge></div>
                  <div>Items: {syncStatus.item_count}</div>
                  {syncStatus.last_sync && <div>Last sync: {new Date(syncStatus.last_sync).toLocaleString()}</div>}
                </div>
              </div>
            )}
            <h4 className="text-foreground font-medium text-sm mb-2">Content Items</h4>
            {contentItems.length === 0 ? (
              <p className="text-muted-foreground text-xs">No content items found.</p>
            ) : (
              <div className="flex flex-col gap-1">
                {contentItems.map((item) => (
                  <div key={item.id} className="p-2 bg-card rounded border border-border">
                    <div className="text-foreground text-xs">{item.title || item.path}</div>
                    <div className="text-muted-foreground text-[11px]">{item.block_count} blocks</div>
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
