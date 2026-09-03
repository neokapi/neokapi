// A flow card that leads with what the flow produces.
//
// The name and the outcome line say what the flow is for; the step chips show
// the sequence it runs (recycle, translate, check) in place of a bare step
// count; and the flow the recipe runs by default carries a Default badge. The
// technical identity a reader could not act on is gone from the face of the
// card.

import { ArrowRight, Copy, FolderInput, Workflow } from "lucide-react";
import {
  Badge,
  Button,
  ConfirmDeleteButton,
  ItemCard,
  Markdown,
  SimpleTooltip,
  Skeleton,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";

export interface FlowCardItem {
  id: string;
  name: string;
  description?: string;
  /** Each step named, in order, for the chip strip. */
  steps?: string[];
  stepCount: number;
  source?: string;
  /** True when this is the project's default flow. */
  isDefault?: boolean;
}

/** The flow's steps as an ordered chip strip. */
export function StepChips({ steps }: { steps: string[] }) {
  if (!steps.length) return null;
  return (
    <div className="flex flex-wrap items-center gap-1" data-testid="flow-steps">
      {steps.map((step, i) => (
        <span key={`${step}-${i}`} className="flex items-center gap-1">
          {i > 0 && <ArrowRight className="size-3 text-muted-foreground/50" aria-hidden="true" />}
          <Badge variant="secondary" className="font-normal">
            {step}
          </Badge>
        </span>
      ))}
    </div>
  );
}

export function FlowCard({
  item,
  loading,
  onClick,
  onCopy,
  onDelete,
  onAdopt,
  adoptProjectName,
}: {
  item?: FlowCardItem;
  loading?: boolean;
  onClick?: () => void;
  onCopy?: () => void;
  onDelete?: () => void;
  onAdopt?: () => void;
  adoptProjectName?: string;
}) {
  if (loading) {
    return (
      <ItemCard>
        <div className="flex items-start gap-3">
          <Skeleton className="mt-0.5 h-5 w-5 shrink-0 rounded" />
          <div className="min-w-0 flex-1">
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="mt-1.5 h-3 w-3/4" />
            <Skeleton className="mt-2.5 h-5 w-40" />
          </div>
        </div>
      </ItemCard>
    );
  }

  if (!item) return null;
  const steps = item.steps ?? [];

  return (
    <ItemCard clickable onClick={onClick} data-testid="flow-card">
      <div className="flex items-start gap-3">
        <Workflow
          size={18}
          className="mt-0.5 shrink-0 text-muted-foreground transition-colors group-hover:text-primary"
        />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate text-sm font-semibold text-foreground transition-colors group-hover:text-primary">
              {item.name}
            </span>
            {item.isDefault && (
              <Badge variant="secondary" className="font-normal" data-testid="flow-default">
                {t("Default")}
              </Badge>
            )}
            {item.source === "built-in" && (
              <Badge variant="outline" className="font-normal text-muted-foreground">
                {t("built-in")}
              </Badge>
            )}
          </div>
          {item.description && (
            <div className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
              <Markdown inline>{item.description}</Markdown>
            </div>
          )}
          {steps.length > 0 ? (
            <div className="mt-2">
              <StepChips steps={steps} />
            </div>
          ) : (
            <p className="mt-2 text-[11px] text-muted-foreground">{t("No steps yet")}</p>
          )}
        </div>

        <div
          className="flex gap-1 opacity-0 transition-opacity group-hover:opacity-100"
          onClick={(e: React.MouseEvent) => e.stopPropagation()}
        >
          {onAdopt && (
            <SimpleTooltip
              content={adoptProjectName ? `Add to project: ${adoptProjectName}` : "Add to project"}
            >
              <Button variant="ghost" size="icon-xs" onClick={onAdopt} aria-label="Add to project">
                <FolderInput size={12} />
              </Button>
            </SimpleTooltip>
          )}
          {onCopy && (
            <SimpleTooltip content="Copy to edit">
              <Button variant="ghost" size="icon-xs" onClick={onCopy} aria-label="Copy to edit">
                <Copy size={12} />
              </Button>
            </SimpleTooltip>
          )}
          {onDelete && <ConfirmDeleteButton onDelete={onDelete} mode="icon" />}
        </div>
      </div>
    </ItemCard>
  );
}
