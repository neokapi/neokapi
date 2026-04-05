/**
 * GlobInput — single-line CodeMirror input with glob pattern syntax highlighting.
 *
 * Highlights: ** (globstar), * (wildcard), ? (single char), {a,b} (braces),
 * [...] (character classes), and / (path separators).
 */

import { CodeInput, type CodeInputProps } from "./code-input";
import { cn } from "../../lib/utils";

export interface GlobInputProps extends Omit<
  CodeInputProps,
  "language" | "singleLine" | "minHeight"
> {
  className?: string;
}

export function GlobInput({ className, ...props }: GlobInputProps) {
  return (
    <CodeInput
      {...props}
      language="glob"
      singleLine
      className={cn("[&_.cm-content]:py-1", className)}
    />
  );
}
