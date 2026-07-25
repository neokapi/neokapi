import { Button, Markdown, cn } from "@neokapi/ui-primitives";
import { ExternalLinkIcon } from "lucide-react";

export interface ToolDocViewerProps {
  /** Markdown content from fullDoc. */
  content: string;
  /** Wiki URL for external link. */
  wikiUrl?: string;
  /** Tool/format display name for the header. */
  title?: string;
  /** Additional class name. */
  className?: string;
}

/**
 * Renders full-page documentation for a tool or format.
 * Content is fullDoc markdown from the docs extraction pipeline, rendered
 * through the shared typeset {@link Markdown} primitive (react-markdown +
 * remark-gfm) — the same renderer used everywhere else markdown metadata is
 * displayed. See `web/docs/contribute/implementation/markdown-in-ui.md`.
 */
export function ToolDocViewer({ content, wikiUrl, title, className }: ToolDocViewerProps) {
  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {(title || wikiUrl) && (
        <div className="flex items-center justify-between">
          {title && <h2 className="text-lg font-semibold">{title}</h2>}
          {wikiUrl && (
            <Button variant="ghost" size="sm" asChild>
              <a
                href={wikiUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="gap-1.5 text-xs text-muted-foreground"
              >
                Okapi Wiki
                <ExternalLinkIcon className="size-3" />
              </a>
            </Button>
          )}
        </div>
      )}
      <Markdown>{content}</Markdown>
    </div>
  );
}
