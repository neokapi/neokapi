// Glass modal components aliased to Dialog names for shadcn compatibility.
//
// The library's cva size variants (sm:max-w-[480px] etc.) live in node_modules
// and are not picked up by Tailwind v4's source detection. We replicate the
// size → max-width mapping here so the classes exist in our own source.
import React from "react";
import {
  ModalRoot,
  ModalTrigger,
  ModalContent,
  ModalHeader,
  ModalFooter,
  ModalTitle,
  ModalDescription,
  ModalClose,
  ModalGlass,
} from "shadcn-glass-ui";
import type { ModalRootProps, ModalContentProps } from "shadcn-glass-ui";
import { cn } from "../../lib/utils";

type DialogSize = "sm" | "md" | "lg" | "xl" | "full";

const sizeClasses: Record<DialogSize, string> = {
  sm: "sm:max-w-[480px]",
  md: "sm:max-w-[640px]",
  lg: "sm:max-w-[800px]",
  xl: "sm:max-w-xl",
  full: "sm:max-w-4xl",
};

const DialogContent = React.forwardRef<
  React.ComponentRef<typeof ModalContent>,
  React.ComponentPropsWithoutRef<typeof ModalContent>
>(({ className, size = "sm", ...props }, ref) => (
  <ModalContent
    ref={ref}
    className={cn(sizeClasses[size as DialogSize], className)}
    size={size}
    {...props}
  />
));
DialogContent.displayName = "DialogContent";

export {
  ModalRoot as Dialog,
  ModalTrigger as DialogTrigger,
  DialogContent,
  ModalHeader as DialogHeader,
  ModalFooter as DialogFooter,
  ModalTitle as DialogTitle,
  ModalDescription as DialogDescription,
  ModalClose as DialogClose,
  ModalGlass,
};
export type { ModalRootProps as DialogProps, ModalContentProps as DialogContentProps };
