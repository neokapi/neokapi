import { useState, useEffect, useCallback } from "react";
import { Sun, Moon, Monitor, Languages, FlaskConical } from "lucide-react";
import { loadTranslations, setTranslations, t } from "@neokapi/kapi-react/runtime";
import { api, type VersionInfo } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import {
  Card,
  CardContent,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  LoadingSpinner,
} from "@neokapi/ui-primitives";
import { DevPseudoCard } from "./DevPseudoCard";
import { CredentialsPage } from "./CredentialsPage";
import { PluginManager } from "./PluginManager";
import { LocaleSettings } from "./LocaleSettings";

export type ThemeMode = "system" | "light" | "dark";
export type UILanguage = "en" | "nb" | "qps";

const UI_LANGUAGES: { value: UILanguage; label: string; icon: typeof Languages }[] = [
  { value: "en", label: t("English", "UI Language"), icon: Languages },
  { value: "nb", label: t("Norsk (bokmål)", "UI Language"), icon: Languages },
  { value: "qps", label: t("Pseudo English (qps)", "UI Language"), icon: FlaskConical },
];

/** Apply theme to document — resolves "system" to the OS preference. */
export function applyTheme(mode: ThemeMode) {
  if (mode === "system") {
    const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    document.documentElement.classList.toggle("dark", prefersDark);
  } else {
    document.documentElement.classList.toggle("dark", mode === "dark");
  }
}

export interface SettingsPageProps {
  /** Pre-loaded theme for Storybook — skips api.getSettings(). */
  theme?: ThemeMode;
  /** Pre-loaded UI language for Storybook — skips api.getSettings(). */
  uiLanguage?: UILanguage;
}

export function SettingsPage({ theme: propTheme, uiLanguage: propLang }: SettingsPageProps = {}) {
  const [theme, setTheme] = useState<ThemeMode>(propTheme ?? "system");
  const [uiLanguage, setUILanguage] = useState<UILanguage>(propLang ?? "en");
  const [loading, setLoading] = useState(!propTheme);
  const [version, setVersion] = useState<VersionInfo | null>(null);

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
    if (propTheme) return;
    api
      .getSettings()
      .then((settings) => {
        if (settings) {
          const t = (settings.theme || "system") as ThemeMode;
          setTheme(t);
          const l = (settings.ui_language || "en") as UILanguage;
          setUILanguage(l);
        }
      })
      .catch((err) => {
        showError("Failed to load settings", err);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [showError, propTheme]);

  // Load build version info for the About card. Best-effort: outside Wails
  // (Storybook) the call returns null and the card simply doesn't render.
  useEffect(() => {
    api
      .getVersion()
      .then((v) => {
        if (v) setVersion(v);
      })
      .catch(() => {
        /* version is non-essential; ignore */
      });
  }, []);

  const handleThemeChange = useCallback(
    async (next: ThemeMode) => {
      setTheme(next);
      applyTheme(next);
      try {
        await api.setTheme(next);
      } catch (err) {
        showError("Failed to save theme", err);
      }
    },
    [showError],
  );

  const handleLanguageChange = useCallback(
    async (next: UILanguage) => {
      setUILanguage(next);
      try {
        if (next === "en") {
          setTranslations("en", {});
        } else {
          await loadTranslations(next, `/translations/${next}.json`);
        }
        await api.setUILanguage(next);
      } catch (err) {
        showError("Failed to save language", err);
      }
    },
    [showError],
  );

  if (loading) {
    return <LoadingSpinner text="Loading settings..." className="p-6" />;
  }

  return (
    <Tabs defaultValue="general" className="flex h-full flex-col">
      <TabsList variant="line">
        <TabsTrigger value="general">General</TabsTrigger>
        <TabsTrigger value="credentials">AI Credentials</TabsTrigger>
        <TabsTrigger value="plugins">Plugins</TabsTrigger>
        <TabsTrigger value="locales">Locales</TabsTrigger>
      </TabsList>

      <TabsContent value="general" className="flex-1 overflow-auto">
        <div className="p-6">
          <h1 className="mb-6 text-xl font-semibold">Settings</h1>

          <div className="max-w-lg space-y-6">
            <Card>
              <CardContent className="p-4">
                <div className="mb-3 text-sm font-medium">Appearance</div>
                <div className="flex gap-2">
                  {[
                    { value: "system" as ThemeMode, icon: Monitor, label: t("System") },
                    { value: "light" as ThemeMode, icon: Sun, label: t("Light") },
                    { value: "dark" as ThemeMode, icon: Moon, label: t("Dark") },
                  ].map(({ value, icon: Icon, label }) => (
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

            <Card>
              <CardContent className="p-4">
                <div className="mb-3 text-sm font-medium">Language</div>
                <div className="flex gap-2">
                  {UI_LANGUAGES.map(({ value, icon: Icon, label }) => (
                    <button
                      key={value}
                      onClick={() => void handleLanguageChange(value)}
                      className={`flex flex-1 flex-col items-center gap-1.5 rounded-md border p-3 text-xs font-medium transition-colors ${
                        uiLanguage === value
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
                  Pseudo English expands strings and adds accents — useful for localization QA.
                </p>
              </CardContent>
            </Card>

            <DevPseudoCard />

            {version && (
              <Card>
                <CardContent className="p-4">
                  <div className="mb-3 text-sm font-medium">{t("About")}</div>
                  <dl
                    className="grid grid-cols-[6rem_1fr] gap-x-4 gap-y-1.5 text-sm"
                    data-testid="version-info"
                  >
                    <dt className="text-muted-foreground">{t("Version")}</dt>
                    <dd className="font-mono text-xs">{version.version || "dev"}</dd>
                    <dt className="text-muted-foreground">{t("Commit")}</dt>
                    <dd className="font-mono text-xs">{version.commit || "unknown"}</dd>
                    <dt className="text-muted-foreground">{t("Build date")}</dt>
                    <dd className="font-mono text-xs">{version.build_date || "unknown"}</dd>
                  </dl>
                  <p className="mt-3 text-[10px] text-muted-foreground">
                    © {new Date().getFullYear()} neokapi
                  </p>
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      </TabsContent>

      <TabsContent value="credentials" className="flex-1 overflow-auto">
        <CredentialsPage />
      </TabsContent>

      <TabsContent value="plugins" className="flex-1 overflow-auto">
        <PluginManager />
      </TabsContent>

      <TabsContent value="locales" className="flex-1 overflow-auto">
        <LocaleSettings />
      </TabsContent>
    </Tabs>
  );
}
