// Re-exported as .ts — uses createElement instead of JSX to avoid .tsx requirement.
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { createElement } from "react";
import { theme } from "../utils";

export function Md({ text, style }: { text: string; style?: React.CSSProperties }) {
  return createElement(Markdown, {
    remarkPlugins: [remarkGfm],
    components: {
      p: ({ children }: { children?: React.ReactNode }) =>
        createElement("p", { style: { ...style, margin: "0 0 4px 0" } }, children),
      strong: ({ children }: { children?: React.ReactNode }) =>
        createElement("strong", null, children),
      em: ({ children }: { children?: React.ReactNode }) =>
        createElement("em", null, children),
      code: ({ children }: { children?: React.ReactNode }) =>
        createElement(
          "code",
          {
            style: {
              padding: "1px 4px",
              borderRadius: 3,
              background: theme.bgMuted,
              fontFamily: "var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace)",
              fontSize: "0.9em",
            },
          },
          children,
        ),
      a: ({ href, children }: { href?: string; children?: React.ReactNode }) =>
        createElement(
          "a",
          {
            href,
            target: "_blank",
            rel: "noopener noreferrer",
            style: {
              color: theme.ring,
              textDecoration: "underline",
              textDecorationColor: `color-mix(in oklch, ${theme.ring} 40%, transparent)`,
            },
          },
          children,
        ),
      ul: ({ children }: { children?: React.ReactNode }) =>
        createElement("ul", { style: { margin: "4px 0", paddingLeft: 16, listStyleType: "disc" } }, children),
      ol: ({ children }: { children?: React.ReactNode }) =>
        createElement("ol", { style: { margin: "4px 0", paddingLeft: 16 } }, children),
      li: ({ children }: { children?: React.ReactNode }) =>
        createElement("li", { style: { marginBottom: 2 } }, children),
      table: ({ children }: { children?: React.ReactNode }) =>
        createElement(
          "table",
          {
            style: {
              margin: "6px 0",
              borderCollapse: "collapse" as const,
              fontSize: "0.95em",
              width: "100%",
            },
          },
          children,
        ),
      thead: ({ children }: { children?: React.ReactNode }) =>
        createElement("thead", { style: { borderBottom: `2px solid ${theme.border}` } }, children),
      th: ({ children }: { children?: React.ReactNode }) =>
        createElement(
          "th",
          {
            style: {
              textAlign: "left" as const,
              padding: "3px 8px 3px 0",
              fontWeight: 600,
              fontSize: "0.9em",
              color: theme.fgSecondary,
            },
          },
          children,
        ),
      td: ({ children }: { children?: React.ReactNode }) =>
        createElement(
          "td",
          {
            style: {
              padding: "2px 8px 2px 0",
              borderBottom: `1px solid ${theme.border}`,
              verticalAlign: "top" as const,
            },
          },
          children,
        ),
    },
    children: text,
  } as React.ComponentProps<typeof Markdown>);
}
