import React, { useMemo } from "react";
import { cn } from "../../lib/utils";
import { detectLang, tokenize } from "./highlight";
import type { Lang, Token } from "./highlight";
import styles from "./CodeView.module.css";

export interface CodeViewProps {
  /** The source text to render. */
  text: string;
  /** Filename used to detect the language when `lang` is not given. */
  filename?: string;
  /** Override the detected language. */
  lang?: Lang;
  /** Show the line-number gutter (default true). */
  lineNumbers?: boolean;
  /** Zero-based line indices to highlight as changed (drives the change accent). */
  changedLines?: ReadonlySet<number>;
  /** Soft-wrap long lines instead of scrolling horizontally (default false). */
  wrap?: boolean;
  /** Max height before the view scrolls (CSS length). */
  maxHeight?: string;
  className?: string;
}

// CodeView renders syntax-highlighted, line-numbered source for the textual
// formats the lab handles. Chrome is built from the shared design tokens (so it
// matches the rest of the UI in both the docs site and Storybook); only the
// syntax token palette and the change accent live in a small CSS module, since
// there is no primitive for code highlighting. Highlighting is purely
// presentational and never alters the bytes.
export default function CodeView({
  text,
  filename,
  lang,
  lineNumbers = true,
  changedLines,
  wrap = false,
  maxHeight = "26rem",
  className,
}: CodeViewProps): React.ReactElement {
  const language = lang ?? (filename ? detectLang(filename) : "text");
  const lines = useMemo(() => tokenize(text, language), [text, language]);

  return (
    <div
      className={cn(
        "kapi-reference overflow-auto rounded-lg border bg-card",
        styles.code,
        className,
      )}
      style={{ maxHeight }}
      data-lang={language}
    >
      <pre className="m-0 bg-transparent py-2 font-mono text-[0.8rem] leading-[1.55]">
        <code>
          {lines.map((tokens, i) => (
            <span
              key={i}
              className={cn(
                "flex items-start pr-3 hover:bg-muted/60",
                changedLines?.has(i) && styles.changed,
              )}
            >
              {lineNumbers && (
                <span className="mr-3 w-11 shrink-0 border-r border-border pr-3 text-right text-muted-foreground/70 tabular-nums select-none">
                  {i + 1}
                </span>
              )}
              <span
                className={cn("flex-1 whitespace-pre", wrap && "whitespace-pre-wrap break-words")}
              >
                {tokens.length === 0 ? (
                  <span> </span>
                ) : (
                  tokens.map((t, j) => <TokenSpan key={j} token={t} />)
                )}
              </span>
            </span>
          ))}
        </code>
      </pre>
    </div>
  );
}

function TokenSpan({ token }: { token: Token }): React.ReactElement {
  if (token.type === "text") return <>{token.text}</>;
  return <span className={styles[`t_${token.type}`]}>{token.text}</span>;
}
