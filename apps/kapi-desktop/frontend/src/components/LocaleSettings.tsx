import { useState, useEffect, useMemo } from "react";
import { RotateCcw, Plus, Trash2, EyeOff, Eye } from "lucide-react";
import { Card, CardContent, Button, Input, Label } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";

interface CustomLocale {
  code: string;
  display_name?: string;
}

interface AppSettings {
  theme: string;
  samples_dismissed?: boolean;
  hidden_locales?: string[];
  custom_locales?: CustomLocale[];
}

interface LocaleEntry {
  code: string;
  displayName: string;
  isCustom: boolean;
  isHidden: boolean;
}

export function LocaleSettings() {
  const { showError } = useError();
  const [allLocales, setAllLocales] = useState<Array<{ code: string; display_name: string }>>([]);
  const [hiddenSet, setHiddenSet] = useState<Set<string>>(new Set());
  const [customLocales, setCustomLocales] = useState<CustomLocale[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [newCode, setNewCode] = useState("");
  const [newDisplayName, setNewDisplayName] = useState("");
  const [addError, setAddError] = useState("");
  const [filter, setFilter] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([api.getAllLocales(), api.getSettings()])
      .then(([locales, settings]) => {
        if (locales) setAllLocales(locales);
        if (settings) {
          const s = settings as AppSettings;
          setHiddenSet(new Set(s.hidden_locales ?? []));
          setCustomLocales(s.custom_locales ?? []);
        }
      })
      .catch((err) => showError("Failed to load locale settings", err))
      .finally(() => setLoading(false));
  }, [showError]);

  const entries = useMemo<LocaleEntry[]>(() => {
    const result: LocaleEntry[] = allLocales.map((l) => ({
      code: l.code,
      displayName: l.display_name,
      isCustom: false,
      isHidden: hiddenSet.has(l.code),
    }));
    for (const cl of customLocales) {
      if (!result.some((e) => e.code === cl.code)) {
        result.push({
          code: cl.code,
          displayName: cl.display_name || cl.code,
          isCustom: true,
          isHidden: false,
        });
      }
    }
    return result;
  }, [allLocales, hiddenSet, customLocales]);

  const filtered = useMemo(() => {
    if (!filter) return entries;
    const q = filter.toLowerCase();
    return entries.filter(
      (e) => e.displayName.toLowerCase().includes(q) || e.code.toLowerCase().includes(q),
    );
  }, [entries, filter]);

  const saveSettings = async (hidden: Set<string>, custom: CustomLocale[]) => {
    try {
      const current = (await api.getSettings()) as AppSettings;
      await api.saveSettings({
        ...current,
        hidden_locales: [...hidden],
        custom_locales: custom,
      });
    } catch (err) {
      showError("Failed to save locale settings", err);
    }
  };

  const toggleSelect = (code: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(code)) next.delete(code);
      else next.add(code);
      return next;
    });
  };

  const selectAll = () => {
    setSelected(new Set(filtered.map((e) => e.code)));
  };

  const selectNone = () => {
    setSelected(new Set());
  };

  // Bulk operations
  const hideSelected = () => {
    const next = new Set(hiddenSet);
    for (const code of selected) {
      const entry = entries.find((e) => e.code === code);
      if (entry && !entry.isCustom) next.add(code);
    }
    setHiddenSet(next);
    setSelected(new Set());
    void saveSettings(next, customLocales);
  };

  const showSelected = () => {
    const next = new Set(hiddenSet);
    for (const code of selected) next.delete(code);
    setHiddenSet(next);
    setSelected(new Set());
    void saveSettings(next, customLocales);
  };

  const removeSelectedCustom = () => {
    const toRemove = new Set(
      [...selected].filter((code) => entries.find((e) => e.code === code)?.isCustom),
    );
    if (toRemove.size === 0) return;
    const next = customLocales.filter((cl) => !toRemove.has(cl.code));
    setCustomLocales(next);
    setSelected((prev) => {
      const s = new Set(prev);
      for (const code of toRemove) s.delete(code);
      return s;
    });
    void saveSettings(hiddenSet, next);
  };

  const addCustom = () => {
    const code = newCode.trim();
    if (!code) return;
    if (entries.some((e) => e.code === code)) {
      setAddError("Locale already exists");
      return;
    }
    if (!/^[a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})*$/.test(code)) {
      setAddError("Invalid locale code");
      return;
    }
    const cl: CustomLocale = { code, display_name: newDisplayName.trim() || undefined };
    const next = [...customLocales, cl];
    setCustomLocales(next);
    setNewCode("");
    setNewDisplayName("");
    setAddError("");
    void saveSettings(hiddenSet, next);
  };

  const resetToDefaults = () => {
    setHiddenSet(new Set());
    setCustomLocales([]);
    setSelected(new Set());
    void saveSettings(new Set(), []);
  };

  const hiddenCount = entries.filter((e) => e.isHidden).length;
  const customCount = entries.filter((e) => e.isCustom).length;
  const selectedHasHidden = [...selected].some((c) => hiddenSet.has(c));
  const selectedHasVisible = [...selected].some(
    (c) => !hiddenSet.has(c) && !entries.find((e) => e.code === c)?.isCustom,
  );
  const selectedHasCustom = [...selected].some((c) => entries.find((e) => e.code === c)?.isCustom);

  if (loading) return null;

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Locales</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {entries.length - hiddenCount} active, {hiddenCount} hidden
            {customCount > 0 && `, ${customCount} custom`}
          </p>
        </div>
        {(hiddenCount > 0 || customCount > 0) && (
          <Button variant="outline" size="sm" onClick={resetToDefaults}>
            <RotateCcw size={12} />
            Reset
          </Button>
        )}
      </div>

      <div className="max-w-2xl space-y-4">
        {/* Add custom locale */}
        <Card>
          <CardContent className="p-4">
            <Label className="mb-2 block text-sm font-medium">Add Custom Locale</Label>
            <div className="flex items-center gap-2">
              <Input
                type="text"
                value={newCode}
                onChange={(e) => {
                  setNewCode(e.target.value);
                  setAddError("");
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") addCustom();
                }}
                placeholder="Code (e.g. gsw)"
                className="w-32"
              />
              <Input
                type="text"
                value={newDisplayName}
                onChange={(e) => setNewDisplayName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") addCustom();
                }}
                placeholder="Display name (e.g. Swiss German)"
                className="flex-1"
              />
              <Button variant="outline" size="sm" onClick={addCustom} disabled={!newCode.trim()}>
                <Plus size={12} />
                Add
              </Button>
            </div>
            {addError && <p className="mt-1 text-xs text-destructive">{addError}</p>}
          </CardContent>
        </Card>

        {/* Toolbar: filter + bulk actions */}
        <div className="flex items-center gap-2">
          <Input
            type="text"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter locales..."
            className="max-w-xs"
          />
          <div className="flex-1" />
          {selected.size > 0 && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span>{selected.size} selected</span>
              {selectedHasVisible && (
                <Button variant="outline" size="sm" onClick={hideSelected}>
                  <EyeOff size={12} />
                  Hide
                </Button>
              )}
              {selectedHasHidden && (
                <Button variant="outline" size="sm" onClick={showSelected}>
                  <Eye size={12} />
                  Show
                </Button>
              )}
              {selectedHasCustom && (
                <Button variant="outline" size="sm" onClick={removeSelectedCustom}>
                  <Trash2 size={12} />
                  Remove
                </Button>
              )}
            </div>
          )}
        </div>

        {/* Locale table */}
        <Card>
          <CardContent className="p-0">
            {/* Header */}
            <div className="flex items-center gap-3 border-b border-border px-4 py-2 text-xs font-medium text-muted-foreground">
              <input
                type="checkbox"
                checked={filtered.length > 0 && filtered.every((e) => selected.has(e.code))}
                onChange={() => {
                  if (filtered.every((e) => selected.has(e.code))) selectNone();
                  else selectAll();
                }}
                className="rounded"
              />
              <span className="flex-1">Language</span>
              <span className="w-24">Code</span>
              <span className="w-16 text-center">Status</span>
            </div>
            {/* Rows */}
            <div className="max-h-[400px] divide-y divide-border overflow-auto">
              {filtered.map((entry) => (
                <label
                  key={entry.code}
                  className={`flex cursor-pointer items-center gap-3 px-4 py-1.5 transition-colors hover:bg-accent/30 ${
                    selected.has(entry.code) ? "bg-accent/20" : ""
                  } ${entry.isHidden ? "opacity-50" : ""}`}
                >
                  <input
                    type="checkbox"
                    checked={selected.has(entry.code)}
                    onChange={() => toggleSelect(entry.code)}
                    className="rounded"
                  />
                  <span className="flex-1 text-sm">{entry.displayName}</span>
                  <span className="w-24 font-mono text-xs text-muted-foreground">{entry.code}</span>
                  <span className="flex w-16 justify-center">
                    {entry.isCustom && (
                      <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[9px] text-primary">
                        custom
                      </span>
                    )}
                    {entry.isHidden && (
                      <span className="rounded bg-muted px-1.5 py-0.5 text-[9px] text-muted-foreground">
                        hidden
                      </span>
                    )}
                  </span>
                </label>
              ))}
              {filtered.length === 0 && (
                <div className="px-4 py-6 text-center text-sm text-muted-foreground">
                  No locales match the filter.
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
