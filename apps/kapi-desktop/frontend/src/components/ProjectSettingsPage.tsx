import { useState, useEffect } from "react";
import { Globe, Plug, Cpu, FileType, AlertTriangle } from "lucide-react";
import {
  Card,
  CardContent,
  Label,
  Input,
  Switch,
  Badge,
  Separator,
  LocaleSelect,
  MultiLocaleSelect,
} from "@neokapi/ui-primitives";
import type { KapiProject, PluginSpec, PluginInfo, FormatDefaults } from "../types/api";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { useLocales } from "../hooks/useLocales";

export interface ProjectSettingsPageProps {
  project: KapiProject;
  onUpdate: (project: KapiProject) => void;
  tabID: string;
  /** Pre-loaded presets for Storybook — skips api.listPresets(). */
  presetList?: Array<{ name: string; description: string }>;
  /** Pre-loaded installed plugins for Storybook — skips api.listPlugins(). */
  installedPlugins?: PluginInfo[];
}

const ENCODING_OPTIONS = ["UTF-8", "UTF-16", "ISO-8859-1", "Windows-1252", "Shift_JIS", "EUC-JP"];

/** Default priority value used when "prefer plugin formats" is toggled on. */
const PLUGIN_PREFER_PRIORITY = 200;

/** Version pin modes for the UI. */
type VersionPin = "none" | "compatible" | "gte" | "exact";

function pinLabel(pin: VersionPin): string {
  switch (pin) {
    case "none":
      return "Any";
    case "compatible":
      return "Compatible (^)";
    case "gte":
      return "This or later (>=)";
    case "exact":
      return "Exact (=)";
  }
}

function parsePin(version?: string): { pin: VersionPin; base: string } {
  if (!version || version === "*") return { pin: "none", base: "" };
  if (version.startsWith("^")) return { pin: "compatible", base: version.slice(1) };
  if (version.startsWith(">=")) return { pin: "gte", base: version.slice(2) };
  return { pin: "exact", base: version };
}

function formatPin(pin: VersionPin, base: string): string | undefined {
  if (pin === "none" || !base) return undefined;
  if (pin === "compatible") return `^${base}`;
  if (pin === "gte") return `>=${base}`;
  return base;
}

export function ProjectSettingsPage({
  project,
  onUpdate,
  tabID,
  presetList: propPresets,
  installedPlugins: propInstalled,
}: ProjectSettingsPageProps) {
  const { showError } = useError();
  const defaults = project.defaults ?? {};
  const plugins = project.plugins ?? {};
  const formatDefaults = defaults.formats ?? {};
  const [presets, setPresets] = useState<Array<{ name: string; description: string }>>(
    propPresets ?? [],
  );
  const [installed, setInstalled] = useState<PluginInfo[]>(propInstalled ?? []);

  useEffect(() => {
    if (propPresets) return;
    api
      .listPresets()
      .then((p) => {
        if (p) setPresets(p);
      })
      .catch((err) => showError("Failed to load presets", err));
  }, [showError, propPresets]);

  useEffect(() => {
    if (propInstalled) return;
    api
      .listPlugins()
      .then((p) => {
        if (p) setInstalled(p);
      })
      .catch((err) => showError("Failed to load plugins", err));
  }, [showError, propInstalled]);

  const { locales } = useLocales();

  // Deduplicate installed plugins by name (keep highest version).
  const installedByName = new Map<string, PluginInfo>();
  for (const p of installed) {
    const existing = installedByName.get(p.name);
    if (!existing || p.version > existing.version) {
      installedByName.set(p.name, p);
    }
  }

  // Detect missing plugins (in project but not installed).
  const missingPlugins = Object.keys(plugins).filter((name) => !installedByName.has(name));

  // --- Update helpers ---

  const updateDefaults = (patch: Partial<typeof defaults>) => {
    onUpdate({ ...project, defaults: { ...defaults, ...patch } });
  };

  const updatePlugin = (name: string, patch: Partial<PluginSpec>) => {
    onUpdate({
      ...project,
      plugins: { ...plugins, [name]: { ...plugins[name], ...patch } },
    });
  };

  const updateFormatDefault = (fmt: string, patch: Partial<FormatDefaults>) => {
    const current = formatDefaults[fmt] ?? {};
    const updated = { ...current, ...patch };
    const isEmpty = !updated.preset && !updated.priority && !updated.config;
    const next = { ...formatDefaults };
    if (isEmpty) {
      delete next[fmt];
    } else {
      next[fmt] = updated;
    }
    updateDefaults({ formats: Object.keys(next).length ? next : undefined });
  };

  const hasFormatDefaults = Object.keys(formatDefaults).length > 0;

  return (
    <div className="p-6">
      <h1 className="mb-6 text-xl font-semibold">Project Settings</h1>

      <div className="max-w-2xl space-y-6">
        {/* Languages */}
        <section>
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            <Globe size={14} />
            Languages
          </h2>
          <Card>
            <CardContent className="space-y-4 p-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Label className="mb-1 block text-xs text-muted-foreground">
                    Source Language
                  </Label>
                  <LocaleSelect
                    value={defaults.source_language ?? ""}
                    onChange={(v) => updateDefaults({ source_language: v })}
                    locales={locales}
                    placeholder="Select source language..."
                  />
                </div>
                <div>
                  <Label className="mb-1 block text-xs text-muted-foreground">
                    Target Languages
                  </Label>
                  <MultiLocaleSelect
                    value={defaults.target_languages ?? []}
                    onChange={(v) => updateDefaults({ target_languages: v })}
                    locales={locales}
                  />
                </div>
              </div>
              <div>
                <Label className="mb-1 block text-xs text-muted-foreground">Locale Format</Label>
                <select
                  value={defaults.locale_format ?? ""}
                  onChange={(e) => updateDefaults({ locale_format: e.target.value || undefined })}
                  className="w-full max-w-xs rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
                >
                  <option value="">BCP-47 (default)</option>
                  <option value="posix">POSIX (underscores)</option>
                </select>
                <p className="mt-1 text-[10px] text-muted-foreground">
                  Determines locale code style in file paths and tool output.
                </p>
              </div>
            </CardContent>
          </Card>
        </section>

        {/* Preset */}
        {presets.length > 0 && (
          <section>
            <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              Preset
            </h2>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center gap-2">
                  <select
                    value={project.preset ?? ""}
                    onChange={async (e) => {
                      const name = e.target.value;
                      if (name) {
                        const updated = await api.applyPreset(tabID, name);
                        if (updated) onUpdate(updated);
                      } else {
                        onUpdate({ ...project, preset: undefined });
                      }
                    }}
                    className="flex-1 rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
                    aria-label="Framework preset"
                  >
                    <option value="">None (custom)</option>
                    {presets.map((p) => (
                      <option key={p.name} value={p.name}>
                        {p.name} — {p.description}
                      </option>
                    ))}
                  </select>
                  {project.preset && <Badge variant="secondary">{project.preset}</Badge>}
                </div>
                <p className="mt-2 text-[10px] text-muted-foreground">
                  Presets configure content patterns, flows, and defaults for common project types.
                </p>
              </CardContent>
            </Card>
          </section>
        )}

        {/* Plugins */}
        <section>
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            <Plug size={14} />
            Plugins
          </h2>

          {/* Missing plugins warning */}
          {missingPlugins.length > 0 && (
            <div className="mb-3 flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3">
              <AlertTriangle size={14} className="mt-0.5 shrink-0 text-amber-500" />
              <div className="flex-1">
                <p className="text-xs font-medium">
                  {missingPlugins.length === 1 ? "Missing plugin" : "Missing plugins"}:{" "}
                  {missingPlugins.join(", ")}
                </p>
                <p className="mt-0.5 text-[10px] text-muted-foreground">
                  This project requires plugins that are not installed. Install them from the
                  Plugins manager in app Settings.
                </p>
              </div>
            </div>
          )}

          <Card>
            <CardContent className="space-y-3 p-4">
              {/* Installed plugins as checkboxes */}
              {[...installedByName.entries()].map(([name, info]) => {
                const inProject = !!plugins[name];
                const spec = plugins[name] ?? {};
                const { pin, base } = parsePin(spec.framework_version);
                const isPreferred = (spec.format_priority ?? 0) >= PLUGIN_PREFER_PRIORITY;

                return (
                  <div key={name} className="space-y-2">
                    <div className="flex items-center justify-between">
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={inProject}
                          onChange={(e) => {
                            if (e.target.checked) {
                              onUpdate({
                                ...project,
                                plugins: { ...plugins, [name]: {} },
                              });
                            } else {
                              const next = { ...plugins };
                              delete next[name];
                              onUpdate({
                                ...project,
                                plugins: Object.keys(next).length ? next : undefined,
                              });
                            }
                          }}
                          className="rounded"
                        />
                        <span className="text-sm font-medium">{name}</span>
                        <Badge variant="secondary" className="text-[10px]">
                          {info.version}
                        </Badge>
                      </label>
                      {inProject && (
                        <div className="flex items-center gap-2">
                          <Label className="text-xs text-muted-foreground">Prefer formats</Label>
                          <Switch
                            checked={isPreferred}
                            onCheckedChange={(checked) =>
                              updatePlugin(name, {
                                format_priority: checked ? PLUGIN_PREFER_PRIORITY : undefined,
                              })
                            }
                          />
                        </div>
                      )}
                    </div>

                    {/* Version pinning (shown when plugin is in project) */}
                    {inProject && (
                      <div className="ml-6 flex items-center gap-2">
                        <Label className="text-xs text-muted-foreground">Version</Label>
                        <select
                          value={pin}
                          onChange={(e) => {
                            const newPin = e.target.value as VersionPin;
                            const ver = base || info.framework_version || info.version;
                            updatePlugin(name, {
                              framework_version: formatPin(newPin, ver),
                            });
                          }}
                          className="rounded border border-input bg-transparent px-1.5 py-0.5 text-xs outline-none"
                        >
                          {(["none", "compatible", "gte", "exact"] as VersionPin[]).map((v) => (
                            <option key={v} value={v}>
                              {pinLabel(v)}
                            </option>
                          ))}
                        </select>
                        {pin !== "none" && (
                          <Input
                            type="text"
                            value={base}
                            onChange={(e) =>
                              updatePlugin(name, {
                                framework_version: formatPin(pin, e.target.value),
                              })
                            }
                            placeholder={info.framework_version || info.version}
                            className="h-6 w-28 text-xs"
                          />
                        )}
                      </div>
                    )}

                    {info.description && (
                      <p className="ml-6 text-[10px] text-muted-foreground">{info.description}</p>
                    )}
                  </div>
                );
              })}

              {installedByName.size === 0 && (
                <p className="text-xs text-muted-foreground">
                  No plugins installed. Install plugins from the Plugins manager in app Settings.
                </p>
              )}
            </CardContent>
          </Card>
        </section>

        {/* Processing */}
        <section>
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            <Cpu size={14} />
            Processing
          </h2>
          <Card>
            <CardContent className="space-y-4 p-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Label htmlFor="concurrency" className="mb-1 block text-xs text-muted-foreground">
                    Concurrency
                  </Label>
                  <Input
                    id="concurrency"
                    type="number"
                    value={defaults.concurrency ?? ""}
                    onChange={(e) => {
                      const v = parseInt(e.target.value, 10);
                      updateDefaults({ concurrency: isNaN(v) ? undefined : v });
                    }}
                    placeholder="auto"
                    min={1}
                    max={64}
                  />
                  <p className="mt-1 text-[10px] text-muted-foreground">
                    Files processed concurrently. Leave empty for auto.
                  </p>
                </div>
                <div>
                  <Label
                    htmlFor="parallel-blocks"
                    className="mb-1 block text-xs text-muted-foreground"
                  >
                    Parallel Blocks
                  </Label>
                  <Input
                    id="parallel-blocks"
                    type="number"
                    value={defaults.parallel_blocks ?? ""}
                    onChange={(e) => {
                      const v = parseInt(e.target.value, 10);
                      updateDefaults({ parallel_blocks: isNaN(v) ? undefined : v });
                    }}
                    placeholder="auto"
                    min={1}
                    max={256}
                  />
                  <p className="mt-1 text-[10px] text-muted-foreground">
                    Blocks sent in parallel within a single tool step.
                  </p>
                </div>
              </div>

              <Separator />

              <div>
                <Label htmlFor="encoding" className="mb-1 block text-xs text-muted-foreground">
                  Default Encoding
                </Label>
                <select
                  id="encoding"
                  value={defaults.encoding ?? ""}
                  onChange={(e) => updateDefaults({ encoding: e.target.value || undefined })}
                  className="w-full max-w-xs rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
                >
                  <option value="">Auto-detect</option>
                  {ENCODING_OPTIONS.map((enc) => (
                    <option key={enc} value={enc}>
                      {enc}
                    </option>
                  ))}
                </select>
                <p className="mt-1 text-[10px] text-muted-foreground">
                  Character encoding for reading and writing files.
                </p>
              </div>
            </CardContent>
          </Card>
        </section>

        {/* Per-format defaults */}
        {hasFormatDefaults && (
          <section>
            <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              <FileType size={14} />
              Format Defaults
            </h2>
            <Card>
              <CardContent className="space-y-4 p-4">
                <p className="text-xs text-muted-foreground">
                  Default settings applied to all content items using a given format.
                </p>
                {Object.entries(formatDefaults).map(([fmt, fd], i) => (
                  <div key={fmt}>
                    {i > 0 && <Separator className="mb-4" />}
                    <div className="mb-2 flex items-center gap-2">
                      <Badge variant="outline">{fmt}</Badge>
                    </div>
                    <div className="grid grid-cols-3 gap-3">
                      <div>
                        <Label className="mb-0.5 block text-xs text-muted-foreground">Preset</Label>
                        <Input
                          type="text"
                          value={fd.preset ?? ""}
                          onChange={(e) =>
                            updateFormatDefault(fmt, { preset: e.target.value || undefined })
                          }
                          placeholder="default"
                          className="h-7 text-xs"
                        />
                      </div>
                      <div>
                        <Label className="mb-0.5 block text-xs text-muted-foreground">
                          Priority
                        </Label>
                        <Input
                          type="number"
                          value={fd.priority ?? ""}
                          onChange={(e) => {
                            const v = parseInt(e.target.value, 10);
                            updateFormatDefault(fmt, { priority: isNaN(v) ? undefined : v });
                          }}
                          placeholder="0"
                          className="h-7 text-xs"
                          min={0}
                        />
                      </div>
                      <div>
                        <Label className="mb-0.5 block text-xs text-muted-foreground">Config</Label>
                        <span className="text-xs text-muted-foreground">
                          {fd.config ? `${Object.keys(fd.config).length} keys` : "none"}
                        </span>
                      </div>
                    </div>
                  </div>
                ))}
              </CardContent>
            </Card>
          </section>
        )}
      </div>
    </div>
  );
}
