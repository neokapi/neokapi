import * as React from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { cn } from "../../lib/utils";

/**
 * Markdown — the single "typeset" prose primitive for neokapi UIs.
 *
 * Every surface that renders a markdown-bearing metadata field (tool / format /
 * flow / plugin descriptions, overviews, parameter help, example descriptions,
 * long-form docs) should render it through this component rather than dropping
 * the raw string into JSX. See
 * `web/docs/contribute/implementation/markdown-in-ui.md` for the catalogue of
 * fields that carry markdown.
 *
 * Rendering is react-markdown + remark-gfm; block styling is the shared
 * `.typeset` layer (styles/typeset.css, the shadcn/typeset contract), imported
 * via `@neokapi/ui-primitives/styles/theme-tokens.css` — which every consumer
 * already imports, so there is no per-app CSS wiring. The element markup is
 * react-markdown's default HTML; typeset styles it. `inline` mode keeps a small
 * bespoke path (typeset is block-oriented) for clamped rows, cells, tooltips.
 */

export interface MarkdownProps {
  /** The markdown source string. Renders nothing when empty/undefined. */
  children?: string | null;
  /**
   * Inline/compact mode: unwraps paragraphs to inline flow and drops block
   * spacing so the result sits happily inside a clamped list row, table cell,
   * or tooltip. Emphasis, inline code, and links still render.
   */
  inline?: boolean;
  /**
   * Typeset variant for the block layer — tunes size/leading/flow. "docs"
   * (default) is the detail-view reading rhythm; "chat" is tighter.
   */
  variant?: "docs" | "chat";
  /** Extra classes for the wrapper element. */
  className?: string;
}

/** External links open in a new tab; typeset styles the anchor itself. */
const blockComponents: Partial<Components> = {
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
};

/**
 * Inline element map — paragraphs collapse to inline flow; block constructs
 * degrade to lightweight inline equivalents so a clamped one-liner never shows
 * literal markup. Self-styled (this path is not inside a `.typeset` container).
 */
const inlineComponents: Partial<Components> = {
  a: ({ href, children }) => (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="text-primary underline decoration-primary/40 underline-offset-2 hover:decoration-primary"
    >
      {children}
    </a>
  ),
  p: ({ children }) => <>{children}</>,
  strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
  em: ({ children }) => <em className="italic">{children}</em>,
  code: ({ children }) => (
    <code className="rounded bg-muted px-1 py-px font-mono text-[0.9em]">{children}</code>
  ),
  ul: ({ children }) => <span>{children}</span>,
  ol: ({ children }) => <span>{children}</span>,
  li: ({ children }) => (
    <span className="[&:not(:first-child)]:before:content-['·_']">{children}</span>
  ),
  h1: ({ children }) => <span className="font-semibold">{children}</span>,
  h2: ({ children }) => <span className="font-semibold">{children}</span>,
  h3: ({ children }) => <span className="font-semibold">{children}</span>,
  h4: ({ children }) => <span className="font-semibold">{children}</span>,
  hr: () => null,
  blockquote: ({ children }) => <span className="italic">{children}</span>,
};

export function Markdown({ children, inline = false, variant = "docs", className }: MarkdownProps) {
  if (!children || !children.trim()) return null;

  if (inline) {
    return (
      <span className={cn("[&>*]:inline", className)}>
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={inlineComponents}>
          {children}
        </ReactMarkdown>
      </span>
    );
  }

  return (
    <div className={cn("typeset", variant === "chat" ? "typeset-chat" : "typeset-docs", className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={blockComponents}>
        {children}
      </ReactMarkdown>
    </div>
  );
}

export default Markdown;
