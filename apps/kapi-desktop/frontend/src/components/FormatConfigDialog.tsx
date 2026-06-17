import { useState, useEffect, useMemo, useCallback } from "react";
import { Loader2, Plus, X, ChevronLeft, ChevronRight } from "lucide-react";
import { t } from "@neokapi/kapi-react/runtime";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  Button,
  Label,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  FormatSelect,
  SchemaForm,
} from "@neokapi/ui-primitives";
import type { ComponentSchema, FormatInfo } from "../types/api";
import { api } from "../hooks/useApi";
import { useSchemaFormHost } from "../hooks/useSchemaFormHost";
import { useError } from "./ErrorBanner";

/** Config + preset for a single format slot. */
export interface FormatConfigValue {
  config?: Record<string, unknown>;
  preset?: string;
}

interface PresetItem {
  name: string;
  description?: string;
  config?: Record<string, unknown>;
}

interface FormatConfigDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  /** Formats shown initially in the list (e.g. the formats matched in the input files). */
  formats: string[];
  /** All registered formats, for the "add format" picker. */
  allFormats: FormatInfo[];
  /** Allow configuring formats beyond the initial list (wildcard items). */
  allowAdd?: boolean;
  /** When set, the add-picker is filtered to formats claiming this extension (e.g. ".json"). */
  filterExtension?: string;
  /** Current config/preset keyed by format name. */
  values: Record<string, FormatConfigValue>;
  /** Persist a change for one format. */
  onChange: (format: string, next: FormatConfigValue) => void;
  /** Footer note clarifying where the config is stored (item vs project-wide). */
  scopeNote?: string;
}

/**
 * Schema-driven format configuration in a right-side drawer, laid out as a
 * master→detail flow so each level gets the full drawer width:
 *
 *   - **List** (wildcard / multi-format only): the formats to configure, with
 *     config-count badges and an "add format" control. Tapping one opens its
 *     detail.
 *   - **Detail**: the format's option form (the framework `SchemaForm`, whose own
 *     header names the format) plus its preset, with a back affordance.
 *
 * A single-format item skips the list and opens straight on its detail.
 */
export function FormatConfigDialog({
  open,
  onOpenChange,
  title,
  description,
  formats,
  allFormats,
  allowAdd = false,
  filterExtension,
  values,
  onChange,
  scopeNote,
}: FormatConfigDialogProps) {
  const { showError } = useError();
  // Native file/credential pickers for SchemaForm widgets (degrades to text
  // inputs outside Wails / in tests).
  const host = useSchemaFormHost();
  // Formats added during this session beyond the initial set.
  const [added, setAdded] = useState<string[]>([]);
  const [adding, setAdding] = useState(false);

  const shown = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const f of [...formats, ...added]) {
      if (f && !seen.has(f)) {
        seen.add(f);
        out.push(f);
      }
    }
    return out;
  }, [formats, added]);

  // A list level exists only when there's a choice to make (wildcard items, or
  // more than one matched format). Single-format items go straight to detail.
  const hasList = allowAdd || formats.length > 1;

  // master→detail: null = list level; a format name = that format's detail.
  // Single-format items open straight on the detail (no list to show).
  const [selected, setSelected] = useState<string | null>(hasList ? null : (formats[0] ?? null));

  // Reset to the entry level each time the drawer opens (rising edge only — the
  // parent re-creates `formats` every render, so depending on it would thrash).
  useEffect(() => {
    if (!open) return;
    setAdded([]);
    setAdding(false);
    setSelected(hasList ? null : (formats[0] ?? null));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const active = selected ?? "";
  const inList = selected === null;

  // Per-format schema + preset caches (loaded lazily for the open detail).
  const [schemas, setSchemas] = useState<Record<string, ComponentSchema | null>>({});
  const [presets, setPresets] = useState<Record<string, PresetItem[]>>({});
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open || !active || schemas[active] !== undefined) return;
    setLoading(true);
    Promise.all([api.getFormatSchema(active), api.listFormatPresets(active)])
      .then(([s, p]) => {
        setSchemas((prev) => ({ ...prev, [active]: (s as ComponentSchema) ?? null }));
        setPresets((prev) => ({ ...prev, [active]: (p as PresetItem[]) ?? [] }));
      })
      .catch((err) => {
        showError("Failed to load format schema", err);
        setSchemas((prev) => ({ ...prev, [active]: null }));
      })
      .finally(() => setLoading(false));
  }, [open, active, schemas, showError]);

  const addOptions = useMemo(() => {
    const ext = filterExtension?.toLowerCase();
    return allFormats.filter((f) => {
      if (shown.includes(f.name)) return false;
      if (!ext) return true;
      return (f.extensions ?? []).some((e) => e.toLowerCase() === ext);
    });
  }, [allFormats, shown, filterExtension]);

  const handleAdd = useCallback((name: string | undefined) => {
    if (!name) return;
    setAdded((prev) => (prev.includes(name) ? prev : [...prev, name]));
    setAdding(false);
    setSelected(name); // jump straight into the newly added format's detail
  }, []);

  const current = values[active] ?? {};
  const activeSchema = schemas[active];
  const activePresets = presets[active] ?? [];

  // Baseline for the per-property "modified" dots: the format's schema defaults,
  // overlaid with the selected preset. A property whose current value differs
  // from this baseline renders as dirty (overridden) — without it, dots only
  // appear when a preset is chosen, which is why edits showed no per-field state.
  const baselineValues = useMemo(() => {
    if (!activeSchema) return undefined;
    const base: Record<string, unknown> = {};
    for (const [k, p] of Object.entries(activeSchema.properties ?? {})) {
      if (p?.default !== undefined) base[k] = p.default;
    }
    const preset = activePresets.find((p) => p.name === current.preset);
    if (preset?.config) Object.assign(base, preset.config);
    return base;
  }, [activeSchema, activePresets, current.preset]);

  function configCount(fmt: string): number {
    return Object.keys(values[fmt]?.config ?? {}).length;
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex w-full flex-col gap-0 p-0 sm:max-w-lg md:max-w-xl">
        <SheetHeader className="border-b border-border">
          <SheetTitle>{title}</SheetTitle>
          {description && <SheetDescription>{description}</SheetDescription>}
        </SheetHeader>

        {/* LIST level — choose a format to configure */}
        {inList ? (
          <div className="min-h-0 flex-1 space-y-2 overflow-auto p-4">
            <Label className="text-xs text-muted-foreground">{t("Formats")}</Label>
            {shown.map((f) => (
              <button
                key={f}
                onClick={() => setSelected(f)}
                className="flex w-full items-center justify-between rounded-md border border-border px-3 py-2.5 text-left text-sm transition-colors hover:bg-accent/50"
              >
                <span className="font-medium" translate="no">
                  {f}
                </span>
                <span className="flex items-center gap-2">
                  {configCount(f) > 0 && (
                    <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary">
                      {t("{count} set", { count: configCount(f) })}
                    </span>
                  )}
                  <ChevronRight size={15} className="text-muted-foreground" />
                </span>
              </button>
            ))}
            {shown.length === 0 && !adding && (
              <p className="py-2 text-xs text-muted-foreground">
                {t("No formats matched yet. Add one to configure it.")}
              </p>
            )}
            {allowAdd &&
              (adding ? (
                <div className="flex items-center gap-2 pt-1">
                  <FormatSelect
                    value=""
                    onChange={handleAdd}
                    formats={addOptions}
                    placeholder={t("Pick a format")}
                    className="flex-1"
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setAdding(false)}
                    aria-label={t("Cancel")}
                  >
                    <X size={14} />
                  </Button>
                </div>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  className="w-full justify-start text-muted-foreground"
                  onClick={() => setAdding(true)}
                >
                  <Plus size={14} />
                  {t("Add format")}
                </Button>
              ))}
          </div>
        ) : (
          /* DETAIL level — one format's options, full width */
          <div className="min-h-0 flex-1 overflow-auto p-4">
            {hasList && (
              <button
                onClick={() => setSelected(null)}
                className="mb-3 flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
              >
                <ChevronLeft size={14} />
                {t("Formats")}
              </button>
            )}
            {loading || activeSchema === undefined ? (
              <div className="flex h-40 items-center justify-center text-muted-foreground">
                <Loader2 className="animate-spin" size={16} />
              </div>
            ) : activeSchema === null ? (
              <div>
                <h3 className="text-sm font-semibold text-foreground" translate="no">
                  {active}
                </h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t("No configurable options for this format.")}
                </p>
              </div>
            ) : (
              <div className="space-y-3">
                {activePresets.length > 0 && (
                  <div>
                    <Label className="mb-0.5 block text-xs text-muted-foreground">
                      {t("Preset")}
                    </Label>
                    <Select
                      value={current.preset || "__default__"}
                      onValueChange={(v) =>
                        onChange(active, {
                          ...current,
                          preset: v === "__default__" ? undefined : v,
                        })
                      }
                    >
                      <SelectTrigger className="h-8 text-xs">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="__default__">{t("Default")}</SelectItem>
                        {activePresets.map((p) => (
                          <SelectItem key={p.name} value={p.name} translate="no">
                            {p.name}
                            {p.description ? ` — ${p.description}` : ""}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )}
                <SchemaForm
                  schema={activeSchema}
                  values={current.config ?? {}}
                  presetValues={baselineValues}
                  host={host}
                  onChange={(cfg) =>
                    onChange(active, {
                      ...current,
                      config: Object.keys(cfg).length > 0 ? cfg : undefined,
                    })
                  }
                />
              </div>
            )}
          </div>
        )}

        <div className="flex items-center justify-between gap-3 border-t border-border p-4">
          {scopeNote ? <p className="text-xs text-muted-foreground">{scopeNote}</p> : <span />}
          <Button size="sm" onClick={() => onOpenChange(false)}>
            {t("Done")}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
