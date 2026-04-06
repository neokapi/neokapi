/**
 * ConfirmDeleteButton — two-click delete with consistent UX.
 *
 * Three display modes:
 * - "icon" — ghost Trash2 icon, switches to Confirm/Cancel on click
 * - "text" — text "Delete" button, switches to Confirm/Cancel
 * - "inline" — small inline text for card footers
 *
 * Manages confirmation state internally. Built on shadcn Button.
 */

import { useState, useCallback } from "react";
import { Trash2 } from "lucide-react";
import { Button } from "./button";
import { cn } from "../../lib/utils";

export interface ConfirmDeleteButtonProps {
  /** Called when the user confirms the delete (second click). */
  onDelete: () => void;
  /** Display mode. */
  mode?: "icon" | "text" | "inline";
  /** Button size. */
  size?: "xs" | "sm";
  className?: string;
}

export function ConfirmDeleteButton({
  onDelete,
  mode = "icon",
  size = "xs",
  className,
}: ConfirmDeleteButtonProps) {
  const [confirming, setConfirming] = useState(false);

  const handleConfirm = useCallback(() => {
    onDelete();
    setConfirming(false);
  }, [onDelete]);

  const handleCancel = useCallback(() => {
    setConfirming(false);
  }, []);

  if (confirming) {
    return (
      <div className={cn("flex items-center gap-1", className)}>
        <Button
          variant="destructive"
          size={size}
          onClick={(e) => {
            e.stopPropagation();
            handleConfirm();
          }}
          className={mode === "inline" ? "h-auto px-1 py-0 text-[10px]" : undefined}
        >
          Confirm
        </Button>
        <Button
          variant="ghost"
          size={size}
          onClick={(e) => {
            e.stopPropagation();
            handleCancel();
          }}
          className={mode === "inline" ? "h-auto px-1 py-0 text-[10px]" : undefined}
        >
          Cancel
        </Button>
      </div>
    );
  }

  if (mode === "icon") {
    return (
      <Button
        variant="ghost"
        size="icon-xs"
        onClick={(e) => {
          e.stopPropagation();
          setConfirming(true);
        }}
        className={cn("hover:bg-destructive/10 hover:text-destructive", className)}
        aria-label="Delete"
      >
        <Trash2 size={12} />
      </Button>
    );
  }

  if (mode === "inline") {
    return (
      <Button
        variant="ghost"
        size={size}
        onClick={(e) => {
          e.stopPropagation();
          setConfirming(true);
        }}
        className={cn(
          "h-auto px-1 py-0 text-[10px] text-destructive/70 hover:text-destructive",
          className,
        )}
      >
        <Trash2 size={10} />
        Delete
      </Button>
    );
  }

  // mode === "text"
  return (
    <Button
      variant="ghost"
      size={size}
      onClick={(e) => {
        e.stopPropagation();
        setConfirming(true);
      }}
      className={cn("text-destructive/70 hover:text-destructive", className)}
    >
      <Trash2 size={12} />
      Delete
    </Button>
  );
}
