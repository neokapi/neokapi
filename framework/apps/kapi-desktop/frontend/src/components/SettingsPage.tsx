import { useState, useEffect, useCallback } from "react";
import { Sun, Moon, Monitor } from "lucide-react";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { Card, CardContent, Tabs, TabsList, TabsTrigger, TabsContent, LoadingSpinner } from "@neokapi/ui-primitives";
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
  const [loading, setLoading] = useState(true);

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

  if (loading) {
    return (
      <LoadingSpinner text="Loading settings..." className="p-6" />
    );
  }

  return (
    <Tabs defaultValue="general" className="flex h-full flex-col">
      <TabsList variant="line">
        <TabsTrigger value="general">General</TabsTrigger>
        <TabsTrigger value="credentials">AI Credentials</TabsTrigger>
        <TabsTrigger value="plugins">Plugins</TabsTrigger>
      </TabsList>

      <TabsContent value="general" className="flex-1 overflow-auto">
        <div className="p-6">
          <h1 className="mb-6 text-xl font-semibold">Settings</h1>

          <div className="max-w-lg space-y-6">
            <Card>
              <CardContent className="p-4">
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
              </CardContent>
            </Card>

          </div>
        </div>
      </TabsContent>

      <TabsContent value="credentials" className="flex-1 overflow-auto">
        <CredentialsPage />
      </TabsContent>

      <TabsContent value="plugins" className="flex-1 overflow-auto">
        <PluginManager />
      </TabsContent>
    </Tabs>
  );
}
