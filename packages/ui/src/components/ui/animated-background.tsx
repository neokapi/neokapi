import { cn } from "../../lib/utils";
import { useTheme } from "../../context/ThemeContext";

export interface AnimatedBackgroundProps {
  readonly showCenterOrb?: boolean;
  readonly className?: string;
}

/**
 * Animated gradient background with floating orbs for glassmorphism UIs.
 * Uses CSS variables from shadcn-glass-ui themes (--bg-from, --bg-via, --bg-to, --orb-1..5).
 */
export function AnimatedBackgroundGlass({ className, showCenterOrb }: AnimatedBackgroundProps) {
  const { theme } = useTheme();
  const shouldShowCenterOrb = showCenterOrb ?? theme === "glass";

  return (
    <div
      className={cn("fixed inset-0 transition-all duration-500 overflow-hidden", className)}
      style={{ background: "linear-gradient(135deg, var(--bg-from), var(--bg-via), var(--bg-to))" }}
      aria-hidden="true"
    >
      <div
        className="absolute top-20 -left-20 w-[600px] h-[600px] rounded-full animate-orb-float"
        style={{ background: "var(--orb-1)", filter: "blur(80px)" }}
      />
      <div
        className="absolute -bottom-20 -right-20 w-[700px] h-[700px] rounded-full animate-orb-float"
        style={{ background: "var(--orb-2)", filter: "blur(100px)", animationDelay: "2s" }}
      />
      <div
        className="absolute top-1/3 -right-10 w-[500px] h-[500px] rounded-full animate-orb-float"
        style={{ background: "var(--orb-3)", filter: "blur(70px)", animationDelay: "4s" }}
      />
      <div
        className="absolute -bottom-10 left-1/4 w-[450px] h-[450px] rounded-full animate-orb-float"
        style={{ background: "var(--orb-4)", filter: "blur(60px)", animationDelay: "6s" }}
      />
      {shouldShowCenterOrb && (
        <div
          className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[500px] h-[500px] rounded-full"
          style={{ background: "var(--orb-5)", filter: "blur(80px)" }}
        />
      )}
    </div>
  );
}
