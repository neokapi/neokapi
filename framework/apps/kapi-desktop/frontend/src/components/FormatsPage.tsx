import { useState, useEffect, useCallback, useRef, useReducer } from "react";
import {
  FileText,
  ArrowLeft,
  FileInput,
  FileOutput,
  Plug,
  Settings2,
} from "lucide-react";
import type { FormatInfo } from "../types/api";
import type { ComponentSchema } from "@neokapi/flow-editor";
import { SchemaForm } from "@neokapi/flow-editor";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";

export function FormatsPage() {
  const [formats, setFormats] = useState<FormatInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedFormat, setSelectedFormat] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  const { showError } = useError();

  useEffect(() => {
    api
      .listFormats()
      .then((f) => {
        if (f) setFormats(f);
      })
      .catch((err) => showError("Failed to load formats", err))
      .finally(() => setLoading(false));
  }, [showError]);

  const filtered = search
    ? formats.filter(
        (f) =>
          f.name.toLowerCase().includes(search.toLowerCase()) ||
          f.display_name?.toLowerCase().includes(search.toLowerCase()) ||
          f.extensions?.some((e) => e.toLowerCase().includes(search.toLowerCase())),
      )
    : formats;

  // Group by source
  const builtIn = filtered.filter((f) => f.source === "built-in" || !f.source);
  const plugin = filtered.filter((f) => f.source && f.source !== "built-in");

  if (selectedFormat) {
    return (
      <FormatDetail
        formatName={selectedFormat}
        formatInfo={formats.find((f) => f.name === selectedFormat)}
        onBack={() => setSelectedFormat(null)}
      />
    );
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Formats</h1>
      </div>

      {/* Search */}
      <div className="relative mb-4">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search formats by name or extension..."
          className="w-full rounded-md border border-input bg-transparent pl-8 pr-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
        />
        <svg
          className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground"
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <circle cx="11" cy="11" r="8" />
          <path d="m21 21-4.3-4.3" />
        </svg>
      </div>

      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <div key={i} className="rounded-lg border border-border p-4 animate-pulse">
              <div className="h-3.5 bg-muted rounded w-1/3 mb-2" />
              <div className="h-2.5 bg-muted rounded w-2/3" />
            </div>
          ))}
        </div>
      )}

      {!loading && (
        <>
          {builtIn.length > 0 && (
            <FormatSection
              title="Built-in Formats"
              formats={builtIn}
              onSelect={setSelectedFormat}
            />
          )}
          {plugin.length > 0 && (
            <FormatSection
              title="Plugin Formats"
              formats={plugin}
              onSelect={setSelectedFormat}
            />
          )}
          {filtered.length === 0 && (
            <div className="py-12 text-center text-muted-foreground">
              <p className="text-sm">
                {search ? "No formats match your search." : "No formats available."}
              </p>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function FormatSection({
  title,
  formats,
  onSelect,
}: {
  title: string;
  formats: FormatInfo[];
  onSelect: (name: string) => void;
}) {
  return (
    <div className="mb-6">
      <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
        {title}
      </h2>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
        {formats.map((f) => (
          <button
            key={f.name}
            onClick={() => onSelect(f.name)}
            className="group w-full text-left rounded-lg border border-border bg-card p-4 transition-all hover:border-primary/30 hover:shadow-md"
          >
            <div className="flex items-start gap-3">
              <FileText
                size={18}
                className="mt-0.5 text-muted-foreground group-hover:text-primary transition-colors shrink-0"
              />
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors truncate">
                    {f.display_name || f.name}
                  </span>
                  {f.source && f.source !== "built-in" && (
                    <Plug size={10} className="text-muted-foreground shrink-0" title={f.source} />
                  )}
                </div>
                {f.extensions && f.extensions.length > 0 && (
                  <div className="flex flex-wrap gap-1 mt-1">
                    {f.extensions.map((ext) => (
                      <span
                        key={ext}
                        className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground font-mono"
                      >
                        {ext}
                      </span>
                    ))}
                  </div>
                )}
                <div className="flex items-center gap-2 mt-1.5 text-[10px] text-muted-foreground">
                  {f.has_reader && (
                    <span className="flex items-center gap-0.5">
                      <FileInput size={9} /> Read
                    </span>
                  )}
                  {f.has_writer && (
                    <span className="flex items-center gap-0.5">
                      <FileOutput size={9} /> Write
                    </span>
                  )}
                  {f.has_schema && (
                    <span className="flex items-center gap-0.5">
                      <Settings2 size={9} /> Configurable
                    </span>
                  )}
                </div>
              </div>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

function FormatDetail({
  formatName,
  formatInfo,
  onBack,
}: {
  formatName: string;
  formatInfo?: FormatInfo;
  onBack: () => void;
}) {
  const [schema, setSchema] = useState<ComponentSchema | null>(null);
  const [presets, setPresets] = useState<
    Array<{ name: string; description: string; format: string; config?: Record<string, unknown> }>
  >([]);
  const [loadingSchema, setLoadingSchema] = useState(true);
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [selectedPreset, setSelectedPreset] = useState<string | null>(null);

  const { showError } = useError();

  useEffect(() => {
    setLoadingSchema(true);
    Promise.all([
      api.getFormatSchema(formatName),
      api.listFormatPresets(formatName),
    ])
      .then(([s, p]) => {
        if (s) setSchema(s as ComponentSchema);
        if (p) setPresets(p);
      })
      .catch((err) => showError("Failed to load format details", err))
      .finally(() => setLoadingSchema(false));
  }, [formatName, showError]);

  const handlePresetSelect = useCallback(
    (presetName: string) => {
      const preset = presets.find((p) => p.name === presetName);
      if (preset?.config) {
        setConfig(preset.config);
        setSelectedPreset(presetName);
      }
    },
    [presets],
  );

  return (
    <div className="p-6">
      {/* Header */}
      <div className="flex items-center gap-3 mb-6">
        <button
          onClick={onBack}
          className="p-1 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft size={16} />
        </button>
        <FileText size={20} className="text-primary" />
        <div>
          <h1 className="text-lg font-semibold">
            {formatInfo?.display_name || formatName}
          </h1>
          <div className="flex items-center gap-2 mt-0.5">
            {formatInfo?.extensions?.map((ext) => (
              <span
                key={ext}
                className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground font-mono"
              >
                {ext}
              </span>
            ))}
            {formatInfo?.mime_types?.map((mt) => (
              <span
                key={mt}
                className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground"
              >
                {mt}
              </span>
            ))}
          </div>
        </div>
      </div>

      {/* Capabilities */}
      <div className="flex gap-3 mb-6">
        {formatInfo?.has_reader && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <FileInput size={12} className="text-primary" /> Reader
          </div>
        )}
        {formatInfo?.has_writer && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <FileOutput size={12} className="text-primary" /> Writer
          </div>
        )}
        {formatInfo?.source && formatInfo.source !== "built-in" && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Plug size={12} /> {formatInfo.source}
          </div>
        )}
      </div>

      {/* Presets */}
      {presets.length > 0 && (
        <div className="mb-6">
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Presets
          </h2>
          <div className="flex flex-wrap gap-2">
            {presets.map((p) => (
              <button
                key={p.name}
                onClick={() => handlePresetSelect(p.name)}
                className={`rounded-md border px-3 py-1.5 text-xs transition-colors ${
                  selectedPreset === p.name
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/30 hover:text-foreground"
                }`}
              >
                {p.name}
                {p.description && (
                  <span className="ml-1 text-muted-foreground">— {p.description}</span>
                )}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Configuration schema */}
      {loadingSchema && (
        <div className="py-8 text-center text-sm text-muted-foreground animate-pulse">
          Loading configuration schema...
        </div>
      )}

      {!loadingSchema && schema && (
        <div>
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">
            Configuration
          </h2>
          <div className="max-w-xl rounded-lg border border-border bg-card p-4">
            <SchemaForm schema={schema} values={config} onChange={setConfig} />
          </div>
        </div>
      )}

      {!loadingSchema && !schema && (
        <div className="py-8 text-center text-sm text-muted-foreground">
          This format has no configurable parameters.
        </div>
      )}
    </div>
  );
}
