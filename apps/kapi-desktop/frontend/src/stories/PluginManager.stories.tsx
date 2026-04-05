import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, useCallback } from "react";
import { Download, RefreshCw, Search, Package, Loader2, CheckCircle2, Trash2 } from "lucide-react";
import { Button, Badge, Card, Input } from "@neokapi/ui-primitives";
import { PluginManager } from "../components/PluginManager";

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
        <Button
          variant="outline"
          size="sm"
          onClick={handleCheckUpdates}
          disabled={checking}
          aria-label="Check for updates"
        >
          {checking ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
          {checking ? "Checking..." : "Check Updates"}
        </Button>
      </div>

      <div className="relative mb-4">
        <Search size={14} className="absolute left-2.5 top-2.5 text-muted-foreground" />
        <Input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search plugins..."
          className="w-full pl-8"
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
          <Card key={plugin.name} className="p-4">
            <div className="flex items-center gap-3">
              <Package size={20} className="shrink-0 text-primary" />
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{plugin.name}</span>
                  <Badge variant="secondary">v{plugin.version}</Badge>
                  <Badge variant="secondary">{plugin.type}</Badge>
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
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => handleUninstall(plugin.name)}
                    className="hover:bg-destructive/10 hover:text-destructive"
                    aria-label={`Uninstall ${plugin.name}`}
                  >
                    <Trash2 size={12} />
                  </Button>
                </div>
              ) : (
                <Button
                  size="sm"
                  onClick={() => handleInstall(plugin.name)}
                  aria-label={`Install ${plugin.name}`}
                >
                  <Download size={12} />
                  Install
                </Button>
              )}
            </div>
          </Card>
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

/**
 * Real component with pre-loaded plugins (no Wails API calls).
 */
export const WithPlugins: StoryObj<typeof PluginManager> = {
  render: () => (
    <PluginManager
      plugins={[
        {
          name: "okapi",
          id: "okapi",
          version: "1.47.0",
          framework_version: "1.47.0",
          description: "Okapi Framework bridge — plugs into Okapi's filters and steps",
          type: "bridge",
          formats: [
            "okf_html",
            "okf_json",
            "okf_xliff",
            "okf_xml",
            "okf_properties",
            "okf_po",
            "okf_ts",
            "okf_dtd",
            "okf_regex",
          ],
        },
      ]}
    />
  ),
};

/**
 * Real component with empty plugins list.
 */
export const Empty: StoryObj<typeof PluginManager> = {
  render: () => <PluginManager plugins={[]} />,
};
