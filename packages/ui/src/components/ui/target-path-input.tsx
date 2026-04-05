/**
 * TargetPathInput — single-line CodeMirror input with target path syntax highlighting.
 *
 * Highlights: {variable} placeholders (e.g. {lang}, {locale}), ** and *
 * wildcards, and / path separators.
 */

import { CodeInput, type CodeInputProps } from "./code-input";
import { cn } from "../../lib/utils";

export interface TargetPathInputProps extends Omit<
  CodeInputProps,
  "language" | "singleLine" | "minHeight"
> {
  className?: string;
}

export function TargetPathInput({ className, ...props }: TargetPathInputProps) {
  return (
    <CodeInput
      {...props}
      language="target-path"
      singleLine
      className={cn("[&_.cm-content]:py-1", className)}
    />
  );
}
