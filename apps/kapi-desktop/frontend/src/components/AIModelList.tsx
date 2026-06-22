import { Cpu, Cloud, Check, Download, KeyRound } from "lucide-react";
import { Badge, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { AIModelOption } from "../types/api";

export interface AIModelListProps {
  models: AIModelOption[];
  /** Override the highlighted row; defaults to the option flagged is_default. */
  selected?: { model: string; provider: string };
  /** Show the provider label per row. Off when the list is already grouped by
   * provider (the group header names it), on for a flat list (the prompt). */
  showProvider?: boolean;
  onSelect: (model: AIModelOption) => void;
}

/**
 * Model-first picker: a flat, selectable list of AI models (local Ollama first,
 * then cloud), where choosing a model implies its provider. Reused by the
 * "AI Models" settings tab and the run-time "pick a model" prompt.
 */
export function AIModelList({ models, selected, showProvider = true, onSelect }: AIModelListProps) {
  const isSelected = (m: AIModelOption) =>
    selected ? selected.model === m.model && selected.provider === m.provider : m.is_default;

  return (
    <ul className="space-y-1.5" role="radiogroup" aria-label={t("AI models")}>
      {models.map((m) => {
        const active = isSelected(m);
        return (
          <li key={`${m.provider}:${m.model}`}>
            <button
              type="button"
              role="radio"
              aria-checked={active}
              onClick={() => onSelect(m)}
              className={cn(
                "flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors",
                active ? "border-primary bg-primary/10" : "border-border hover:border-primary/30",
              )}
            >
              {m.local ? (
                <Cpu size={16} className="shrink-0 text-primary" />
              ) : (
                <Cloud size={16} className="shrink-0 text-muted-foreground" />
              )}
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate text-sm font-medium" translate="no">
                    {m.model}
                  </span>
                  {showProvider && (
                    <Badge variant="secondary" translate="no">
                      {m.label}
                    </Badge>
                  )}
                  {m.local && !m.installed && (
                    <Badge variant="outline" className="gap-1">
                      <Download size={10} />
                      {t("not installed")}
                    </Badge>
                  )}
                  {m.needs_key && (
                    <Badge variant="outline" className="gap-1 text-amber-600">
                      <KeyRound size={10} />
                      {t("needs key")}
                    </Badge>
                  )}
                </div>
                {m.note && (
                  <p className="mt-0.5 truncate text-[11px] text-muted-foreground">{m.note}</p>
                )}
              </div>
              {active && <Check size={16} className="shrink-0 text-primary" />}
            </button>
          </li>
        );
      })}
    </ul>
  );
}
