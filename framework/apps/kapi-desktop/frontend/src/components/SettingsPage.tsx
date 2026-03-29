import { useState, useEffect } from "react";
import { Sun, Moon, FolderCog, Loader2 } from "lucide-react";
import { api } from "../hooks/useApi";

export function SettingsPage() {
  const [theme, setTheme] = useState<"light" | "dark">("dark");
  const [pluginDir, setPluginDir] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.getSettings().then((settings) => {
      if (settings) {
        setTheme(settings.theme === "light" ? "light" : "dark");
        setPluginDir(settings.plugin_dir || "");
      }
      setLoading(false);
    });
  }, []);

  const toggleTheme = async () => {
    const next = theme === "dark" ? "light" : "dark";
    setTheme(next);
    document.documentElement.classList.toggle("dark", next === "dark");
    await api.setTheme(next);
  };

  const handleSavePluginDir = async () => {
    await api.saveSettings({ theme, plugin_dir: pluginDir });
  };

  if (loading) {
    return (
      <div className="flex items-center gap-2 p-6 text-sm text-muted-foreground">
        <Loader2 size={16} className="animate-spin" />
        Loading settings...
      </div>
    );
  }

  return (
    <div className="p-6">
      <h1 className="mb-6 text-xl font-semibold">Settings</h1>

      <div className="max-w-lg space-y-6">
        <div className="flex items-center justify-between rounded-lg border border-border p-4">
          <div className="flex items-center gap-3">
            {theme === "dark" ? <Moon size={18} /> : <Sun size={18} />}
            <div>
              <div className="text-sm font-medium">Theme</div>
              <div className="text-xs text-muted-foreground">
                {theme === "dark" ? "Dark mode" : "Light mode"}
              </div>
            </div>
          </div>
          <button
            onClick={toggleTheme}
            className="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent"
            aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`}
          >
            Switch to {theme === "dark" ? "light" : "dark"}
          </button>
        </div>

        <div className="rounded-lg border border-border p-4">
          <div className="mb-3 flex items-center gap-3">
            <FolderCog size={18} />
            <div className="text-sm font-medium">Plugin Directory</div>
          </div>
          <input
            type="text"
            value={pluginDir}
            onChange={(e) => setPluginDir(e.target.value)}
            onBlur={handleSavePluginDir}
            placeholder="~/.config/kapi/plugins"
            className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            aria-label="Plugin directory path"
          />
          <p className="mt-1 text-xs text-muted-foreground">
            Override with KAPI_PLUGIN_DIR environment variable
          </p>
        </div>
      </div>
    </div>
  );
}
