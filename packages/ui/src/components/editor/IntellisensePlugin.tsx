import { useState, useEffect, useCallback, useRef } from "react";
import { createPortal } from "react-dom";
import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import {
  $getSelection,
  $isRangeSelection,
  COMMAND_PRIORITY_CRITICAL,
  KEY_ESCAPE_COMMAND,
  KEY_ENTER_COMMAND,
} from "lexical";
import type { SpanInfo } from "../../types/span";
import { $createTagChipNode } from "./TagChipNode";
import { TagChipComponent } from "./TagChipComponent";
import { useAvailableSpans } from "./useAvailableSpans";
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
} from "../ui/command";

interface IntellisensePluginProps {
  sourceSpans: SpanInfo[];
  usedSpans: SpanInfo[];
}

const categoryLabels: Record<string, string> = {
  formatting: "Formatting",
  linking: "Links",
  media: "Media",
  structure: "Structure",
  code: "Code",
  generic: "Other",
};

/**
 * Ctrl+Space intellisense dropdown for inserting inline code tags.
 * Uses cmdk (Command) for fuzzy filtering and keyboard navigation.
 */
export function IntellisensePlugin({ sourceSpans, usedSpans }: IntellisensePluginProps) {
  const [editor] = useLexicalComposerContext();
  const [isOpen, setIsOpen] = useState(false);
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const { groups } = useAvailableSpans(sourceSpans, usedSpans);

  // Ctrl+Space to open
  useEffect(() => {
    const root = editor.getRootElement();
    if (!root) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === " ") {
        e.preventDefault();

        const nativeSelection = window.getSelection();
        if (!nativeSelection || nativeSelection.rangeCount === 0) return;

        const range = nativeSelection.getRangeAt(0);
        const rect = range.getBoundingClientRect();

        setPosition({
          top: rect.bottom + window.scrollY + 4,
          left: rect.left + window.scrollX,
        });
        setIsOpen(true);
      }
    };

    root.addEventListener("keydown", handleKeyDown);
    return () => root.removeEventListener("keydown", handleKeyDown);
  }, [editor]);

  // Intercept Escape and Enter in the editor when dropdown is open
  useEffect(() => {
    if (!isOpen) return;

    const unregEscape = editor.registerCommand(
      KEY_ESCAPE_COMMAND,
      () => {
        setIsOpen(false);
        editor.focus();
        return true;
      },
      COMMAND_PRIORITY_CRITICAL,
    );

    const unregEnter = editor.registerCommand(
      KEY_ENTER_COMMAND,
      () => true, // Swallow Enter — cmdk handles it
      COMMAND_PRIORITY_CRITICAL,
    );

    return () => {
      unregEscape();
      unregEnter();
    };
  }, [editor, isOpen]);

  // Close on outside click
  useEffect(() => {
    if (!isOpen) return;

    const handleClick = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setIsOpen(false);
        editor.focus();
      }
    };

    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [isOpen, editor]);

  const insertTag = useCallback(
    (span: SpanInfo) => {
      editor.update(() => {
        const selection = $getSelection();
        if ($isRangeSelection(selection)) {
          selection.insertNodes([$createTagChipNode(span)]);
        }
      });
      setIsOpen(false);
      editor.focus();
    },
    [editor],
  );

  const insertPair = useCallback(
    (opening: SpanInfo, closing: SpanInfo) => {
      editor.update(() => {
        const selection = $getSelection();
        if ($isRangeSelection(selection)) {
          const openNode = $createTagChipNode(opening);
          const closeNode = $createTagChipNode(closing);
          selection.insertNodes([openNode, closeNode]);
        }
      });
      setIsOpen(false);
      editor.focus();
    },
    [editor],
  );

  if (!isOpen || !position || sourceSpans.length === 0) return null;

  // Group by category for display
  const categoryMap = new Map<string, typeof groups>();
  for (const group of groups) {
    const cat = group.category;
    if (!categoryMap.has(cat)) categoryMap.set(cat, []);
    categoryMap.get(cat)!.push(group);
  }

  return createPortal(
    <div
      ref={dropdownRef}
      className="fixed z-50 animate-in fade-in-0 slide-in-from-top-1 duration-100"
      style={{ top: position.top, left: position.left }}
    >
      <Command
        className="w-64 rounded-lg border shadow-lg"
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            setIsOpen(false);
            editor.focus();
          }
        }}
      >
        <CommandInput placeholder="Search tags..." autoFocus />
        <CommandList>
          <CommandEmpty>No matching tags</CommandEmpty>
          {[...categoryMap.entries()].map(([category, catGroups]) => (
            <CommandGroup key={category} heading={categoryLabels[category] || category}>
              {catGroups.map((group) => {
                const opening = group.items.find((i) => i.span.span_type === "opening");
                const closing = group.items.find((i) => i.span.span_type === "closing");
                const placeholder = group.items.find((i) => i.span.span_type === "placeholder");

                if (opening && closing) {
                  const blocked = opening.blocked || closing.blocked;
                  return (
                    <CommandItem
                      key={`pair-${group.pairIndex}`}
                      value={`${opening.label} ${closing.label} ${opening.span.type}`}
                      onSelect={() => !blocked && insertPair(opening.span, closing.span)}
                      disabled={blocked}
                    >
                      <span className="flex items-center gap-0.5">
                        <TagChipComponent spanInfo={opening.span} />
                        <TagChipComponent spanInfo={closing.span} />
                      </span>
                      <span className="text-xs text-muted-foreground truncate">
                        {opening.span.type}
                      </span>
                      {opening.sourceIndex < 9 && (
                        <CommandShortcut>
                          {"\u2318"}
                          {opening.sourceIndex + 1}
                        </CommandShortcut>
                      )}
                    </CommandItem>
                  );
                }

                if (placeholder) {
                  return (
                    <CommandItem
                      key={`ph-${group.pairIndex}`}
                      value={`${placeholder.label} ${placeholder.span.type} ${placeholder.span.data}`}
                      onSelect={() => !placeholder.blocked && insertTag(placeholder.span)}
                      disabled={placeholder.blocked}
                    >
                      <TagChipComponent spanInfo={placeholder.span} />
                      <span className="text-xs text-muted-foreground truncate">
                        {placeholder.span.type}
                      </span>
                      {placeholder.sourceIndex < 9 && (
                        <CommandShortcut>
                          {"\u2318"}
                          {placeholder.sourceIndex + 1}
                        </CommandShortcut>
                      )}
                    </CommandItem>
                  );
                }

                return null;
              })}
            </CommandGroup>
          ))}
        </CommandList>
      </Command>
    </div>,
    document.body,
  );
}
