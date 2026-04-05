import { useState, useEffect } from "react";
import { Globe, Plug, Cpu, FileType, X } from "lucide-react";
import {
  Card,
  CardContent,
  Label,
  Input,
  Switch,
  Badge,
  Button,
  Separator,
} from "@neokapi/ui-primitives";
import type { KapiProject, PluginSpec, FormatDefaults } from "../types/api";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";

export interface ProjectSettingsPageProps {
  project: KapiProject;
  onUpdate: (project: KapiProject) => void;
  tabID: string;
  /** Pre-loaded presets for Storybook — skips api.listPresets(). */
  presetList?: Array<{ name: string; description: string }>;
}

const ENCODING_OPTIONS = ["UTF-8", "UTF-16", "ISO-8859-1", "Windows-1252", "Shift_JIS", "EUC-JP"];

/** Default priority value used when "prefer plugin formats" is toggled on. */
const PLUGIN_PREFER_PRIORITY = 200;

export function ProjectSettingsPage({
  project,
  onUpdate,
  tabID,
  presetList: propPresets,
}: ProjectSettingsPageProps) {
  const { showError } = useError();
  const defaults = project.defaults ?? {};
  const plugins = project.plugins ?? {};
  const formatDefaults = defaults.formats ?? {};
  const [presets, setPresets] = useState<Array<{ name: string; description: string }>>(
    propPresets ?? [],
  );

  useEffect(() => {
    if (propPresets) return;
    api
      .listPresets()
      .then((p) => {
        if (p) setPresets(p);
      })
      .catch((err) => showError("Failed to load presets", err));
  }, [showError, propPresets]);

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

  const hasPlugins = Object.keys(plugins).length > 0;
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
                  <Label htmlFor="source-lang" className="mb-1 block text-xs text-muted-foreground">
                    Source Language
                  </Label>
                  <Input
                    id="source-lang"
                    type="text"
                    value={defaults.source_language ?? ""}
                    onChange={(e) => updateDefaults({ source_language: e.target.value })}
                    placeholder="en-US"
                  />
                </div>
                <div>
                  <Label className="mb-1 block text-xs text-muted-foreground">
                    Target Languages
                  </Label>
                  <div className="flex flex-wrap items-center gap-1.5 rounded border border-input bg-transparent px-2 py-1.5">
                    {(defaults.target_languages ?? []).map((lang) => (
                      <span
                        key={lang}
                        className="flex items-center gap-1 rounded bg-accent px-2 py-0.5 text-xs"
                      >
                        {lang}
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() =>
                            updateDefaults({
                              target_languages: defaults.target_languages?.filter(
                                (l) => l !== lang,
                              ),
                            })
                          }
                          className="ml-0.5 h-4 w-4 rounded-full hover:text-destructive"
                          aria-label={`Remove ${lang}`}
                        >
                          <X size={10} />
                        </Button>
                      </span>
                    ))}
                    <input
                      type="text"
                      placeholder={
                        defaults.target_languages?.length ? "" : "Add language (e.g. fr-FR)"
                      }
                      className="min-w-[80px] flex-1 bg-transparent text-sm outline-none"
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === ",") {
                          e.preventDefault();
                          const val = e.currentTarget.value.trim();
                          if (val && !defaults.target_languages?.includes(val)) {
                            updateDefaults({
                              target_languages: [...(defaults.target_languages ?? []), val],
                            });
                            e.currentTarget.value = "";
                          }
                        }
                      }}
                    />
                  </div>
                </div>
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
          <Card>
            <CardContent className="space-y-4 p-4">
              <div className="flex flex-wrap items-center gap-1.5 rounded border border-input bg-transparent px-2 py-1.5">
                {Object.entries(plugins).map(([name, spec]) => (
                  <span
                    key={name}
                    className="flex items-center gap-1 rounded bg-accent px-2 py-0.5 text-xs"
                  >
                    {name}
                    {spec.version && <span className="text-muted-foreground">{spec.version}</span>}
                    {spec.framework_version && (
                      <span className="text-muted-foreground">fw:{spec.framework_version}</span>
                    )}
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => {
                        const next = { ...plugins };
                        delete next[name];
                        onUpdate({
                          ...project,
                          plugins: Object.keys(next).length ? next : undefined,
                        });
                      }}
                      className="ml-0.5 h-4 w-4 rounded-full hover:text-destructive"
                      aria-label={`Remove ${name}`}
                    >
                      <X size={10} />
                    </Button>
                  </span>
                ))}
                <input
                  type="text"
                  placeholder={Object.keys(plugins).length ? "" : "Add plugin (e.g. okapi)"}
                  className="min-w-[120px] flex-1 bg-transparent text-sm outline-none"
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === ",") {
                      e.preventDefault();
                      const val = e.currentTarget.value.trim();
                      if (val && !plugins[val]) {
                        onUpdate({
                          ...project,
                          plugins: { ...plugins, [val]: { version: "*" } },
                        });
                        e.currentTarget.value = "";
                      }
                    }
                  }}
                />
              </div>
              <p className="text-[10px] text-muted-foreground">
                Add plugins by name. Version ranges can be edited in the YAML directly.
              </p>

              {/* Plugin format priority */}
              {hasPlugins && (
                <>
                  <Separator />
                  <div>
                    <p className="mb-3 text-xs text-muted-foreground">
                      When a plugin provides a format that also exists as a built-in, the higher
                      priority wins. Built-in formats default to priority 0.
                    </p>
                    {Object.entries(plugins).map(([name, spec]) => {
                      const isPreferred = (spec.format_priority ?? 0) >= PLUGIN_PREFER_PRIORITY;
                      return (
                        <div key={name} className="mb-2">
                          <div className="flex items-center justify-between">
                            <span className="text-sm font-medium">{name}</span>
                            <div className="flex items-center gap-3">
                              <Label
                                htmlFor={`prefer-${name}`}
                                className="text-xs text-muted-foreground"
                              >
                                Prefer over built-in
                              </Label>
                              <Switch
                                id={`prefer-${name}`}
                                checked={isPreferred}
                                onCheckedChange={(checked) =>
                                  updatePlugin(name, {
                                    format_priority: checked ? PLUGIN_PREFER_PRIORITY : undefined,
                                  })
                                }
                              />
                            </div>
                          </div>
                          {isPreferred && (
                            <div className="mt-2 flex items-center gap-2">
                              <Label className="text-xs text-muted-foreground">Priority</Label>
                              <Input
                                type="number"
                                value={spec.format_priority ?? PLUGIN_PREFER_PRIORITY}
                                onChange={(e) => {
                                  const v = parseInt(e.target.value, 10);
                                  updatePlugin(name, {
                                    format_priority: isNaN(v) ? undefined : v,
                                  });
                                }}
                                className="h-7 w-20 text-xs"
                                min={1}
                              />
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </>
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
