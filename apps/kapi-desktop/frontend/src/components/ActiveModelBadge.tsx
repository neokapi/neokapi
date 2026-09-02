import { useQuery } from "@tanstack/react-query";
import { Cpu } from "lucide-react";
import { Badge, SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";

import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type { AIModelOrigin, AIModelResolution } from "../types/api";

/**
 * ActiveModelBadge — which provider and model an AI run will actually use.
 *
 * `kapi models setup` / `kapi models default` write one pair of keys
 * (`ai.provider` / `ai.model`) that both the CLI and the desktop read, but a
 * recipe may override them per project or per locale, and with nothing
 * configured a built-in default carries the run. Showing only the raw config
 * keys therefore mis-reports what will happen. The answer comes from the backend
 * (`host.EffectiveAIModel`), the single statement of that precedence — so this
 * badge and the run cannot disagree.
 */

const ORIGIN_LABEL: Record<AIModelOrigin, () => string> = {
  "locale-preset": () => t("from this language's recipe defaults"),
  "project-preset": () => t("from the project recipe"),
  "app-config": () => t("from your kapi default (kapi models default)"),
  "built-in": () => t("built-in fallback, no model configured"),
};

export interface ActiveModelBadgeProps {
  /** Project tab whose recipe presets apply; "" for the app-wide answer. */
  tabID?: string;
  /** Target locale whose per-locale preset applies; omit for project-wide. */
  locale?: string;
  /** Pre-loaded resolution for Storybook/tests — skips api.getEffectiveModel(). */
  resolution?: AIModelResolution | null;
  className?: string;
}

export function ActiveModelBadge({
  tabID = "",
  locale = "",
  resolution: propResolution,
  className,
}: ActiveModelBadgeProps) {
  const query = useQuery({
    queryKey: qk.effectiveModel(tabID, locale),
    enabled: propResolution === undefined,
    queryFn: () => api.getEffectiveModel(tabID, locale),
  });

  const res: AIModelResolution | null = propResolution ?? query.data ?? null;
  if (!res) return null;

  // The model half is what the user picked; an empty model means the provider's
  // own default, which is worth naming rather than leaving blank.
  const modelLabel = res.model || t("provider default");
  const origin = ORIGIN_LABEL[res.model_origin] ?? ORIGIN_LABEL["built-in"];

  return (
    <SimpleTooltip
      content={
        res.configured
          ? t("{provider} · {model} · {origin}", {
              provider: res.provider,
              model: modelLabel,
              origin: origin(),
            })
          : t("No model configured. Runs fall back to {provider}. Set one in AI Models.", {
              provider: res.provider,
            })
      }
    >
      <Badge
        variant="outline"
        data-slot="active-model"
        data-configured={res.configured}
        className={cn(
          "gap-1 font-normal",
          !res.configured && "border-amber-500/50 text-amber-600 dark:text-amber-400",
          className,
        )}
      >
        <Cpu size={11} aria-hidden />
        <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
          {t("model")}
        </span>
        {/* Provider ids and model tags are identifiers: monospaced, LTR, and
            isolated so an RTL UI language cannot reorder them. */}
        <bdi dir="ltr" className="font-mono text-[11px]">
          {res.provider}
          {" · "}
          {modelLabel}
        </bdi>
      </Badge>
    </SimpleTooltip>
  );
}
