import { useState, useEffect, useCallback } from "react";
import {
  Button,
  Input,
  Badge,
  cn,
  Label,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@neokapi/ui";
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js";

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

// CONNECTOR_FIELDS describes the config inputs each remote connector type
// needs. The desktop offers remote/CMS connectors only — local files are
// managed by kapi, not the desktop — so no connector takes a filesystem path.
const CONNECTOR_FIELDS: Record<
  string,
  { key: string; label: string; placeholder: string; secret?: boolean }[]
> = {
  wordpress: [
    { key: "url", label: "Site URL", placeholder: "https://example.com" },
    { key: "username", label: "Username", placeholder: "admin" },
    { key: "password", label: "Application Password", placeholder: "", secret: true },
  ],
  figma: [
    { key: "file_key", label: "File Key", placeholder: "abc123" },
    { key: "token", label: "Access Token", placeholder: "", secret: true },
  ],
  hubspot: [{ key: "api_key", label: "API Key", placeholder: "", secret: true }],
};

export function ConnectorPanel() {
  const [connectorTypes, setConnectorTypes] = useState<string[]>([]);
  const [activeConnectors, setActiveConnectors] = useState<ConnectorInfo[]>([]);
  const [showAddDialog, setShowAddDialog] = useState(false);
  const [selectedType, setSelectedType] = useState("");
  const [configValues, setConfigValues] = useState<Record<string, string>>({});
  const [contentItems, setContentItems] = useState<ContentItem[]>([]);
  const [selectedConnector, setSelectedConnector] = useState<string | null>(null);
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fields = selectedType ? (CONNECTOR_FIELDS[selectedType] ?? []) : [];

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
    void loadConnectorTypes();
    void loadActiveConnectors();
  }, [loadConnectorTypes, loadActiveConnectors]);

  const handleAddConnector = async () => {
    if (!selectedType) return;
    setError(null);
    try {
      const config: Record<string, string> = {};
      for (const [k, v] of Object.entries(configValues)) {
        if (v) config[k] = v;
      }
      await Backend.ConfigureConnector(selectedType, config);
      setSelectedType("");
      setConfigValues({});
      setShowAddDialog(false);
      void loadActiveConnectors();
    } catch (e) {
      setError(String(e));
    }
  };

  const handleAddDialogClose = (open: boolean) => {
    if (!open) {
      setSelectedType("");
      setConfigValues({});
    }
    setShowAddDialog(open);
  };

  const handleRemoveConnector = async (id: string) => {
    try {
      await Backend.RemoveConnector(id);
      if (selectedConnector === id) {
        setSelectedConnector(null);
        setContentItems([]);
        setSyncStatus(null);
      }
      void loadActiveConnectors();
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
    } catch {
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
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-foreground text-xl font-semibold">Connectors</h2>
        <Button onClick={() => setShowAddDialog(true)} data-testid="add-connector-btn">
          Add Connector
        </Button>
      </div>

      {error && (
        <div className="p-3 bg-destructive/10 rounded-lg mb-4 text-destructive text-sm">
          {error}
        </div>
      )}

      <Dialog open={showAddDialog} onOpenChange={handleAddDialogClose}>
        <DialogContent
          data-testid="connector-form"
          onInteractOutside={(e: Event) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>Add Connector</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4 py-2">
            <p className="text-muted-foreground text-xs">
              Connectors link your project to remote systems. Local files and project configuration
              are managed by kapi, not Bowrain Desktop.
            </p>
            <div className="flex flex-col gap-1">
              <Label className="text-muted-foreground">Type</Label>
              <Select value={selectedType} onValueChange={setSelectedType}>
                <SelectTrigger>
                  <SelectValue placeholder="Select type..." />
                </SelectTrigger>
                <SelectContent>
                  {connectorTypes.map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {fields.map((f) => (
              <div key={f.key} className="flex flex-col gap-1">
                <Label className="text-muted-foreground">{f.label}</Label>
                <Input
                  type={f.secret ? "password" : "text"}
                  value={configValues[f.key] ?? ""}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                    setConfigValues((prev) => ({ ...prev, [f.key]: e.target.value }))
                  }
                  placeholder={f.placeholder}
                  data-testid={`connector-field-${f.key}`}
                />
              </div>
            ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleAddDialogClose(false)}>
              Cancel
            </Button>
            <Button onClick={handleAddConnector} disabled={!selectedType}>
              Add
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

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
                    onClick={(e: React.MouseEvent) => {
                      e.stopPropagation();
                      void handleRemoveConnector(c.id);
                    }}
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
                  <div>
                    Status:{" "}
                    <Badge variant={syncStatus.status === "synced" ? "default" : "secondary"}>
                      {syncStatus.status}
                    </Badge>
                  </div>
                  <div>Items: {syncStatus.item_count}</div>
                  {syncStatus.last_sync && (
                    <div>Last sync: {new Date(syncStatus.last_sync).toLocaleString()}</div>
                  )}
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
                    <div className="text-muted-foreground text-[11px]">
                      {item.block_count} blocks
                    </div>
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
