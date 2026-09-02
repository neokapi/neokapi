import { useEffect, useState } from "react";
import {
  useWorkspace,
  useApi,
  useAnalytics,
  AnalyticsEvents,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  ErrorNotice,
  Input,
  Label,
  Textarea,
} from "@neokapi/ui";

function SettingsField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="grid grid-cols-3 gap-2 items-baseline py-2.5 border-b border-border/50 last:border-b-0">
      <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
        {label}
      </div>
      <div className={`col-span-2 text-sm text-foreground ${mono ? "font-mono text-xs" : ""}`}>
        {value}
      </div>
    </div>
  );
}

/**
 * General identity card. When `editable` (admin/owner), Name and Description can
 * be renamed via updateWorkspace; slug and role stay immutable (slug rename has
 * its own reserved-slug flow and lives elsewhere).
 */
function GeneralIdentityCard({ editable }: { editable: boolean }) {
  const { activeWorkspace, setActiveWorkspace } = useWorkspace();
  const adapter = useApi();
  const { capture } = useAnalytics();

  const [name, setName] = useState(activeWorkspace?.name ?? "");
  const [description, setDescription] = useState(activeWorkspace?.description ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);

  // Re-seed the form when the active workspace changes (e.g. switching orgs).
  useEffect(() => {
    setName(activeWorkspace?.name ?? "");
    setDescription(activeWorkspace?.description ?? "");
    setError(null);
  }, [activeWorkspace?.slug, activeWorkspace?.name, activeWorkspace?.description]);

  if (!activeWorkspace) return null;

  const trimmedName = name.trim();
  const trimmedDescription = description.trim();
  const dirty =
    trimmedName !== activeWorkspace.name ||
    trimmedDescription !== (activeWorkspace.description ?? "");
  const canSave = editable && dirty && trimmedName.length > 0 && !saving;

  const handleSave = async () => {
    if (!canSave) return;
    setSaving(true);
    setError(null);
    try {
      const updated = await adapter.updateWorkspace(activeWorkspace.slug, {
        name: trimmedName,
        description: trimmedDescription,
      });
      capture(AnalyticsEvents.settingsSaved, { section: "general" });
      setActiveWorkspace(updated);
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  };

  const handleReset = () => {
    setName(activeWorkspace.name);
    setDescription(activeWorkspace.description ?? "");
    setError(null);
  };

  if (!editable) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>General</CardTitle>
          <CardDescription>Workspace details and identity</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid">
            <SettingsField label="Name" value={activeWorkspace.name} />
            <SettingsField label="Slug" value={activeWorkspace.slug} mono />
            <SettingsField
              label="Description"
              value={activeWorkspace.description || "No description"}
            />
            <SettingsField label="Your Role" value={activeWorkspace.role} />
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>General</CardTitle>
        <CardDescription>Workspace details and identity</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="ws-name">Name</Label>
          <Input
            id="ws-name"
            value={name}
            maxLength={120}
            onChange={(e) => setName(e.target.value)}
            placeholder="Workspace name"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="ws-description">Description</Label>
          <Textarea
            id="ws-description"
            value={description}
            rows={3}
            maxLength={500}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What this workspace is for (optional)"
          />
        </div>
        <div className="grid">
          <SettingsField label="Slug" value={activeWorkspace.slug} mono />
          <SettingsField label="Your Role" value={activeWorkspace.role} />
        </div>
        {error != null && <ErrorNotice error={error} />}
        <div className="flex items-center justify-end gap-2">
          {dirty && (
            <Button variant="ghost" onClick={handleReset} disabled={saving}>
              Reset
            </Button>
          )}
          <Button onClick={handleSave} disabled={!canSave}>
            {saving ? "Saving…" : "Save changes"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function SettingsIndexRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Settings · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  const isAdmin = activeWorkspace.role === "owner" || activeWorkspace.role === "admin";

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6 py-4" data-testid="settings-heading">
      <GeneralIdentityCard editable={isAdmin} />
    </div>
  );
}
