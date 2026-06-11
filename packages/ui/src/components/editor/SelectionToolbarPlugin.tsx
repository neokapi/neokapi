import { useState, useEffect, useCallback, useRef } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { createPortal } from "react-dom";
import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { $getSelection, $isRangeSelection, $createTextNode } from "lexical";
import type { SpanInfo } from "../../types/span";
import { $createTagChipNode } from "./TagChipNode";
import { TagChipComponent } from "./TagChipComponent";
import { useAvailableSpans, type PairGroup } from "./useAvailableSpans";
import { cn } from "../../lib/utils";

interface SelectionToolbarPluginProps {
  sourceSpans: SpanInfo[];
  usedSpans: SpanInfo[];
}

/**
 * Floating toolbar that appears above text selections, allowing users
 * to wrap the selection with an opening/closing tag pair or replace it
 * with a placeholder tag.
 */
export function SelectionToolbarPlugin({ sourceSpans, usedSpans }: SelectionToolbarPluginProps) {
  const [editor] = useLexicalComposerContext();
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null);
  const toolbarRef = useRef<HTMLDivElement>(null);
  const { groups } = useAvailableSpans(sourceSpans, usedSpans);

  useEffect(() => {
    return editor.registerUpdateListener(({ editorState }) => {
      editorState.read(() => {
        const selection = $getSelection();
        if (!$isRangeSelection(selection) || selection.isCollapsed()) {
          setPosition(null);
          return;
        }

        const nativeSelection = window.getSelection();
        if (!nativeSelection || nativeSelection.rangeCount === 0) {
          setPosition(null);
          return;
        }

        const range = nativeSelection.getRangeAt(0);
        const rect = range.getBoundingClientRect();
        if (rect.width === 0) {
          setPosition(null);
          return;
        }

        setPosition({
          top: rect.top + window.scrollY - 4,
          left: rect.left + window.scrollX + rect.width / 2,
        });
      });
    });
  }, [editor]);

  const wrapWithPair = useCallback(
    (opening: SpanInfo, closing: SpanInfo) => {
      editor.update(() => {
        const selection = $getSelection();
        if (!$isRangeSelection(selection) || selection.isCollapsed()) return;

        // Get the selected text content before modifying
        const selectedText = selection.getTextContent();

        // Remove the selected content and insert opening + text + closing
        selection.removeText();

        const openNode = $createTagChipNode(opening);
        const textNode = $createTextNode(selectedText);
        const closeNode = $createTagChipNode(closing);

        selection.insertNodes([openNode, textNode, closeNode]);
      });
      setPosition(null);
    },
    [editor],
  );

  const replaceWithTag = useCallback(
    (span: SpanInfo) => {
      editor.update(() => {
        const selection = $getSelection();
        if (!$isRangeSelection(selection)) return;
        // Use the selected text as the entity/placeholder value in the target.
        const selectedText = selection.getTextContent();
        const targetSpan: SpanInfo = selectedText
          ? { ...span, data: selectedText, display_text: selectedText }
          : span;
        selection.insertNodes([$createTagChipNode(targetSpan)]);
      });
      setPosition(null);
    },
    [editor],
  );

  if (!position || sourceSpans.length === 0) return null;

  return createPortal(
    <div
      ref={toolbarRef}
      className="fixed z-50 -translate-x-1/2 -translate-y-full animate-in fade-in-0 zoom-in-95 duration-100"
      style={{ top: position.top, left: position.left }}
      onMouseDown={(e) => e.preventDefault()}
    >
      <div className="flex items-center gap-0.5 rounded-lg border bg-popover p-1 shadow-md">
        {groups.map((group) => (
          <ToolbarPairButton
            key={group.pairIndex}
            group={group}
            onWrap={wrapWithPair}
            onReplace={replaceWithTag}
          />
        ))}
      </div>
    </div>,
    document.body,
  );
}

function ToolbarPairButton({
  group,
  onWrap,
  onReplace,
}: {
  group: PairGroup;
  onWrap: (opening: SpanInfo, closing: SpanInfo) => void;
  onReplace: (span: SpanInfo) => void;
}) {
  const opening = group.items.find((i) => i.span.span_type === "opening");
  const closing = group.items.find((i) => i.span.span_type === "closing");
  const placeholder = group.items.find((i) => i.span.span_type === "placeholder");

  // Pair: wrap selection
  if (opening && closing) {
    const blocked = opening.blocked || closing.blocked;
    return (
      <button
        className={cn(
          "flex items-center gap-0.5 rounded px-1 py-0.5 text-xs transition-colors hover:bg-accent",
          blocked && "opacity-40 cursor-not-allowed",
        )}
        onClick={() => !blocked && onWrap(opening.span, closing.span)}
        disabled={blocked}
        title={
          blocked
            ? t("Tag already used")
            : t("Wrap with {opening}...{closing}", {
                opening: opening.label,
                closing: closing.label,
              })
        }
      >
        <TagChipComponent spanInfo={opening.span} />
        <span className="text-[10px] text-muted-foreground mx-px">...</span>
        <TagChipComponent spanInfo={closing.span} />
      </button>
    );
  }

  // Placeholder: replace selection
  if (placeholder) {
    return (
      <button
        className={cn(
          "flex items-center rounded px-1 py-0.5 text-xs transition-colors hover:bg-accent",
          placeholder.blocked && "opacity-40 cursor-not-allowed",
        )}
        onClick={() => !placeholder.blocked && onReplace(placeholder.span)}
        disabled={placeholder.blocked}
        title={
          placeholder.blocked
            ? t("Tag already used")
            : t("Replace with {label}", { label: placeholder.label })
        }
      >
        <TagChipComponent spanInfo={placeholder.span} />
      </button>
    );
  }

  return null;
}
