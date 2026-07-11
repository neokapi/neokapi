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
 * `web/docs/contribute/notes-internal/markdown-in-ui.md` for the catalogue of
 * fields that carry markdown.
 *
 * Styling is expressed as Tailwind utilities on the element map (not a global
 * `prose`/`.typeset` class) so it renders correctly in any consumer whose
 * Tailwind build scans `@neokapi/ui-primitives/src` — which every consumer that
 * imports `styles/theme-tokens.css` already does. That keeps the component free
 * of per-app CSS-import wiring.
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
  /** Extra classes for the wrapper element. */
  className?: string;
}

const linkComponents: Partial<Components> = {
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
};

/** Block-level element map — full typeset prose. */
const blockComponents: Partial<Components> = {
  ...linkComponents,
  p: ({ children }) => <p className="mb-2 leading-relaxed last:mb-0">{children}</p>,
  h1: ({ children }) => (
    <h1 className="mt-4 mb-2 text-base font-semibold first:mt-0">{children}</h1>
  ),
  h2: ({ children }) => (
    <h2 className="mt-4 mb-2 text-sm font-semibold first:mt-0">{children}</h2>
  ),
  h3: ({ children }) => (
    <h3 className="mt-3 mb-1.5 text-sm font-semibold first:mt-0">{children}</h3>
  ),
  h4: ({ children }) => (
    <h4 className="mt-3 mb-1.5 text-xs font-semibold uppercase tracking-wide first:mt-0">
      {children}
    </h4>
  ),
  ul: ({ children }) => <ul className="my-2 ml-4 list-disc space-y-1">{children}</ul>,
  ol: ({ children }) => <ol className="my-2 ml-4 list-decimal space-y-1">{children}</ol>,
  li: ({ children }) => <li className="leading-relaxed">{children}</li>,
  strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
  em: ({ children }) => <em className="italic">{children}</em>,
  code: ({ children }) => (
    <code className="rounded bg-muted px-1 py-px font-mono text-[0.9em]">{children}</code>
  ),
  pre: ({ children }) => (
    <pre className="my-2 overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs leading-relaxed [&_code]:bg-transparent [&_code]:p-0">
      {children}
    </pre>
  ),
  blockquote: ({ children }) => (
    <blockquote className="my-2 border-l-2 border-border pl-3 text-muted-foreground italic">
      {children}
    </blockquote>
  ),
  hr: () => <hr className="my-3 border-border" />,
  table: ({ children }) => (
    <div className="my-2 overflow-x-auto">
      <table className="w-full border-collapse text-[0.95em]">{children}</table>
    </div>
  ),
  thead: ({ children }) => <thead className="border-b-2 border-border">{children}</thead>,
  th: ({ children }) => (
    <th className="py-1 pr-3 text-left font-semibold text-muted-foreground">{children}</th>
  ),
  td: ({ children }) => <td className="border-b border-border py-1 pr-3 align-top">{children}</td>,
};

/**
 * Inline element map — paragraphs collapse to inline flow; block constructs
 * degrade to lightweight inline equivalents so a clamped one-liner never shows
 * literal markup.
 */
const inlineComponents: Partial<Components> = {
  ...linkComponents,
  p: ({ children }) => <>{children}</>,
  strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
  em: ({ children }) => <em className="italic">{children}</em>,
  code: ({ children }) => (
    <code className="rounded bg-muted px-1 py-px font-mono text-[0.9em]">{children}</code>
  ),
  ul: ({ children }) => <span>{children}</span>,
  ol: ({ children }) => <span>{children}</span>,
  li: ({ children }) => <span className="[&:not(:first-child)]:before:content-['·_']">{children}</span>,
  h1: ({ children }) => <span className="font-semibold">{children}</span>,
  h2: ({ children }) => <span className="font-semibold">{children}</span>,
  h3: ({ children }) => <span className="font-semibold">{children}</span>,
  h4: ({ children }) => <span className="font-semibold">{children}</span>,
  hr: () => null,
  blockquote: ({ children }) => <span className="italic">{children}</span>,
};

export function Markdown({ children, inline = false, className }: MarkdownProps) {
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
    <div className={cn("text-sm text-foreground", className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={blockComponents}>
        {children}
      </ReactMarkdown>
    </div>
  );
}

export default Markdown;
