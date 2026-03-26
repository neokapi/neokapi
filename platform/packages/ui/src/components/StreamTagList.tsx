import type { StreamTag } from "../types/api";
import { StreamTagBadge } from "./StreamTagBadge";
import { Trash2 } from "./icons";

export interface StreamTagListProps {
  tags: StreamTag[];
  /** Called when the delete button is clicked. Omit to hide delete buttons. */
  onDelete?: (tagName: string) => void;
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

/** Vertical list of stream tags with optional delete action. */
export function StreamTagList({ tags, onDelete }: StreamTagListProps) {
  if (tags.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">No tags on this stream.</p>
    );
  }

  return (
    <ul className="divide-y divide-border">
      {tags.map((tag) => (
        <li key={tag.id} className="flex items-center justify-between gap-3 py-2 px-1">
          <div className="flex items-center gap-3 min-w-0">
            <StreamTagBadge tag={tag} />
            <span className="text-xs text-muted-foreground whitespace-nowrap">
              cursor {tag.cursor}
            </span>
            <span className="text-xs text-muted-foreground whitespace-nowrap">
              {formatDate(tag.created_at)}
            </span>
          </div>
          {onDelete && (
            <button
              type="button"
              onClick={() => onDelete(tag.name)}
              className="p-1 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors"
              title={`Delete tag ${tag.name}`}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}
