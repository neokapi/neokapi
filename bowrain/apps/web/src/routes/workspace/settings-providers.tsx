import { useEffect, useState } from "react";
import {
  useWorkspace,
  useProviderConfigs,
  useProviderApi,
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ConfirmDialog,
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
  SettingsSkeleton,
  type ProviderConfig,
  type ProviderConfigWithKey,
} from "@neokapi/ui";

// Provider types supported by the credential store (cli/credentials/store.go).
const PROVIDER_TYPES: { value: string; label: string }[] = [
  { value: "anthropic", label: "Anthropic" },
  { value: "openai", label: "OpenAI" },
  { value: "azureopenai", label: "Azure OpenAI" },
  { value: "gemini", label: "Gemini" },
  { value: "ollama", label: "Ollama" },
];

function providerLabel(type: string): string {
  return PROVIDER_TYPES.find((p) => p.value === type)?.label ?? type;
}

interface FormState {
  id: string;
  name: string;
  providerType: string;
  model: string;
  baseURL: string;
  apiKey: string;
}

const EMPTY_FORM: FormState = {
  id: "",
  name: "",
  providerType: "anthropic",
  model: "",
  baseURL: "",
  apiKey: "",
};

type TestState = { status: "idle" | "testing" | "ok" | "error"; message?: string };

export function SettingsProvidersRoute() {
  const { activeWorkspace } = useWorkspace();
  const { configs, loading, error, refresh } = useProviderConfigs();
  const { saveProviderConfig, deleteProviderConfig, testProviderConfig } = useProviderApi();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [test, setTest] = useState<TestState>({ status: "idle" });

  const [deleteTarget, setDeleteTarget] = useState<ProviderConfig | null>(null);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Providers — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  const openAdd = () => {
    setEditing(false);
    setForm(EMPTY_FORM);
    setSaveError(null);
    setTest({ status: "idle" });
    setDialogOpen(true);
  };

  // The API never returns saved API keys, so the field starts blank on edit;
  // an empty key on save preserves the stored secret.
  const openEdit = (cfg: ProviderConfig) => {
    setEditing(true);
    setForm({
      id: cfg.id,
      name: cfg.name,
      providerType: cfg.provider_type,
      model: cfg.model,
      baseURL: cfg.base_url,
      apiKey: "",
    });
    setSaveError(null);
    setTest({ status: "idle" });
    setDialogOpen(true);
  };

  const toPayload = (): ProviderConfigWithKey => ({
    id: form.id,
    name: form.name.trim(),
    provider_type: form.providerType,
    model: form.model.trim(),
    base_url: form.baseURL.trim(),
    api_key: form.apiKey,
  });

  const nameValid = form.name.trim().length > 0;
  // A new provider needs a key; on edit a blank key keeps the existing one.
  const canSave = nameValid && (editing || form.apiKey.length > 0) && !saving;

  const handleSave = async () => {
    if (!canSave) return;
    setSaving(true);
    setSaveError(null);
    try {
      await saveProviderConfig(toPayload());
      setDialogOpen(false);
      refresh();
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : "Failed to save provider");
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    setTest({ status: "testing" });
    try {
      await testProviderConfig(toPayload());
      setTest({ status: "ok", message: "Connection succeeded." });
    } catch (e) {
      setTest({
        status: "error",
        message: e instanceof Error ? e.message : "Connection test failed.",
      });
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteProviderConfig(deleteTarget.id);
      setDeleteTarget(null);
      refresh();
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="mx-auto w-full max-w-3xl py-4 space-y-4">
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4">
          <div>
            <CardTitle>Providers</CardTitle>
            <CardDescription>
              Translation and AI providers available to flows in this workspace. API keys are stored
              securely and never displayed.
            </CardDescription>
          </div>
          <Button onClick={openAdd}>Add provider</Button>
        </CardHeader>
        <CardContent>
          {loading && configs.length === 0 ? (
            <SettingsSkeleton />
          ) : error ? (
            <div className="py-8 text-center text-sm text-destructive">{error}</div>
          ) : configs.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              No providers configured yet.
            </div>
          ) : (
            <ul className="divide-y divide-border/50">
              {configs.map((cfg) => (
                <li key={cfg.id} className="flex items-center gap-3 py-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">{cfg.name}</span>
                      <Badge variant="secondary">{providerLabel(cfg.provider_type)}</Badge>
                    </div>
                    <div className="mt-0.5 text-xs text-muted-foreground">
                      {cfg.model || "default model"}
                      {cfg.base_url ? ` · ${cfg.base_url}` : ""}
                    </div>
                  </div>
                  <Button variant="outline" size="sm" onClick={() => openEdit(cfg)}>
                    Edit
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive"
                    onClick={() => setDeleteTarget(cfg)}
                  >
                    Delete
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Dialog
        open={dialogOpen}
        onOpenChange={(v: boolean) => {
          if (!v) setDialogOpen(false);
        }}
      >
        <DialogContent
          className="sm:max-w-[480px]"
          onInteractOutside={(e: Event) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>{editing ? "Edit provider" : "Add provider"}</DialogTitle>
            <DialogDescription>
              Configure a translation or AI provider for this workspace.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-2">
            <div>
              <Label className="text-muted-foreground">Name</Label>
              <Input
                value={form.name}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setForm((f) => ({ ...f, name: e.target.value }))
                }
                placeholder="Production Anthropic"
                autoFocus
                className="mt-1"
              />
            </div>

            <div>
              <Label className="text-muted-foreground">Type</Label>
              <Select
                value={form.providerType}
                onValueChange={(v: string) => setForm((f) => ({ ...f, providerType: v }))}
              >
                <SelectTrigger className="mt-1">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PROVIDER_TYPES.map((p) => (
                    <SelectItem key={p.value} value={p.value}>
                      {p.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div>
              <Label className="text-muted-foreground">Model</Label>
              <Input
                value={form.model}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setForm((f) => ({ ...f, model: e.target.value }))
                }
                placeholder="Optional — leave blank for the provider default"
                className="mt-1"
              />
            </div>

            <div>
              <Label className="text-muted-foreground">Base URL</Label>
              <Input
                value={form.baseURL}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setForm((f) => ({ ...f, baseURL: e.target.value }))
                }
                placeholder="Optional — custom or self-hosted endpoint"
                className="mt-1"
              />
            </div>

            <div>
              <Label className="text-muted-foreground">API key</Label>
              <Input
                type="password"
                value={form.apiKey}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setForm((f) => ({ ...f, apiKey: e.target.value }))
                }
                placeholder={editing ? "Leave blank to keep the current key" : "sk-…"}
                autoComplete="off"
                className="mt-1"
              />
            </div>

            {saveError && <p className="text-sm text-destructive">{saveError}</p>}
            {test.status === "ok" && <p className="text-sm text-foreground">{test.message}</p>}
            {test.status === "error" && <p className="text-sm text-destructive">{test.message}</p>}
          </div>

          <DialogFooter className="sm:justify-between">
            <Button
              variant="outline"
              onClick={() => void handleTest()}
              disabled={!nameValid || test.status === "testing" || saving}
            >
              {test.status === "testing" ? "Testing…" : "Test"}
            </Button>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={saving}>
                Cancel
              </Button>
              <Button onClick={() => void handleSave()} disabled={!canSave}>
                {saving ? "Saving…" : "Save"}
              </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null);
        }}
        title="Delete provider"
        description={`Remove "${deleteTarget?.name ?? ""}" and its stored API key. Flows relying on this provider will no longer run.`}
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={() => void handleDelete()}
      />
    </div>
  );
}
