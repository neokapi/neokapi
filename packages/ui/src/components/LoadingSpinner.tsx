import * as React from "react";
import { Loader2 } from "lucide-react";
import { cn } from "../lib/utils";

interface LoadingSpinnerProps {
  size?: "sm" | "md" | "lg";
  text?: string;
  className?: string;
}

const sizeMap = { sm: 12, md: 16, lg: 24 };

export function LoadingSpinner({ size = "md", text, className }: LoadingSpinnerProps) {
  return (
    <div className={cn("flex items-center gap-2 text-muted-foreground", className)}>
      <Loader2 size={sizeMap[size]} className="animate-spin" />
      {text && <span className="text-sm">{text}</span>}
    </div>
  );
}
