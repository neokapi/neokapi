/**
 * ActionCard — clickable card for templates, presets, and quick actions.
 *
 * Used in ProjectSetupPage (templates), ProjectPresetPage (presets),
 * and HomePage (quick actions). Built on shadcn Button.
 */

import { Loader2 } from "lucide-react";
import { cn } from "../../lib/utils";

export interface ActionCardProps {
  /** Icon displayed on the left. */
  icon?: React.ReactNode;
  /** Card title. */
  title: string;
  /** Description text below the title. */
  description?: string;
  /** Optional badge rendered next to the title. */
  badge?: React.ReactNode;
  onClick: () => void;
  disabled?: boolean;
  loading?: boolean;
  /** Highlighted variant (e.g., for detected/recommended presets). */
  highlighted?: boolean;
  className?: string;
  children?: React.ReactNode;
}

export function ActionCard({
  icon,
  title,
  description,
  badge,
  onClick,
  disabled = false,
  loading = false,
  highlighted = false,
  className,
  children,
}: ActionCardProps) {
  return (
    <button
      onClick={onClick}
      disabled={disabled || loading}
      className={cn(
        "group flex w-full items-start gap-4 rounded-xl border p-5 text-left transition-colors",
        highlighted
          ? "border-primary/30 bg-primary/5 hover:border-primary/50 hover:bg-primary/10"
          : "border-border hover:border-primary/30 hover:bg-accent/30",
        disabled && "cursor-not-allowed opacity-50",
        className,
      )}
    >
      {icon && <div className="shrink-0 pt-0.5 text-primary">{icon}</div>}
      <div className="flex-1">
        <div className="flex items-center gap-2 text-sm font-medium">
          {title}
          {badge}
          {loading && <Loader2 size={14} className="animate-spin" />}
        </div>
        {description && (
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{description}</p>
        )}
        {children}
      </div>
    </button>
  );
}
