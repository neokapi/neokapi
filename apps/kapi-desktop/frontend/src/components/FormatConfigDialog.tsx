import { useState, useEffect, useMemo, useCallback } from "react";
import { Loader2, Plus, X } from "lucide-react";
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
} from "@neokapi/ui-primitives";
import type { ComponentSchema, FormatInfo } from "../types/api";
import { api } from "../hooks/useApi";
import { FormatConfigEditor } from "./FormatConfigEditor";
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
  /** Formats shown initially in the left selector (e.g. the formats matched in the input files). */
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
 * Schema-driven format configuration in a right-side drawer. Replaces the inline
 * JSON textarea: each format is configured through its real option schema
 * (FormatConfigEditor) in an independently scrollable pane. For a single-format
 * item the drawer shows one form; for a wildcard item it shows a format picker on
 * the left (defaulted to the formats matched in the input files, optionally
 * filtered by the glob extension, with an "add format" control) and the selected
 * format's schema form on the right.
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

  const [active, setActive] = useState<string>(shown[0] ?? "");
  useEffect(() => {
    if (!active && shown.length > 0) setActive(shown[0]);
  }, [shown, active]);

  // Reset session-added formats each time the dialog opens.
  useEffect(() => {
    if (open) {
      setAdded([]);
      setAdding(false);
      setActive(formats[0] ?? "");
    }
  }, [open, formats]);

  // Per-format schema + preset caches (loaded lazily for the active format).
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
    setActive(name);
    setAdding(false);
  }, []);

  const current = values[active] ?? {};
  const activeSchema = schemas[active];
  const activePresets = presets[active] ?? [];
  const presetValues = activePresets.find((p) => p.name === current.preset)?.config;
  const multiPane = allowAdd || shown.length > 1;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 p-0 sm:max-w-lg md:max-w-xl lg:max-w-2xl"
      >
        <SheetHeader className="border-b border-border">
          <SheetTitle>{title}</SheetTitle>
          {description && <SheetDescription>{description}</SheetDescription>}
        </SheetHeader>

        <div className="flex min-h-0 flex-1">
          {/* Left: format picker (wildcard / multi-format only) */}
          {multiPane && (
            <div className="w-44 shrink-0 space-y-1 overflow-auto border-r border-border p-3">
              <Label className="mb-1 block text-xs text-muted-foreground">{t("Formats")}</Label>
              {shown.map((f) => (
                <button
                  key={f}
                  onClick={() => setActive(f)}
                  className={`flex w-full items-center justify-between rounded px-2 py-1 text-left text-xs ${
                    f === active ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
                  }`}
                  translate="no"
                >
                  <span className="truncate">{f}</span>
                  {values[f]?.config && Object.keys(values[f].config!).length > 0 && (
                    <span className="ml-1 rounded bg-primary/10 px-1 text-[10px] text-primary">
                      {Object.keys(values[f].config!).length}
                    </span>
                  )}
                </button>
              ))}
              {allowAdd &&
                (adding ? (
                  <div className="flex items-center gap-1 pt-1">
                    <FormatSelect
                      value=""
                      onChange={handleAdd}
                      formats={addOptions}
                      placeholder={t("Pick a format")}
                      className="h-7 text-xs"
                    />
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => setAdding(false)}
                      aria-label={t("Cancel")}
                    >
                      <X size={11} />
                    </Button>
                  </div>
                ) : (
                  <Button
                    variant="ghost"
                    size="xs"
                    className="mt-1 w-full justify-start text-muted-foreground"
                    onClick={() => setAdding(true)}
                  >
                    <Plus size={11} />
                    {t("Add format")}
                  </Button>
                ))}
            </div>
          )}

          {/* Right: schema form for the active format (independently scrollable) */}
          <div className="min-h-0 flex-1 overflow-auto p-4">
            {!active ? (
              <p className="text-sm text-muted-foreground">
                {t("No format selected. Add a format to configure it.")}
              </p>
            ) : loading || activeSchema === undefined ? (
              <div className="flex h-40 items-center justify-center text-muted-foreground">
                <Loader2 className="animate-spin" size={16} />
              </div>
            ) : activeSchema === null ? (
              <p className="text-sm text-muted-foreground">
                {t("No configurable options for this format.")}
              </p>
            ) : (
              <div className="space-y-3">
                {/* Preset */}
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
                <FormatConfigEditor
                  schema={activeSchema}
                  values={current.config ?? {}}
                  presetValues={presetValues}
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
        </div>

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
