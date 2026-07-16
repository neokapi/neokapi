import { useMemo, useState, type ReactNode } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Badge,
  Button,
  Card,
  cn,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
} from "@neokapi/ui-primitives";
import { AlertTriangle, Clock, Globe, Loader2, Palette, Plug, Plus, Trash2, Upload } from "./icons";
import { ConfirmDialog } from "./ConfirmDialog";
import { useApi } from "../context/ApiContext";
import type { ConnectorInfo } from "../types/api";

// ---------------------------------------------------------------------------
// Connector type catalog
// ---------------------------------------------------------------------------

interface ConnectorField {
  key: string;
  label: string;
  placeholder?: string;
  /** Renders a password input and never round-trips the value back to the UI. */
  secret?: boolean;
  required?: boolean;
  help?: string;
}

interface ConnectorType {
  type: string;
  label: string;
  /** Human category shown in the picker; the server echoes its own on the connector. */
  category: string;
  description: string;
  /** Docs page slug under the connectors section. */
  docsSlug: string;
  icon: ReactNode;
  fields: ConnectorField[];
}

// Only the remote/CMS connectors are configurable from the browser — local
// files and git checkouts are a server-side concern (kapi owns local files).
// Fields mirror the connector structs in bowrain/connector/*.go exactly.
const CONNECTOR_TYPES: ConnectorType[] = [
  {
    type: "wordpress",
    label: "WordPress",
    category: "CMS",
    description: "Read and write posts and pages on a WordPress site over its REST API.",
    docsSlug: "wordpress",
    icon: <Globe className="size-4" />,
    fields: [
      {
        key: "name",
        label: "Display name",
        placeholder: "Marketing site",
        help: "Optional label shown in this list.",
      },
      {
        key: "url",
        label: "Site URL",
        placeholder: "https://example.com",
        required: true,
      },
      { key: "username", label: "Username", placeholder: "editor" },
      {
        key: "password",
        label: "Application password",
        secret: true,
        help: "A WordPress application password, not your login password.",
      },
    ],
  },
  {
    type: "figma",
    label: "Figma",
    category: "Design",
    description: "Pull text layers from a Figma file so copy can be translated in place.",
    docsSlug: "figma",
    icon: <Palette className="size-4" />,
    fields: [
      {
        key: "name",
        label: "Display name",
        placeholder: "Product design file",
        help: "Optional label shown in this list.",
      },
      {
        key: "file_key",
        label: "File key",
        placeholder: "abc123XYZ",
        required: true,
        help: "The key in the file URL: figma.com/file/<file-key>/…",
      },
      {
        key: "token",
        label: "Access token",
        secret: true,
        required: true,
        help: "A personal access token with file read scope.",
      },
    ],
  },
  {
    type: "hubspot",
    label: "HubSpot",
    category: "Marketing",
    description: "Connect HubSpot marketing content for translation.",
    docsSlug: "hubspot",
    icon: <Plug className="size-4" />,
    fields: [
      {
        key: "name",
        label: "Display name",
        placeholder: "HubSpot portal",
        help: "Optional label shown in this list.",
      },
      {
        key: "api_key",
        label: "API key",
        secret: true,
        required: true,
        help: "A private app access token with content scopes.",
      },
    ],
  },
];

const CONNECTOR_TYPE_BY_ID = new Map(CONNECTOR_TYPES.map((t) => [t.type, t]));

function iconForCategory(category: string): ReactNode {
  switch (category.toLowerCase()) {
    case "cms":
      return <Globe className="size-4" />;
    case "design":
      return <Palette className="size-4" />;
    default:
      return <Plug className="size-4" />;
  }
}

// The server echoes lowercase category ids ("cms", …); render friendly labels.
const CATEGORY_LABELS: Record<string, string> = {
  cms: "CMS",
  design: "Design",
  marketing: "Marketing",
  tms: "TMS",
};

function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category.toLowerCase()] ?? category;
}

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------

export interface ConnectorsPanelProps {
  workspaceSlug: string;
  projectId: string;
  /** Base URL of the connectors docs section (no trailing slash). */
  docsBaseUrl?: string;
}

export function ConnectorsPanel({
  workspaceSlug,
  projectId,
  docsBaseUrl = "https://bowrain.cloud/docs/server/connectors",
}: ConnectorsPanelProps) {
  const api = useApi();
  const queryClient = useQueryClient();
  const connectorsKey = ["connectors", workspaceSlug];

  const [addOpen, setAddOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<ConnectorInfo | null>(null);

  const {
    data: connectors,
    isLoading,
    error: listError,
  } = useQuery({
    queryKey: connectorsKey,
    queryFn: () => api.listConnectors(workspaceSlug),
    staleTime: 15_000,
  });

  const removeMutation = useMutation({
    mutationFn: (connectorId: string) => api.removeConnector(workspaceSlug, connectorId),
    onSuccess: () => {
      setRemoveTarget(null);
      void queryClient.invalidateQueries({ queryKey: connectorsKey });
    },
  });

  const list = connectors ?? [];

  return (
    <div className="mx-auto w-full max-w-3xl p-4 md:p-6">
      <div className="mb-2 flex items-center justify-between gap-4">
        <div>
          <h1 className="text-foreground text-xl font-semibold">Connectors</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Link this project to a CMS, design, or marketing system. Connectors are shared across
            the workspace; fetching and publishing act on this project.
          </p>
        </div>
        {list.length > 0 && (
          <Button onClick={() => setAddOpen(true)} data-testid="add-connector-btn">
            <Plus className="size-4" />
            Add connector
          </Button>
        )}
      </div>

      {listError && (
        <Card className="border-destructive/40 bg-destructive/10 text-destructive mt-4 flex items-start gap-2 p-3 text-sm">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" />
          <span>Couldn't load connectors: {errorMessage(listError)}</span>
        </Card>
      )}

      {isLoading ? (
        <div className="mt-4 flex flex-col gap-3">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : list.length === 0 ? (
        <EmptyState onAdd={() => setAddOpen(true)} docsBaseUrl={docsBaseUrl} />
      ) : (
        <div className="mt-4 flex flex-col gap-3">
          {list.map((c) => (
            <ConnectorRow
              key={c.id}
              connector={c}
              workspaceSlug={workspaceSlug}
              projectId={projectId}
              onRemove={() => setRemoveTarget(c)}
            />
          ))}
        </div>
      )}

      <AddConnectorDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        workspaceSlug={workspaceSlug}
        docsBaseUrl={docsBaseUrl}
        onAdded={() => {
          setAddOpen(false);
          void queryClient.invalidateQueries({ queryKey: connectorsKey });
        }}
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRemoveTarget(null);
        }}
        title="Remove connector?"
        description={
          removeTarget
            ? `Remove "${removeTarget.name || removeTarget.id}"? This stops syncing; content already fetched into the project is kept.`
            : ""
        }
        confirmLabel="Remove"
        variant="destructive"
        loading={removeMutation.isPending}
        onConfirm={() => {
          if (removeTarget) removeMutation.mutate(removeTarget.id);
        }}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Connector row
// ---------------------------------------------------------------------------

function ConnectorRow({
  connector,
  workspaceSlug,
  projectId,
  onRemove,
}: {
  connector: ConnectorInfo;
  workspaceSlug: string;
  projectId: string;
  onRemove: () => void;
}) {
  const api = useApi();
  const queryClient = useQueryClient();
  const [publishConfirm, setPublishConfirm] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  // The list response carries only {id, name, category} — `name` is the
  // user-supplied label (may be empty), so fall back to the connector id.
  const displayName = connector.name || connector.id;

  const statusQuery = useQuery({
    queryKey: ["connector-status", workspaceSlug, connector.id],
    queryFn: () => api.getConnectorStatus(workspaceSlug, connector.id),
    staleTime: 30_000,
    retry: false,
  });

  const fetchMutation = useMutation({
    mutationFn: () => api.fetchConnector(workspaceSlug, connector.id, projectId),
    onMutate: () => {
      setFeedback(null);
      setActionError(null);
    },
    onSuccess: (res) => {
      setFeedback(`Fetched ${res.items_fetched} item${res.items_fetched === 1 ? "" : "s"}.`);
      void queryClient.invalidateQueries({
        queryKey: ["connector-status", workspaceSlug, connector.id],
      });
    },
    onError: (e) => setActionError(errorMessage(e)),
  });

  const publishMutation = useMutation({
    mutationFn: () => api.publishConnector(workspaceSlug, connector.id, projectId),
    onMutate: () => {
      setFeedback(null);
      setActionError(null);
    },
    onSuccess: () => {
      setPublishConfirm(false);
      setFeedback("Published this project's content.");
      void queryClient.invalidateQueries({
        queryKey: ["connector-status", workspaceSlug, connector.id],
      });
    },
    onError: (e) => {
      setPublishConfirm(false);
      setActionError(errorMessage(e));
    },
  });

  const status = statusQuery.data;
  const busy = fetchMutation.isPending || publishMutation.isPending;

  return (
    <Card className="p-4" data-testid={`connector-${connector.id}`}>
      <div className="flex items-start justify-between gap-4">
        <div className="flex min-w-0 items-start gap-3">
          <span className="text-muted-foreground bg-muted flex size-9 shrink-0 items-center justify-center rounded-md">
            {iconForCategory(connector.category)}
          </span>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-foreground truncate text-sm font-medium">{displayName}</span>
              {connector.category && (
                <Badge variant="secondary">{categoryLabel(connector.category)}</Badge>
              )}
            </div>
            <StatusLine
              loading={statusQuery.isLoading}
              error={statusQuery.error ? errorMessage(statusQuery.error) : null}
              status={status}
            />
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => fetchMutation.mutate()}
            data-testid={`connector-fetch-${connector.id}`}
          >
            {fetchMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            Fetch now
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => setPublishConfirm(true)}
            data-testid={`connector-publish-${connector.id}`}
          >
            <Upload className="size-4" />
            Publish
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={onRemove}
            aria-label={`Remove ${displayName}`}
            data-testid={`connector-remove-${connector.id}`}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>

      {feedback && <p className="text-muted-foreground mt-3 text-xs">{feedback}</p>}
      {actionError && (
        <p className="text-destructive mt-3 flex items-start gap-1.5 text-xs">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <span>{actionError}</span>
        </p>
      )}

      <ConfirmDialog
        open={publishConfirm}
        onOpenChange={setPublishConfirm}
        title="Publish to this connector?"
        description={`Send this project's current content out to "${displayName}". This writes to the remote system and overwrites matching content there.`}
        confirmLabel="Publish"
        loading={publishMutation.isPending}
        onConfirm={() => publishMutation.mutate()}
      />
    </Card>
  );
}

function StatusLine({
  loading,
  error,
  status,
}: {
  loading: boolean;
  error: string | null;
  status?: {
    lastSync: string;
    itemCount: number;
    pendingPull: number;
    pendingPush: number;
    errors: string[];
  };
}) {
  if (loading) return <Skeleton className="mt-1.5 h-3.5 w-40" />;
  if (error) {
    return <p className="text-muted-foreground mt-1 text-xs">Status unavailable</p>;
  }
  if (!status) return null;

  const lastSync = status.lastSync ? new Date(status.lastSync) : null;
  const lastSyncValid = lastSync && !Number.isNaN(lastSync.getTime()) && lastSync.getTime() > 0;

  return (
    <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
      <span className="inline-flex items-center gap-1">
        <Clock className="size-3.5" />
        {lastSyncValid ? `Synced ${lastSync.toLocaleString()}` : "Never synced"}
      </span>
      <span>
        {status.itemCount} item{status.itemCount === 1 ? "" : "s"}
      </span>
      {status.pendingPull > 0 && <Badge variant="secondary">{status.pendingPull} to pull</Badge>}
      {status.pendingPush > 0 && <Badge variant="secondary">{status.pendingPush} to push</Badge>}
      {status.errors.length > 0 && (
        <Badge variant="destructive">
          {status.errors.length} error{status.errors.length === 1 ? "" : "s"}
        </Badge>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

function EmptyState({ onAdd, docsBaseUrl }: { onAdd: () => void; docsBaseUrl: string }) {
  return (
    <Card className="mt-4 flex flex-col items-center gap-3 px-6 py-12 text-center">
      <span className="text-muted-foreground bg-muted flex size-12 items-center justify-center rounded-full">
        <Plug className="size-6" />
      </span>
      <h2 className="text-foreground text-base font-medium">No connectors yet</h2>
      <p className="text-muted-foreground max-w-md text-sm">
        Connect a CMS, design tool, or marketing platform to fetch its content into this project and
        publish translations back. Fetching and publishing are manual — automations can react once
        content is pushed.
      </p>
      <div className="mt-1 flex items-center gap-3">
        <Button onClick={onAdd} data-testid="add-connector-btn">
          <Plus className="size-4" />
          Add connector
        </Button>
        <a
          href={docsBaseUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="text-primary text-sm underline-offset-4 hover:underline"
        >
          Learn about connectors
        </a>
      </div>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Add dialog
// ---------------------------------------------------------------------------

function AddConnectorDialog({
  open,
  onOpenChange,
  workspaceSlug,
  docsBaseUrl,
  onAdded,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceSlug: string;
  docsBaseUrl: string;
  onAdded: () => void;
}) {
  const api = useApi();
  const [type, setType] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);

  const typeDef = type ? CONNECTOR_TYPE_BY_ID.get(type) : undefined;

  const reset = () => {
    setType("");
    setValues({});
    setError(null);
  };

  const handleOpenChange = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  const missingRequired = useMemo(() => {
    if (!typeDef) return true;
    return typeDef.fields.some((f) => f.required && !values[f.key]?.trim());
  }, [typeDef, values]);

  const addMutation = useMutation({
    mutationFn: () => {
      const config: Record<string, string> = {};
      for (const [k, v] of Object.entries(values)) {
        if (v.trim()) config[k] = v.trim();
      }
      return api.addConnector(workspaceSlug, type, config);
    },
    onSuccess: () => {
      reset();
      onAdded();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        data-testid="connector-form"
        onInteractOutside={(e: Event) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Add connector</DialogTitle>
          <DialogDescription>
            Connect a remote system to this workspace. Credentials are write-only: the server uses
            them to sync and this interface never displays them again.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-1">
          <div className="flex flex-col gap-1.5">
            <Label>Type</Label>
            <Select
              value={type}
              onValueChange={(v) => {
                setType(v);
                setValues({});
                setError(null);
              }}
            >
              <SelectTrigger data-testid="connector-type">
                <SelectValue placeholder="Select a system…" />
              </SelectTrigger>
              <SelectContent>
                {CONNECTOR_TYPES.map((t) => (
                  <SelectItem key={t.type} value={t.type}>
                    <span className="flex items-center gap-2">
                      {t.icon}
                      {t.label} · {t.category}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {typeDef && (
              <p className="text-muted-foreground text-xs">
                {typeDef.description}{" "}
                <a
                  href={`${docsBaseUrl}/${typeDef.docsSlug}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary underline-offset-4 hover:underline"
                >
                  Setup guide
                </a>
              </p>
            )}
          </div>

          {typeDef?.fields.map((f) => (
            <div key={f.key} className="flex flex-col gap-1.5">
              <Label htmlFor={`connector-field-${f.key}`}>
                {f.label}
                {f.required && <span className="text-destructive"> *</span>}
              </Label>
              <Input
                id={`connector-field-${f.key}`}
                type={f.secret ? "password" : "text"}
                autoComplete={f.secret ? "new-password" : "off"}
                value={values[f.key] ?? ""}
                placeholder={f.placeholder}
                onChange={(e) => setValues((prev) => ({ ...prev, [f.key]: e.target.value }))}
                data-testid={`connector-field-${f.key}`}
              />
              {f.help && <p className="text-muted-foreground text-xs">{f.help}</p>}
            </div>
          ))}

          {error && (
            <p
              className={cn(
                "text-destructive flex items-start gap-1.5 text-xs",
                !typeDef && "hidden",
              )}
            >
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
              <span>{error}</span>
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => addMutation.mutate()}
            disabled={!typeDef || missingRequired || addMutation.isPending}
          >
            {addMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            Add connector
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
