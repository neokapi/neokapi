/**
 * Browsers: Preset Browser
 *
 * Browse presets per format. Shows all available presets,
 * their parameter values, and diff from the default preset.
 */
import { useState, useMemo } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, Input } from "@neokapi/ui-primitives";
import { presets } from "../_lib/reference-data";

const allPresets = presets as Record<string, Record<string, Record<string, unknown> | null>>;

interface PresetEntry {
  formatId: string;
  presetId: string;
  values: Record<string, unknown>;
}

function PresetBrowser() {
  const [selectedFormat, setSelectedFormat] = useState<string | null>(null);
  const [selectedPreset, setSelectedPreset] = useState<PresetEntry | null>(null);
  const [search, setSearch] = useState("");

  const formatIds = useMemo(() => {
    return Object.keys(allPresets)
      .filter((id) => {
        const fp = allPresets[id];
        if (!fp) return false;
        // Filter out formats where all presets have null params
        return Object.values(fp).some((v) => v != null);
      })
      .sort();
  }, []);

  const filteredFormats = useMemo(() => {
    if (!search) return formatIds;
    const q = search.toLowerCase();
    return formatIds.filter((id) => id.toLowerCase().includes(q));
  }, [formatIds, search]);

  const totalPresets = useMemo(
    () =>
      formatIds.reduce((sum, id) => {
        const fp = allPresets[id];
        return sum + (fp ? Object.values(fp).filter((v) => v != null).length : 0);
      }, 0),
    [formatIds],
  );

  // Get default preset for a format (the one named "default" or the first one)
  function getDefaultPreset(formatId: string): Record<string, unknown> | null {
    const fp = allPresets[formatId];
    if (!fp) return null;
    if (fp["default"]) return fp["default"];
    const keys = Object.keys(fp);
    return keys.length > 0 ? fp[keys[0]] : null;
  }

  // Compute diff between a preset and the default
  function computeDiff(
    preset: Record<string, unknown>,
    defaultPreset: Record<string, unknown>,
  ): { key: string; presetVal: unknown; defaultVal: unknown }[] {
    const diffs: { key: string; presetVal: unknown; defaultVal: unknown }[] = [];
    const allKeys = new Set([...Object.keys(preset), ...Object.keys(defaultPreset)]);
    for (const key of allKeys) {
      if (JSON.stringify(preset[key]) !== JSON.stringify(defaultPreset[key])) {
        diffs.push({ key, presetVal: preset[key], defaultVal: defaultPreset[key] });
      }
    }
    return diffs;
  }

  if (selectedPreset) {
    const defaultPreset = getDefaultPreset(selectedPreset.formatId);
    const diffs =
      defaultPreset && selectedPreset.presetId !== "default"
        ? computeDiff(selectedPreset.values, defaultPreset)
        : [];

    return (
      <div style={{ maxWidth: 700 }}>
        <Button
          variant="link"
          size="sm"
          onClick={() => setSelectedPreset(null)}
          className="mb-4 px-0"
        >
          &larr; Back to {selectedFormat || "formats"}
        </Button>

        <h3 className="text-lg font-semibold mb-1">
          {selectedPreset.formatId} / {selectedPreset.presetId}
        </h3>

        {diffs.length > 0 && (
          <div className="mb-4">
            <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
              Differences from default ({diffs.length} parameter{diffs.length !== 1 ? "s" : ""})
            </h4>
            <div className="space-y-1">
              {diffs.map((d) => (
                <div key={d.key} className="rounded border p-2 text-xs">
                  <code className="font-medium">{d.key}</code>
                  <div className="flex gap-4 mt-1">
                    <span className="text-destructive">
                      default: <code>{JSON.stringify(d.defaultVal)}</code>
                    </span>
                    <span className="text-primary">
                      preset: <code>{JSON.stringify(d.presetVal)}</code>
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          All Values
        </h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-80">
          {JSON.stringify(selectedPreset.values, null, 2)}
        </pre>
      </div>
    );
  }

  if (selectedFormat) {
    const rawPresets = allPresets[selectedFormat] || {};
    const formatPresets = Object.fromEntries(
      Object.entries(rawPresets).filter(([, v]) => v != null),
    );
    const presetNames = Object.keys(formatPresets).sort();

    return (
      <div style={{ maxWidth: 700 }}>
        <Button
          variant="link"
          size="sm"
          onClick={() => setSelectedFormat(null)}
          className="mb-4 px-0"
        >
          &larr; Back to format list
        </Button>

        <h3 className="text-lg font-semibold mb-3">
          {selectedFormat} — {presetNames.length} preset{presetNames.length !== 1 ? "s" : ""}
        </h3>

        <div className="space-y-2">
          {presetNames.map((presetId) => {
            const values = formatPresets[presetId] as Record<string, unknown>;
            const paramCount = Object.keys(values).length;
            const defaultPreset = getDefaultPreset(selectedFormat);
            const diffCount =
              defaultPreset && presetId !== "default"
                ? computeDiff(values, defaultPreset).length
                : 0;

            return (
              <Button
                key={presetId}
                variant="outline"
                onClick={() => setSelectedPreset({ formatId: selectedFormat, presetId, values })}
                className="w-full h-auto whitespace-normal rounded-lg p-3 text-left hover:border-primary/30 hover:bg-primary/5"
              >
                <div className="flex items-center justify-between">
                  <code className="text-sm font-medium">{presetId}</code>
                  <div className="flex gap-2 text-[10px] text-muted-foreground">
                    <span>{paramCount} params</span>
                    {diffCount > 0 && (
                      <span className="text-primary">
                        {diffCount} diff{diffCount !== 1 ? "s" : ""} from default
                      </span>
                    )}
                  </div>
                </div>
              </Button>
            );
          })}
        </div>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: 700 }}>
      <div className="mb-4">
        <Input
          type="text"
          placeholder="Search formats with presets..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <p className="text-xs text-muted-foreground mb-4">
        {totalPresets} presets across {formatIds.length} formats
      </p>

      <div className="space-y-1">
        {filteredFormats.map((formatId) => {
          const fp = allPresets[formatId] || {};
          const presetCount = Object.values(fp).filter((v) => v != null).length;
          return (
            <Button
              key={formatId}
              variant="outline"
              onClick={() => setSelectedFormat(formatId)}
              className="w-full h-auto whitespace-normal rounded-lg p-3 text-left hover:border-primary/30 hover:bg-primary/5"
            >
              <div className="flex items-center justify-between">
                <code className="text-sm font-medium">{formatId}</code>
                <span className="text-xs text-muted-foreground">
                  {presetCount} preset{presetCount !== 1 ? "s" : ""}
                </span>
              </div>
            </Button>
          );
        })}
      </div>
    </div>
  );
}

const meta: Meta<typeof PresetBrowser> = {
  title: "Formats & Tools/Browsers/Preset Browser",
  component: PresetBrowser,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PresetBrowser>;

export const AllPresets: Story = {};
