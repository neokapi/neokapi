import { useState } from "react";
import { Globe, Link, Lock, ExternalLink } from "lucide-react";
import type { DashboardVisibility } from "../../types/api";

export interface PulseSettingsProps {
  workspaceSlug: string;
  visibility: DashboardVisibility;
  pulseBaseUrl?: string;
  onVisibilityChange: (visibility: DashboardVisibility) => Promise<void>;
}

const options: {
  value: DashboardVisibility;
  label: string;
  description: string;
  icon: typeof Globe;
}[] = [
  {
    value: "private",
    label: "Private",
    description: "Only workspace members can view the Pulse dashboard.",
    icon: Lock,
  },
  {
    value: "unlisted",
    label: "Unlisted",
    description: "Anyone with the link can view, but it won't appear in public listings.",
    icon: Link,
  },
  {
    value: "public",
    label: "Public",
    description: "Listed on the Pulse front page and discoverable by anyone.",
    icon: Globe,
  },
];

export function PulseSettings({
  workspaceSlug,
  visibility,
  pulseBaseUrl,
  onVisibilityChange,
}: PulseSettingsProps) {
  const [current, setCurrent] = useState<DashboardVisibility>(visibility);
  const [saving, setSaving] = useState(false);

  async function handleSelect(value: DashboardVisibility) {
    if (value === current || saving) return;
    setSaving(true);
    try {
      await onVisibilityChange(value);
      setCurrent(value);
    } finally {
      setSaving(false);
    }
  }

  const dashboardUrl = pulseBaseUrl
    ? `${pulseBaseUrl}/${workspaceSlug}`
    : `https://pulse.bowrain.cloud/${workspaceSlug}`;

  const isAccessible = current !== "private";

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-semibold">Dashboard visibility</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          Control who can see your workspace's Pulse activity dashboard.
        </p>
      </div>

      <div className="space-y-2" role="radiogroup" aria-label="Dashboard visibility">
        {options.map((opt) => {
          const Icon = opt.icon;
          const selected = current === opt.value;
          return (
            <button
              key={opt.value}
              role="radio"
              aria-checked={selected}
              disabled={saving}
              onClick={() => handleSelect(opt.value)}
              className={`flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors ${
                selected
                  ? "border-primary bg-primary/5"
                  : "border-border hover:border-muted-foreground/30 hover:bg-accent/50"
              } ${saving ? "opacity-60 cursor-wait" : "cursor-pointer"}`}
            >
              <div
                className={`mt-0.5 rounded-md p-1.5 ${
                  selected
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground"
                }`}
              >
                <Icon className="h-4 w-4" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{opt.label}</span>
                  {saving && selected && (
                    <span className="text-xs text-muted-foreground">Saving...</span>
                  )}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">{opt.description}</p>
              </div>
              <div
                className={`mt-1 h-4 w-4 shrink-0 rounded-full border-2 ${
                  selected ? "border-primary bg-primary" : "border-muted-foreground/40"
                }`}
              >
                {selected && (
                  <div className="flex h-full w-full items-center justify-center">
                    <div className="h-1.5 w-1.5 rounded-full bg-primary-foreground" />
                  </div>
                )}
              </div>
            </button>
          );
        })}
      </div>

      {isAccessible && (
        <div className="rounded-lg border bg-muted/50 p-3">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0">
              <p className="text-xs font-medium text-muted-foreground">Dashboard URL</p>
              <p className="mt-0.5 truncate text-sm font-mono">{dashboardUrl}</p>
            </div>
            <a
              href={dashboardUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="shrink-0 rounded-md p-2 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
              aria-label="Open Pulse dashboard"
            >
              <ExternalLink className="h-4 w-4" />
            </a>
          </div>
        </div>
      )}
    </div>
  );
}
