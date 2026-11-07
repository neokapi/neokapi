import { useState, useEffect, useCallback } from "react";
import { Sun, Moon, Monitor, Languages, FlaskConical } from "lucide-react";
import { loadTranslations, setTranslations, t } from "@neokapi/i18n-react/runtime";
import { api, type VersionInfo } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { Button, Switch, LoadingSpinner } from "@neokapi/ui-primitives";
import { analyticsAvailable, setTelemetryEnabled } from "../analytics";
import { DevPseudoCard } from "./DevPseudoCard";
import { CredentialsPage } from "./CredentialsPage";
import { PluginManager } from "./PluginManager";
import { LocaleSettings } from "./LocaleSettings";
import { SubNavTabs } from "./SubNavTabs";

type SettingsTab = "general" | "credentials" | "plugins" | "locales";

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
  /** Whether the home's Start here steps are dismissed, as the app holds it. */
  startHereDismissed?: boolean;
  /** Bring Start here back. Absent writes the setting directly. */
  onRestoreStartHere?: () => void;
}

export function SettingsPage({
  theme: propTheme,
  uiLanguage: propLang,
  startHereDismissed: propStartHere,
  onRestoreStartHere,
}: SettingsPageProps = {}) {
  const [tab, setTab] = useState<SettingsTab>("general");
  const [theme, setTheme] = useState<ThemeMode>(propTheme ?? "system");
  const [uiLanguage, setUILanguage] = useState<UILanguage>(propLang ?? "en");
  const [loading, setLoading] = useState(!propTheme);
  const [version, setVersion] = useState<VersionInfo | null>(null);
  const [telemetryOn, setTelemetryOn] = useState(true);
  const [startHereRead, setStartHereRead] = useState(false);
  const startHereDismissed = propStartHere ?? startHereRead;

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
          setTelemetryOn(!settings.telemetry_disabled);
          setStartHereRead(!!settings.start_here_dismissed);
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

  const handleTelemetryChange = useCallback(
    async (next: boolean) => {
      setTelemetryOn(next);
      try {
        await setTelemetryEnabled(next);
      } catch (err) {
        showError("Failed to save telemetry setting", err);
      }
    },
    [showError],
  );

  const handleRestoreStartHere = useCallback(() => {
    if (onRestoreStartHere) {
      onRestoreStartHere();
      return;
    }
    setStartHereRead(false);
    api.restoreStartHere().catch((err) => showError("Failed to save the setting", err));
  }, [onRestoreStartHere, showError]);

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
    <div className="flex h-full flex-col">
      <SubNavTabs
        label={t("App Settings sections")}
        tabs={[
          { id: "general", label: t("General") },
          { id: "credentials", label: t("AI Models") },
          { id: "plugins", label: t("Plugins") },
          { id: "locales", label: t("Languages") },
        ]}
        active={tab}
        onSelect={setTab}
      />

      {tab === "general" && (
        <div role="tabpanel" className="flex-1 overflow-auto">
          <div className="p-6">
            <h1 className="mb-6 text-xl font-semibold">App settings</h1>

            {/* Labelled rows, one control each: the settings shape in
                page-patterns.md. */}
            <div className="max-w-2xl divide-y divide-border/60 border-y border-border/60">
              <SettingRow label={t("Appearance")}>
                <Segmented
                  label={t("Appearance")}
                  value={theme}
                  onChange={(v) => void handleThemeChange(v as ThemeMode)}
                  options={[
                    { value: "system", icon: Monitor, label: t("System") },
                    { value: "light", icon: Sun, label: t("Light") },
                    { value: "dark", icon: Moon, label: t("Dark") },
                  ]}
                />
              </SettingRow>

              <SettingRow label={t("Language")}>
                <Segmented
                  label={t("Language")}
                  value={uiLanguage}
                  onChange={(v) => void handleLanguageChange(v as UILanguage)}
                  options={UI_LANGUAGES}
                />
              </SettingRow>

              <SettingRow label={t("Start here")}>
                <Button
                  size="sm"
                  variant="outline"
                  data-slot="restore-start-here"
                  disabled={!startHereDismissed}
                  onClick={handleRestoreStartHere}
                >
                  {t("Show it again")}
                </Button>
              </SettingRow>

              {/* Usage statistics (D1 opt-out) — only meaningful in builds that
                carry an analytics key; keyless builds never emit anything. */}
              {analyticsAvailable() && (
                <SettingRow
                  label={t("Usage statistics")}
                  meta={t(
                    "Screen views and feature use; never your content, project names or paths.",
                  )}
                >
                  <Switch
                    aria-label={t("Share anonymous usage statistics")}
                    checked={telemetryOn}
                    onCheckedChange={(checked) => void handleTelemetryChange(checked)}
                  />
                </SettingRow>
              )}

              {version && (
                <SettingRow label={t("Version")}>
                  <span
                    translate="no"
                    className="font-mono text-xs text-muted-foreground"
                    data-testid="version-info"
                  >
                    {version.version || "dev"} · {version.commit || "unknown"} ·{" "}
                    {version.build_date || "unknown"}
                  </span>
                </SettingRow>
              )}
            </div>

            <div className="mt-6 max-w-2xl">
              <DevPseudoCard />
            </div>
          </div>
        </div>
      )}

      {tab === "credentials" && (
        <div role="tabpanel" className="flex-1 overflow-auto">
          <CredentialsPage />
        </div>
      )}

      {tab === "plugins" && (
        <div role="tabpanel" className="flex-1 overflow-auto">
          <PluginManager />
        </div>
      )}

      {tab === "locales" && (
        <div role="tabpanel" className="flex-1 overflow-auto">
          <LocaleSettings />
        </div>
      )}
    </div>
  );
}

/** One setting: its label on the left, its one control on the right. */
function SettingRow({
  label,
  meta,
  children,
}: {
  label: string;
  meta?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-12 items-center justify-between gap-6 py-2.5">
      <div className="min-w-0">
        <div className="text-sm">{label}</div>
        {meta && <div className="text-[11px] text-muted-foreground">{meta}</div>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

/** A small segmented control: one choice among a few, each with an icon. */
function Segmented({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string; icon: React.ComponentType<{ size?: number }> }[];
}) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="flex rounded-md border border-border p-0.5"
    >
      {options.map(({ value: v, label: l, icon: Icon }) => (
        <button
          key={v}
          type="button"
          role="radio"
          // Each label arrives translated, so the button is not extracted.
          translate="no"
          aria-checked={value === v}
          onClick={() => onChange(v)}
          className={`flex items-center gap-1.5 rounded px-2.5 py-1 text-xs transition-colors focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none ${
            value === v
              ? "bg-accent font-medium text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          <Icon size={13} />
          {l}
        </button>
      ))}
    </div>
  );
}
