import { useState, useEffect, useMemo } from "react";
import { RotateCcw, Plus, Trash2, EyeOff, Eye } from "lucide-react";
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
  Badge,
  SelectableList,
  type SelectableListColumn,
  type SelectableListAction,
} from "@neokapi/ui-primitives";
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

const columns: SelectableListColumn<LocaleEntry>[] = [
  {
    header: "Language",
    cell: (item) => <span className="text-sm">{item.displayName}</span>,
  },
  {
    header: "Code",
    cell: (item) => <span className="font-mono text-xs text-muted-foreground">{item.code}</span>,
    className: "w-24",
  },
  {
    header: "Status",
    cell: (item) => (
      <>
        {item.isCustom && (
          <Badge variant="secondary" className="text-[9px]">
            custom
          </Badge>
        )}
        {item.isHidden && (
          <Badge variant="outline" className="text-[9px]">
            hidden
          </Badge>
        )}
      </>
    ),
    className: "w-20",
  },
];

export function LocaleSettings() {
  const { showError } = useError();
  const [allLocales, setAllLocales] = useState<Array<{ code: string; display_name: string }>>([]);
  const [hiddenSet, setHiddenSet] = useState<Set<string>>(new Set());
  const [customLocales, setCustomLocales] = useState<CustomLocale[]>([]);
  const [newCode, setNewCode] = useState("");
  const [newDisplayName, setNewDisplayName] = useState("");
  const [addError, setAddError] = useState("");
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

  const actions: SelectableListAction<LocaleEntry>[] = [
    {
      label: (
        <>
          <EyeOff size={12} /> Hide
        </>
      ),
      onAction: (selected) => {
        const next = new Set(hiddenSet);
        for (const s of selected) if (!s.isCustom) next.add(s.code);
        setHiddenSet(next);
        void saveSettings(next, customLocales);
      },
      when: (item) => !item.isHidden && !item.isCustom,
    },
    {
      label: (
        <>
          <Eye size={12} /> Show
        </>
      ),
      onAction: (selected) => {
        const next = new Set(hiddenSet);
        for (const s of selected) next.delete(s.code);
        setHiddenSet(next);
        void saveSettings(next, customLocales);
      },
      when: (item) => item.isHidden,
    },
    {
      label: (
        <>
          <Trash2 size={12} /> Remove
        </>
      ),
      onAction: (selected) => {
        const codes = new Set(selected.map((s) => s.code));
        const next = customLocales.filter((cl) => !codes.has(cl.code));
        setCustomLocales(next);
        void saveSettings(hiddenSet, next);
      },
      when: (item) => item.isCustom,
    },
  ];

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
    void saveSettings(new Set(), []);
  };

  const hiddenCount = entries.filter((e) => e.isHidden).length;
  const customCount = entries.filter((e) => e.isCustom).length;

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

        {/* Locale table */}
        <SelectableList
          items={entries}
          getKey={(e) => e.code}
          columns={columns}
          actions={actions}
          filterFn={(item, q) =>
            item.displayName.toLowerCase().includes(q.toLowerCase()) ||
            item.code.toLowerCase().includes(q.toLowerCase())
          }
          filterPlaceholder="Filter locales..."
          rowClassName={(item) => (item.isHidden ? "opacity-50" : "")}
        />
      </div>
    </div>
  );
}
