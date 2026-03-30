import { relativeTime, formatSize } from "./utils";

interface ResourceCardProps {
  name: string;
  path: string;
  entryCount?: number;
  size?: number;
  modified?: string;
  icon?: React.ReactNode;
  onClick: () => void;
  className?: string;
}

/**
 * Card for the resource picker landing page.
 * Shows resource name, path, entry count, last modified, file size.
 */
export function ResourceCard({
  name,
  path,
  entryCount,
  size,
  modified,
  icon,
  onClick,
  className,
}: ResourceCardProps) {
  return (
    <button
      onClick={onClick}
      className={`w-full text-left rounded-lg border border-border bg-card p-4 transition-all hover:border-primary/30 hover:shadow-md group ${className ?? ""}`}
    >
      <div className="flex items-start gap-3">
        {icon && (
          <div className="mt-0.5 text-muted-foreground group-hover:text-primary transition-colors">
            {icon}
          </div>
        )}
        <div className="flex-1 min-w-0">
          <div className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors truncate">
            {name}
          </div>
          <div className="text-[11px] text-muted-foreground truncate mt-0.5">{path}</div>
          <div className="flex items-center gap-3 mt-2 text-[11px] text-muted-foreground">
            {entryCount !== undefined && (
              <span>
                {entryCount} {entryCount === 1 ? "entry" : "entries"}
              </span>
            )}
            {size !== undefined && size > 0 && <span>{formatSize(size)}</span>}
            {modified && <span>{relativeTime(modified)}</span>}
          </div>
        </div>
      </div>
    </button>
  );
}
