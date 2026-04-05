/**
 * CodeInput — CodeMirror 6 wrapper for syntax-highlighted editing.
 *
 * Supports language modes: "regex", "javascript", "json", "plain".
 * Designed as a drop-in replacement for <textarea> in schema-form widgets.
 */

import { useRef, useEffect, useCallback } from "react";
import {
  EditorView,
  keymap,
  placeholder as cmPlaceholder,
  type ViewUpdate,
} from "@codemirror/view";
import { EditorState, type Transaction } from "@codemirror/state";
import { defaultKeymap } from "@codemirror/commands";
import {
  syntaxHighlighting,
  HighlightStyle,
  StreamLanguage,
  type StringStream,
} from "@codemirror/language";
import { javascript } from "@codemirror/lang-javascript";
import { json } from "@codemirror/lang-json";
import { tags } from "@lezer/highlight";
import { cn } from "../../lib/utils";

export type CodeLanguage =
  | "regex"
  | "javascript"
  | "json"
  | "simplifier-rules"
  | "glob"
  | "target-path"
  | "plain";

export interface CodeInputProps {
  value: string;
  onChange: (value: string) => void;
  language?: CodeLanguage;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  singleLine?: boolean;
  minHeight?: number;
}

// Theme that inherits from CSS custom properties (works with any app theme)
const baseTheme = EditorView.theme({
  "&": {
    fontSize: "12px",
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
  },
  "&.cm-focused": {
    outline: "none",
  },
  ".cm-content": {
    padding: "8px 0",
    caretColor: "var(--foreground)",
  },
  ".cm-cursor, .cm-dropCursor": {
    borderLeftColor: "var(--foreground)",
  },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground": {
    background: "var(--accent) !important",
    opacity: "0.3",
  },
  ".cm-activeLine": {
    backgroundColor: "transparent",
  },
  ".cm-gutters": {
    display: "none",
  },
  ".cm-placeholder": {
    color: "var(--muted-foreground)",
    fontStyle: "italic",
  },
  ".cm-scroller": {
    overflow: "auto",
  },
});

// Theme-aware highlight style — uses OKLCH colors at lightness 0.75–0.8,
// which are readable on both light and dark backgrounds.
const codeHighlight = HighlightStyle.define([
  // Keywords: function, const, let, return, if, etc.
  { tag: tags.keyword, color: "oklch(0.75 0.15 300)" }, // violet
  // Strings
  { tag: tags.string, color: "oklch(0.75 0.14 150)" }, // green
  // Numbers
  { tag: tags.number, color: "oklch(0.78 0.14 75)" }, // amber
  // Comments
  { tag: tags.comment, color: "var(--muted-foreground)", fontStyle: "italic" },
  // Function/variable names
  { tag: tags.variableName, color: "oklch(0.78 0.12 220)" }, // sky
  { tag: tags.function(tags.variableName), color: "oklch(0.8 0.14 200)" }, // cyan
  // Properties
  { tag: tags.propertyName, color: "oklch(0.78 0.12 220)" }, // sky
  // Operators: = + - * / || &&
  { tag: tags.operator, color: "oklch(0.7 0.1 50)" }, // warm gray
  // Punctuation: ( ) { } [ ] , ;
  { tag: tags.paren, color: "oklch(0.75 0.15 300)" }, // violet
  { tag: tags.squareBracket, color: "oklch(0.78 0.14 150)" }, // green
  { tag: tags.brace, color: "oklch(0.78 0.14 75)" }, // amber
  { tag: tags.punctuation, color: "var(--muted-foreground)" },
  // Types / class names
  { tag: tags.typeName, color: "oklch(0.78 0.14 75)" }, // amber
  // Regex-specific: escape sequences
  { tag: tags.escape, color: "oklch(0.75 0.15 320)" }, // pink
  // Meta / annotations
  { tag: tags.meta, color: "oklch(0.7 0.12 250)" }, // blue
  // Boolean / null / undefined
  { tag: tags.bool, color: "oklch(0.78 0.14 75)" }, // amber
  { tag: tags.null, color: "oklch(0.78 0.14 75)" }, // amber
  // JSON keys
  { tag: tags.special(tags.propertyName), color: "oklch(0.78 0.12 220)" }, // sky
]);

// Simple stream tokenizer for Okapi simplifier rules grammar.
// Based on SimplifierRules.jj: `if FIELD OP "value" [and|or ...];`
const simplifierRulesLang = StreamLanguage.define({
  token(stream: StringStream) {
    // Whitespace
    if (stream.eatSpace()) return null;

    // Line comments: # ...
    if (stream.match("#")) {
      stream.skipToEnd();
      return "comment";
    }

    // Block comments: /* ... */
    if (stream.match("/*")) {
      while (!stream.match("*/", true)) {
        if (stream.next() == null) break;
      }
      return "comment";
    }

    // Strings: "..."
    if (stream.match('"')) {
      while (!stream.match('"', true)) {
        stream.next(); // handles escapes implicitly
        if (stream.eol()) break;
      }
      return "string";
    }

    // Operators: !=, !~, =, ~
    if (stream.match("!=") || stream.match("!~")) return "operator";
    if (stream.match("=") || stream.match("~")) return "operator";

    // Semicolons
    if (stream.match(";")) return "punctuation";

    // Parens
    if (stream.match("(") || stream.match(")")) return "paren";

    // Keywords and identifiers
    const wordMatch = stream.match(/^[a-zA-Z_]+/) as RegExpMatchArray | null;
    if (wordMatch) {
      const word = wordMatch[0];
      if (word === "if") return "keyword";
      if (word === "and" || word === "or") return "keyword";
      if (["DATA", "OUTER_DATA", "ORIGINAL_ID", "TYPE", "TAG_TYPE"].includes(word))
        return "variableName";
      if (["OPENING", "CLOSING", "STANDALONE"].includes(word)) return "string";
      if (["ADDABLE", "DELETABLE", "CLONEABLE"].includes(word)) return "bool";
      return null;
    }

    stream.next();
    return null;
  },
});

// Glob pattern tokenizer: highlights **, *, ?, {a,b}, [...], and / separators.
const globLang = StreamLanguage.define({
  token(stream: StringStream) {
    // Globstar **
    if (stream.match("**")) return "keyword";
    // Wildcard *
    if (stream.match("*")) return "keyword";
    // Single-char wildcard ?
    if (stream.match("?")) return "keyword";
    // Brace expansion {a,b,c}
    if (stream.match("{")) {
      while (!stream.match("}", true)) {
        if (stream.match(",")) continue;
        if (stream.next() == null) break;
      }
      return "brace";
    }
    // Character class [...]
    if (stream.match("[")) {
      if (stream.match("!") || stream.match("^")) {
        /* negation prefix */
      }
      while (!stream.match("]", true)) {
        if (stream.next() == null) break;
      }
      return "squareBracket";
    }
    // Path separator
    if (stream.match("/")) return "punctuation";
    // Literal text
    stream.match(/^[^*?{[/]+/);
    return null;
  },
});

// Target path tokenizer: highlights {variable} placeholders and / separators.
const targetPathLang = StreamLanguage.define({
  token(stream: StringStream) {
    // Variable placeholder {lang}, {locale}, etc.
    if (stream.match("{")) {
      stream.match(/^[^}]*/);
      stream.match("}");
      return "variableName";
    }
    // Glob tokens (target paths can also contain wildcards)
    if (stream.match("**")) return "keyword";
    if (stream.match("*")) return "keyword";
    // Path separator
    if (stream.match("/")) return "punctuation";
    // Literal text
    stream.match(/^[^{*/]+/);
    return null;
  },
});

function getLanguageExtension(lang: CodeLanguage) {
  switch (lang) {
    case "javascript":
      return javascript();
    case "json":
      return json();
    case "regex":
      return javascript();
    case "simplifier-rules":
      return simplifierRulesLang;
    case "glob":
      return globLang;
    case "target-path":
      return targetPathLang;
    case "plain":
    default:
      return [];
  }
}

export function CodeInput({
  value,
  onChange,
  language = "plain",
  placeholder,
  disabled = false,
  className,
  singleLine = false,
  minHeight,
}: CodeInputProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;

  // Create editor on mount
  useEffect(() => {
    if (!containerRef.current) return;

    const updateListener = EditorView.updateListener.of((update: ViewUpdate) => {
      if (update.docChanged) {
        onChangeRef.current(update.state.doc.toString());
      }
    });

    const extensions = [
      keymap.of(defaultKeymap),
      baseTheme,
      updateListener,
      EditorView.editable.of(!disabled),
      EditorState.readOnly.of(disabled),
      getLanguageExtension(language),
      syntaxHighlighting(codeHighlight),
      ...(placeholder ? [cmPlaceholder(placeholder)] : []),
      ...(singleLine
        ? [
            EditorState.transactionFilter.of((tr: Transaction) => {
              // Block newlines in single-line mode
              if (tr.newDoc.lines > 1) return [];
              return tr;
            }),
          ]
        : []),
    ];

    const state = EditorState.create({
      doc: value,
      extensions,
    });

    const view = new EditorView({
      state,
      parent: containerRef.current,
    });

    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // Recreate editor when language or disabled changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language, disabled]);

  // Sync external value changes (e.g., preset application)
  const syncValue = useCallback(() => {
    const view = viewRef.current;
    if (!view) return;
    const currentDoc = view.state.doc.toString();
    if (currentDoc !== value) {
      view.dispatch({
        changes: { from: 0, to: currentDoc.length, insert: value },
      });
    }
  }, [value]);

  useEffect(() => {
    syncValue();
  }, [syncValue]);

  return (
    <div
      ref={containerRef}
      data-slot="code-input"
      className={cn(
        "rounded-md border border-input bg-transparent text-sm",
        "focus-within:border-ring focus-within:ring-ring/50 focus-within:ring-[3px]",
        "[&_.cm-editor]:outline-none [&_.cm-content]:px-3",
        singleLine
          ? "[&_.cm-editor]:min-h-0 [&_.cm-content]:py-1.5"
          : "[&_.cm-editor]:min-h-[80px]",
        disabled && "opacity-50 cursor-not-allowed",
        className,
      )}
      style={minHeight ? { minHeight } : undefined}
    />
  );
}
