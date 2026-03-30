import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, useCallback } from "react";
import { Download, RefreshCw, Search, Package, Loader2, CheckCircle2, Trash2 } from "lucide-react";

interface Plugin {
  name: string;
  version: string;
  description: string;
  type: string;
  installed: boolean;
  installing?: boolean;
  progress?: number;
}

const REGISTRY_PLUGINS: Plugin[] = [
  {
    name: "okapi",
    version: "1.47.0",
    description: "Okapi Framework bridge — plugs into Okapi's filters and steps",
    type: "bridge",
    installed: true,
  },
  {
    name: "deepl",
    version: "0.3.0",
    description: "DeepL MT provider for machine translation",
    type: "tool",
    installed: false,
  },
  {
    name: "memoq-xliff",
    version: "1.2.0",
    description: "memoQ XLIFF format with custom extensions",
    type: "format",
    installed: false,
  },
  {
    name: "sdl-trados",
    version: "0.9.0",
    description: "SDL Trados Studio package format support",
    type: "format",
    installed: false,
  },
  {
    name: "google-mt",
    version: "1.0.0",
    description: "Google Cloud Translation API provider",
    type: "tool",
    installed: false,
  },
];

function SimulatedPluginManager() {
  const [plugins, setPlugins] = useState<Plugin[]>(REGISTRY_PLUGINS);
  const [search, setSearch] = useState("");
  const [checking, setChecking] = useState(false);
  const [updateMessage, setUpdateMessage] = useState<string | null>(null);

  const handleInstall = useCallback((name: string) => {
    setPlugins((prev) =>
      prev.map((p) => (p.name === name ? { ...p, installing: true, progress: 0 } : p)),
    );

    // Simulate download progress
    let progress = 0;
    const interval = setInterval(() => {
      progress += Math.random() * 25 + 5;
      if (progress >= 100) {
        clearInterval(interval);
        setPlugins((prev) =>
          prev.map((p) =>
            p.name === name ? { ...p, installing: false, installed: true, progress: undefined } : p,
          ),
        );
      } else {
        setPlugins((prev) =>
          prev.map((p) => (p.name === name ? { ...p, progress: Math.min(progress, 95) } : p)),
        );
      }
    }, 300);
  }, []);

  const handleUninstall = useCallback((name: string) => {
    setPlugins((prev) => prev.map((p) => (p.name === name ? { ...p, installed: false } : p)));
  }, []);

  const handleCheckUpdates = useCallback(async () => {
    setChecking(true);
    setUpdateMessage(null);
    await new Promise((r) => setTimeout(r, 1500));
    setChecking(false);
    setUpdateMessage("All plugins are up to date");
  }, []);

  const filtered = plugins.filter(
    (p) =>
      p.name.toLowerCase().includes(search.toLowerCase()) ||
      p.description.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Plugins</h1>
        <button
          onClick={handleCheckUpdates}
          disabled={checking}
          className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent disabled:opacity-50"
          aria-label="Check for updates"
        >
          {checking ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
          {checking ? "Checking..." : "Check Updates"}
        </button>
      </div>

      <div className="relative mb-4">
        <Search size={14} className="absolute left-2.5 top-2.5 text-muted-foreground" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search plugins..."
          className="w-full rounded-md border border-input bg-transparent py-2 pl-8 pr-3 text-sm outline-none focus:ring-1 focus:ring-ring"
        />
      </div>

      {updateMessage && (
        <div className="mb-4 flex items-center gap-2 rounded-md bg-accent/50 px-3 py-2 text-xs">
          <CheckCircle2 size={14} className="text-green-500" />
          {updateMessage}
        </div>
      )}

      <div className="space-y-2">
        {filtered.map((plugin) => (
          <div key={plugin.name} className="rounded-lg border border-border p-4">
            <div className="flex items-center gap-3">
              <Package size={20} className="shrink-0 text-primary" />
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{plugin.name}</span>
                  <span className="rounded bg-accent px-1.5 py-0.5 text-xs">v{plugin.version}</span>
                  <span className="rounded bg-accent px-1.5 py-0.5 text-xs">{plugin.type}</span>
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">{plugin.description}</p>
              </div>

              {plugin.installing ? (
                <div className="flex w-32 flex-col items-end gap-1">
                  <div className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Loader2 size={12} className="animate-spin" />
                    Installing... {Math.round(plugin.progress ?? 0)}%
                  </div>
                  <div className="h-1.5 w-full overflow-hidden rounded-full bg-accent">
                    <div
                      className="h-full rounded-full bg-primary transition-all duration-300"
                      style={{ width: `${plugin.progress ?? 0}%` }}
                    />
                  </div>
                </div>
              ) : plugin.installed ? (
                <div className="flex items-center gap-2">
                  <span className="flex items-center gap-1 text-xs text-green-500">
                    <CheckCircle2 size={12} />
                    Installed
                  </span>
                  <button
                    onClick={() => handleUninstall(plugin.name)}
                    className="rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                    aria-label={`Uninstall ${plugin.name}`}
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              ) : (
                <button
                  onClick={() => handleInstall(plugin.name)}
                  className="flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
                  aria-label={`Install ${plugin.name}`}
                >
                  <Download size={12} />
                  Install
                </button>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

const meta: Meta<typeof SimulatedPluginManager> = {
  title: "Interactions/Plugin Installation",
  component: SimulatedPluginManager,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Demonstrates plugin discovery, installation with download progress, update checking, and uninstallation.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SimulatedPluginManager>;

export const Default: Story = {};
