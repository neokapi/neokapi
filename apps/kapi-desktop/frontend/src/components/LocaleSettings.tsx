import { useState, useEffect, useMemo } from "react";
import { RotateCcw, Plus, X } from "lucide-react";
import { Card, CardContent, Button, Input, Label, Switch } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";

interface AppSettings {
  theme: string;
  samples_dismissed?: boolean;
  hidden_locales?: string[];
  custom_locales?: string[];
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
  const [customCodes, setCustomCodes] = useState<string[]>([]);
  const [newCode, setNewCode] = useState("");
  const [addError, setAddError] = useState("");
  const [loading, setLoading] = useState(true);

  // Load all well-known locales (unfiltered) and current settings.
  useEffect(() => {
    Promise.all([api.getAllLocales(), api.getSettings()])
      .then(([locales, settings]) => {
        if (locales) setAllLocales(locales);
        if (settings) {
          const s = settings as AppSettings;
          setHiddenSet(new Set(s.hidden_locales ?? []));
          setCustomCodes(s.custom_locales ?? []);
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
    for (const code of customCodes) {
      if (!result.some((e) => e.code === code)) {
        result.push({ code, displayName: code, isCustom: true, isHidden: false });
      }
    }
    return result;
  }, [allLocales, hiddenSet, customCodes]);

  const saveSettings = async (hidden: Set<string>, custom: string[]) => {
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

  const toggleLocale = (code: string) => {
    const next = new Set(hiddenSet);
    if (next.has(code)) next.delete(code);
    else next.add(code);
    setHiddenSet(next);
    void saveSettings(next, customCodes);
  };

  const addCustom = () => {
    const code = newCode.trim();
    if (!code) return;
    if (entries.some((e) => e.code === code)) {
      setAddError("Locale already exists");
      return;
    }
    // Basic validation: must look like a locale code.
    if (!/^[a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})*$/.test(code)) {
      setAddError("Invalid locale code");
      return;
    }
    const next = [...customCodes, code];
    setCustomCodes(next);
    setNewCode("");
    setAddError("");
    void saveSettings(hiddenSet, next);
  };

  const removeCustom = (code: string) => {
    const next = customCodes.filter((c) => c !== code);
    setCustomCodes(next);
    void saveSettings(hiddenSet, next);
  };

  const resetToDefaults = () => {
    setHiddenSet(new Set());
    setCustomCodes([]);
    void saveSettings(new Set(), []);
  };

  const hiddenCount = entries.filter((e) => e.isHidden).length;
  const customCount = entries.filter((e) => e.isCustom).length;

  if (loading) return null;

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Locales</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Customize which locales appear in project language selectors.
            {hiddenCount > 0 && ` ${hiddenCount} hidden.`}
            {customCount > 0 && ` ${customCount} custom.`}
          </p>
        </div>
        {(hiddenCount > 0 || customCount > 0) && (
          <Button variant="outline" size="sm" onClick={resetToDefaults}>
            <RotateCcw size={12} />
            Reset
          </Button>
        )}
      </div>

      <div className="max-w-2xl space-y-6">
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
                placeholder="e.g. gsw, nb-NO, pt-BR"
                className="max-w-xs"
              />
              <Button variant="outline" size="sm" onClick={addCustom} disabled={!newCode.trim()}>
                <Plus size={12} />
                Add
              </Button>
            </div>
            {addError && <p className="mt-1 text-xs text-destructive">{addError}</p>}
            <p className="mt-1 text-[10px] text-muted-foreground">
              Add locale codes not in the default list. Must be a valid BCP-47 tag.
            </p>
          </CardContent>
        </Card>

        {/* Locale list */}
        <Card>
          <CardContent className="p-0">
            <div className="max-h-[400px] divide-y divide-border overflow-auto">
              {entries.map((entry) => (
                <div
                  key={entry.code}
                  className={`flex items-center justify-between px-4 py-2 ${entry.isHidden ? "opacity-50" : ""}`}
                >
                  <div className="flex items-center gap-2">
                    <span className="text-sm">{entry.displayName}</span>
                    <span className="text-xs text-muted-foreground">({entry.code})</span>
                    {entry.isCustom && (
                      <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary">
                        custom
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    {entry.isCustom ? (
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => removeCustom(entry.code)}
                        className="hover:text-destructive"
                        aria-label={`Remove ${entry.code}`}
                      >
                        <X size={12} />
                      </Button>
                    ) : (
                      <Switch
                        checked={!entry.isHidden}
                        onCheckedChange={() => toggleLocale(entry.code)}
                        aria-label={`Toggle ${entry.displayName}`}
                      />
                    )}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
