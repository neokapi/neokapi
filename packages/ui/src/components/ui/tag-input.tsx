/**
 * TagInput — chip-based input for comma-separated tags/values.
 *
 * Displays existing values as removable pills. New values are added by
 * typing and pressing Enter or comma. Backspace removes the last tag
 * when the input is empty.
 */

import { useState, useRef, useCallback } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { X } from "lucide-react";
import { cn } from "../../lib/utils";

export interface TagInputProps {
  value: string[];
  onChange: (value: string[]) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

export function TagInput({
  value,
  onChange,
  placeholder = t("Add tag..."),
  disabled,
  className,
}: TagInputProps) {
  const [input, setInput] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const addTag = useCallback(
    (tag: string) => {
      const trimmed = tag.trim();
      if (!trimmed || value.includes(trimmed)) return;
      onChange([...value, trimmed]);
    },
    [value, onChange],
  );

  const removeTag = useCallback(
    (index: number) => {
      const next = [...value];
      next.splice(index, 1);
      onChange(next);
    },
    [value, onChange],
  );

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if ((e.key === "Enter" || e.key === ",") && input.trim()) {
      e.preventDefault();
      addTag(input);
      setInput("");
    } else if (e.key === "Backspace" && !input && value.length > 0) {
      removeTag(value.length - 1);
    }
  };

  return (
    <div
      data-slot="tag-input"
      className={cn(
        "flex flex-wrap items-center gap-1 rounded-md border border-input bg-transparent px-2 py-1.5",
        "focus-within:border-ring focus-within:ring-ring/50 focus-within:ring-[3px]",
        disabled && "opacity-50 cursor-not-allowed",
        className,
      )}
      onClick={() => inputRef.current?.focus()}
    >
      {value.map((tag, i) => (
        <span
          key={`${tag}-${i}`}
          className="inline-flex items-center gap-0.5 px-1.5 py-0.5 text-xs rounded bg-secondary text-secondary-foreground"
        >
          {tag}
          {!disabled && (
            <button
              type="button"
              className="text-muted-foreground hover:text-foreground"
              onClick={(e) => {
                e.stopPropagation();
                removeTag(i);
              }}
            >
              <X className="size-3" />
            </button>
          )}
        </span>
      ))}
      <input
        ref={inputRef}
        value={input}
        placeholder={value.length === 0 ? placeholder : ""}
        disabled={disabled}
        className="flex-1 min-w-[60px] bg-transparent text-xs outline-none placeholder:text-muted-foreground"
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={() => {
          if (input.trim()) {
            addTag(input);
            setInput("");
          }
        }}
      />
    </div>
  );
}
