import { useEffect } from "react";
import {
  useWorkspace,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
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

export function SettingsIndexRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Settings — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  return (
    <div className="mx-auto w-full max-w-3xl py-4">
      <Card>
        <CardHeader>
          <CardTitle>General</CardTitle>
          <CardDescription>Workspace details and identity</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid">
            <SettingsField label="Name" value={activeWorkspace.name} />
            <SettingsField label="Slug" value={activeWorkspace.slug} />
            <SettingsField
              label="Description"
              value={activeWorkspace.description || "No description"}
            />
            <SettingsField label="Your Role" value={activeWorkspace.role} />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
