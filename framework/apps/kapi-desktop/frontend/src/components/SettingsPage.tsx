import { useState, useEffect, useCallback } from "react";
import { Sun, Moon, Monitor, FolderCog, Loader2 } from "lucide-react";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { CredentialsPage } from "./CredentialsPage";
import { PluginManager } from "./PluginManager";

export type ThemeMode = "system" | "light" | "dark";

/** Apply theme to document — resolves "system" to the OS preference. */
export function applyTheme(mode: ThemeMode) {
  if (mode === "system") {
    const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    document.documentElement.classList.toggle("dark", prefersDark);
  } else {
    document.documentElement.classList.toggle("dark", mode === "dark");
  }
}

export function SettingsPage() {
  const [theme, setTheme] = useState<ThemeMode>("system");
  const [pluginDir, setPluginDir] = useState("");
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<"general" | "credentials" | "plugins">("general");

  const { showError } = useError();

  // Listen for OS theme changes when in "system" mode.
  useEffect(() => {
    if (theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = (e: MediaQueryListEvent) => {
      document.documentElement.classList.toggle("dark", e.matches);
    };
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, [theme]);

  useEffect(() => {
    api.getSettings().then((settings) => {
      if (settings) {
        const t = (settings.theme || "system") as ThemeMode;
        setTheme(t);
        setPluginDir(settings.plugin_dir || "");
      }
    }).catch((err) => {
      showError("Failed to load settings", err);
    }).finally(() => {
      setLoading(false);
    });
  }, [showError]);

  const handleThemeChange = useCallback(async (next: ThemeMode) => {
    setTheme(next);
    applyTheme(next);
    try { await api.setTheme(next); } catch (err) { showError("Failed to save theme", err); }
  }, [showError]);

  const handleSavePluginDir = async () => {
    try { await api.saveSettings({ theme, plugin_dir: pluginDir }); } catch (err) { showError("Failed to save settings", err); }
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
    <div className="flex h-full flex-col">
      {/* Tabs */}
      <div className="flex border-b border-border">
        <button
          onClick={() => setActiveTab("general")}
          className={`px-4 py-2.5 text-sm font-medium transition-colors ${
            activeTab === "general"
              ? "border-b-2 border-primary text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          General
        </button>
        <button
          onClick={() => setActiveTab("credentials")}
          className={`px-4 py-2.5 text-sm font-medium transition-colors ${
            activeTab === "credentials"
              ? "border-b-2 border-primary text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          AI Credentials
        </button>
        <button
          onClick={() => setActiveTab("plugins")}
          className={`px-4 py-2.5 text-sm font-medium transition-colors ${
            activeTab === "plugins"
              ? "border-b-2 border-primary text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          Plugins
        </button>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-auto">
        {activeTab === "general" && (
          <div className="p-6">
            <h1 className="mb-6 text-xl font-semibold">Settings</h1>

            <div className="max-w-lg space-y-6">
              <div className="rounded-lg border border-border p-4">
                <div className="mb-3 text-sm font-medium">Appearance</div>
                <div className="flex gap-2">
                  {([
                    { value: "system" as ThemeMode, icon: Monitor, label: "System" },
                    { value: "light" as ThemeMode, icon: Sun, label: "Light" },
                    { value: "dark" as ThemeMode, icon: Moon, label: "Dark" },
                  ]).map(({ value, icon: Icon, label }) => (
                    <button
                      key={value}
                      onClick={() => void handleThemeChange(value)}
                      className={`flex flex-1 flex-col items-center gap-1.5 rounded-md border p-3 text-xs font-medium transition-colors ${
                        theme === value
                          ? "border-primary bg-primary/10 text-primary"
                          : "border-border text-muted-foreground hover:border-primary/30 hover:text-foreground"
                      }`}
                    >
                      <Icon size={16} />
                      {label}
                    </button>
                  ))}
                </div>
                <p className="mt-2 text-[10px] text-muted-foreground">
                  System follows your operating system's appearance setting.
                </p>
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
        )}

        {activeTab === "credentials" && <CredentialsPage />}
        {activeTab === "plugins" && <PluginManager />}
      </div>
    </div>
  );
}
