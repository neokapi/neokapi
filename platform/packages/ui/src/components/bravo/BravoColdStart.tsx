/** Cold start animation shown while the agent container is provisioning. */
export function BravoColdStart() {
  return (
    <div className="flex flex-col items-center justify-center gap-4 py-16 px-8">
      {/* Animated bot icon */}
      <div className="relative">
        <svg
          viewBox="0 0 48 48"
          className="size-16 text-primary/70 animate-bounce"
          style={{ animationDuration: "2s" }}
          fill="none"
          stroke="currentColor"
          strokeWidth={1.5}
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <rect x="6" y="8" width="36" height="28" rx="8" />
          <circle cx="18" cy="22" r="3" fill="currentColor" stroke="none" />
          <circle cx="30" cy="22" r="3" fill="currentColor" stroke="none" />
          <path d="M19 30 Q24 34 29 30" />
          <line x1="24" y1="8" x2="24" y2="2" />
          <circle cx="24" cy="1" r="2" fill="currentColor" stroke="none" />
        </svg>
        {/* Pulse ring */}
        <div
          className="absolute inset-0 rounded-full border-2 border-primary/20 animate-ping"
          style={{ animationDuration: "2s" }}
        />
      </div>

      <div className="text-center space-y-1.5">
        <p className="text-sm font-medium text-foreground">Waking up @bravo...</p>
        <p className="text-xs text-muted-foreground leading-relaxed max-w-[240px]">
          Spinning up a fresh environment. This takes 15-30 seconds on the first message.
        </p>
      </div>

      {/* Animated dots */}
      <div className="flex gap-1.5">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="size-1.5 rounded-full bg-primary/50 animate-pulse"
            style={{ animationDelay: `${i * 300}ms`, animationDuration: "1.2s" }}
          />
        ))}
      </div>
    </div>
  );
}
