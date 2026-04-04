import { Card } from "@neokapi/ui-primitives";
import { useState, useEffect, useCallback } from "react";
import { X, Plus } from "lucide-react";
import { useApi } from "../context/ApiContext";
import { useLocales } from "../hooks/useLocales";
import { LocaleSelect } from "./LocaleSelect";
import type { Workspace } from "../types/api";

/** Common languages that workspaces typically start with. */
const DEFAULT_LANGUAGES = [
  "en",
  "fr",
  "de",
  "es",
  "it",
  "pt",
  "nl",
  "ja",
  "ko",
  "zh",
  "ar",
  "ru",
  "sv",
  "nb",
  "da",
  "fi",
  "pl",
  "cs",
  "tr",
  "th",
];

interface WorkspaceLanguageSettingsProps {
  workspace: Workspace;
  onUpdate?: (languages: string[]) => void;
}

export function WorkspaceLanguageSettings({ workspace, onUpdate }: WorkspaceLanguageSettingsProps) {
  const api = useApi();
  const { getDisplayName } = useLocales();
  const [languages, setLanguages] = useState<string[]>(
    workspace.languages && workspace.languages.length > 0 ? workspace.languages : DEFAULT_LANGUAGES,
  );
  const [adding, setAdding] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setLanguages(
      workspace.languages && workspace.languages.length > 0
        ? workspace.languages
        : DEFAULT_LANGUAGES,
    );
  }, [workspace.languages]);

  const save = useCallback(
    async (next: string[]) => {
      setSaving(true);
      try {
        await api.updateWorkspace(workspace.slug, { languages: next } as Partial<Workspace>);
        setLanguages(next);
        onUpdate?.(next);
      } catch {
        setLanguages(languages);
      } finally {
        setSaving(false);
      }
    },
    [api, workspace.slug, languages, onUpdate],
  );

  const handleAdd = useCallback(
    (code: string) => {
      if (!code || languages.includes(code)) {
        setAdding(false);
        return;
      }
      const next = [...languages, code].sort();
      setAdding(false);
      void save(next);
    },
    [languages, save],
  );

  const handleRemove = useCallback(
    (code: string) => {
      const next = languages.filter((l) => l !== code);
      void save(next);
    },
    [languages, save],
  );

  return (
    <Card className="p-6">
      <div className="mb-4">
        <h2 className="text-xl font-semibold">Languages</h2>
        <p className="mt-1 text-[13px] text-muted-foreground">
          Languages available across all projects in this workspace. Language pickers throughout the
          platform will use this list.
        </p>
      </div>

      <div className="flex flex-wrap gap-2 items-center">
        {languages.map((lang) => (
          <span
            key={lang}
            className="inline-flex items-center gap-1.5 rounded-full bg-muted px-3 py-1 text-sm"
          >
            <span className="font-medium">{getDisplayName(lang)}</span>
            <span className="text-muted-foreground text-[11px]">{lang}</span>
            <button
              type="button"
              disabled={saving}
              onClick={() => handleRemove(lang)}
              className="ml-0.5 rounded-full p-0.5 hover:bg-muted-foreground/20 transition-colors disabled:opacity-50 bg-transparent border-none cursor-pointer"
              aria-label={`Remove ${lang}`}
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}

        {adding ? (
          <div className="w-52">
            <LocaleSelect value="" onChange={handleAdd} placeholder="Select language..." />
          </div>
        ) : (
          <button
            type="button"
            disabled={saving}
            onClick={() => setAdding(true)}
            className="inline-flex items-center gap-1 rounded-full border border-dashed border-muted-foreground/40 px-3 py-1 text-sm text-muted-foreground hover:border-foreground hover:text-foreground transition-colors disabled:opacity-50 bg-transparent cursor-pointer"
          >
            <Plus className="h-3.5 w-3.5" />
            Add
          </button>
        )}
      </div>
    </Card>
  );
}
