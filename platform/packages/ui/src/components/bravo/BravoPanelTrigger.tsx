import { cn } from "../../lib/utils";

export interface BravoPanelTriggerProps {
  onClick: () => void;
  active?: boolean;
  hasUnread?: boolean;
}

/** Friendly bot icon for the top bar. */
function BravoIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={cn("size-[18px]", className)}
    >
      {/* Head */}
      <rect x="3" y="4" width="18" height="14" rx="4" />
      {/* Eyes */}
      <circle cx="9" cy="11" r="1.5" fill="currentColor" stroke="none" />
      <circle cx="15" cy="11" r="1.5" fill="currentColor" stroke="none" />
      {/* Antenna */}
      <line x1="12" y1="4" x2="12" y2="1" />
      <circle cx="12" cy="0.5" r="1" fill="currentColor" stroke="none" />
      {/* Mouth */}
      <path d="M9.5 14.5 Q12 16.5 14.5 14.5" fill="none" />
    </svg>
  );
}

export function BravoPanelTrigger({ onClick, active, hasUnread }: BravoPanelTriggerProps) {
  return (
    <button
      className={cn(
        "relative flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-colors",
        active
          ? "text-primary"
          : "text-muted-foreground hover:text-foreground",
      )}
      onClick={onClick}
      aria-label="Toggle @bravo assistant"
    >
      <BravoIcon />
      {hasUnread && (
        <span className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-primary animate-pulse" />
      )}
    </button>
  );
}
