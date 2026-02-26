import { useEffect } from "react";
import { BlockInfo } from "../../types/api";

interface UseVisualEditorKeyboardOptions {
  blocks: BlockInfo[];
  selectedIndex: number;
  editingIndex: number | null;
  onNavigate: (index: number) => void;
  onStartEditing: () => void;
  onCancelEditing: () => void;
  onSaveAndNext: () => void;
  onApprove: () => void;
  onReject: () => void;
  enabled: boolean;
}

export function useVisualEditorKeyboard(options: UseVisualEditorKeyboardOptions): void {
  const {
    blocks,
    selectedIndex,
    editingIndex,
    onNavigate,
    onStartEditing,
    onCancelEditing,
    onSaveAndNext,
    onApprove,
    onReject,
    enabled,
  } = options;

  useEffect(() => {
    if (!enabled) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't capture when focus is in an input or textarea
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;

      const isEditing = editingIndex !== null;
      const mod = e.ctrlKey || e.metaKey;

      // Approve: Ctrl/Cmd+Shift+A (works in any mode)
      if (mod && e.shiftKey && e.key === "A") {
        e.preventDefault();
        onApprove();
        return;
      }

      // Reject: Ctrl/Cmd+Shift+R (works in any mode)
      if (mod && e.shiftKey && e.key === "R") {
        e.preventDefault();
        onReject();
        return;
      }

      if (isEditing) {
        if (e.key === "Escape") {
          e.preventDefault();
          onCancelEditing();
        } else if (e.key === "Enter" && mod) {
          e.preventDefault();
          onSaveAndNext();
        }
        return;
      }

      // Navigation mode (not editing)
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          onNavigate(Math.min(selectedIndex + 1, blocks.length - 1));
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          onNavigate(Math.max(selectedIndex - 1, 0));
          break;
        case "Enter":
          e.preventDefault();
          onStartEditing();
          break;
        case "n":
          e.preventDefault();
          // Next untranslated block
          for (let i = selectedIndex + 1; i < blocks.length; i++) {
            if (Object.keys(blocks[i].targets).length === 0) {
              onNavigate(i);
              break;
            }
          }
          break;
        case "N":
          e.preventDefault();
          // Previous untranslated block
          for (let i = selectedIndex - 1; i >= 0; i--) {
            if (Object.keys(blocks[i].targets).length === 0) {
              onNavigate(i);
              break;
            }
          }
          break;
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [
    enabled,
    blocks,
    selectedIndex,
    editingIndex,
    onNavigate,
    onStartEditing,
    onCancelEditing,
    onSaveAndNext,
    onApprove,
    onReject,
  ]);
}
