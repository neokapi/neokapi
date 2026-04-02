import * as React from "react";
import { ExternalLinkIcon } from "lucide-react";

import { cn } from "../../lib/utils";
import { Button } from "../ui/button";

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
 * Content is fullDoc markdown from the docs extraction pipeline.
 *
 * Uses simple HTML rendering of trusted markdown content.
 * The content comes from our own docs pipeline (not user input),
 * so it is safe to render directly. For untrusted content,
 * use a sanitizer like DOMPurify.
 */
export function ToolDocViewer({
  content,
  wikiUrl,
  title,
  className,
}: ToolDocViewerProps) {
  // Simple markdown → HTML conversion for headings, bold, code, lists.
  // Content is trusted (from our docs pipeline, not user input).
  const html = React.useMemo(() => markdownToHtml(content), [content]);

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {(title || wikiUrl) && (
        <div className="flex items-center justify-between">
          {title && (
            <h2 className="text-lg font-semibold">{title}</h2>
          )}
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
      {/* Safe: content is from our own docs pipeline, not user input */}
      <div
        className="prose prose-sm dark:prose-invert max-w-none"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}

/** Minimal markdown → HTML for documentation rendering. */
function markdownToHtml(md: string): string {
  return md
    // Fenced code blocks
    .replace(
      /```(\w*)\n([\s\S]*?)```/g,
      (_m, lang, code) =>
        `<pre><code class="language-${lang}">${escapeHtml(code.trim())}</code></pre>`,
    )
    // Inline code
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    // Headings
    .replace(/^#### (.+)$/gm, "<h4>$1</h4>")
    .replace(/^### (.+)$/gm, "<h3>$1</h3>")
    .replace(/^## (.+)$/gm, "<h2>$1</h2>")
    .replace(/^# (.+)$/gm, "<h1>$1</h1>")
    // Bold
    .replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>")
    // Blockquotes
    .replace(/^> (.+)$/gm, "<blockquote>$1</blockquote>")
    // Unordered lists
    .replace(/^- (.+)$/gm, "<li>$1</li>")
    .replace(/(<li>.*<\/li>\n?)+/g, (m) => `<ul>${m}</ul>`)
    // Horizontal rules
    .replace(/^---$/gm, "<hr>")
    // Paragraphs (double newline)
    .replace(/\n\n/g, "</p><p>")
    .replace(/^(.+)$/gm, (m) =>
      m.startsWith("<") ? m : `<p>${m}</p>`,
    );
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}
