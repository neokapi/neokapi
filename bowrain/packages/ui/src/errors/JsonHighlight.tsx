import { useMemo } from "react";
import { cn } from "@neokapi/ui-primitives";

/**
 * Dependency-free JSON syntax highlighting: serialize with JSON.stringify and
 * tokenize the (guaranteed-valid) output into styled spans. Colors use
 * Tailwind palette classes with dark: variants so both themes read well.
 */

type TokenType = "key" | "string" | "number" | "literal" | "punct" | "ws";

interface Token {
  type: TokenType;
  text: string;
}

const TOKEN_CLASS: Record<TokenType, string | undefined> = {
  key: "text-sky-700 dark:text-sky-300",
  string: "text-emerald-700 dark:text-emerald-300",
  number: "text-amber-700 dark:text-amber-400",
  literal: "text-fuchsia-700 dark:text-fuchsia-300",
  punct: "text-muted-foreground",
  ws: undefined,
};

const TOKEN_RE =
  /("(?:\\.|[^"\\])*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|(true|false|null)|([{}[\],:])|(\s+)/g;

/** Tokenize a JSON.stringify-produced string into typed tokens. */
export function tokenizeJson(text: string): Token[] {
  const tokens: Token[] = [];
  let match: RegExpExecArray | null;
  let last = 0;
  TOKEN_RE.lastIndex = 0;
  while ((match = TOKEN_RE.exec(text)) !== null) {
    if (match.index > last) {
      tokens.push({ type: "ws", text: text.slice(last, match.index) });
    }
    const [, str, keyColon, num, lit, punct, ws] = match;
    if (str !== undefined) {
      tokens.push({ type: keyColon !== undefined ? "key" : "string", text: str });
      if (keyColon !== undefined) tokens.push({ type: "punct", text: keyColon });
    } else if (num !== undefined) {
      tokens.push({ type: "number", text: num });
    } else if (lit !== undefined) {
      tokens.push({ type: "literal", text: lit });
    } else if (punct !== undefined) {
      tokens.push({ type: "punct", text: punct });
    } else if (ws !== undefined) {
      tokens.push({ type: "ws", text: ws });
    }
    last = TOKEN_RE.lastIndex;
  }
  if (last < text.length) tokens.push({ type: "ws", text: text.slice(last) });
  return tokens;
}

export interface JsonHighlightProps {
  /** Any serializable value; rendered as pretty-printed, highlighted JSON. */
  value: unknown;
  className?: string;
}

/** Pretty-printed, syntax-highlighted JSON. Wrap in a `<pre>` for layout. */
export function JsonHighlight({ value, className }: JsonHighlightProps) {
  const tokens = useMemo(() => {
    let text: string;
    try {
      text = JSON.stringify(value, null, 2) ?? String(value);
    } catch {
      text = String(value);
    }
    return tokenizeJson(text);
  }, [value]);

  return (
    <code className={cn("font-mono", className)}>
      {tokens.map((t, i) =>
        t.type === "ws" ? (
          t.text
        ) : (
          <span key={i} className={TOKEN_CLASS[t.type]}>
            {t.text}
          </span>
        ),
      )}
    </code>
  );
}
