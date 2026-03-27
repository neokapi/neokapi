import type { Workspace } from "../types/api";

interface WorkspaceIconProps {
  workspace: Workspace;
  active: boolean;
  onClick: () => void;
  size?: number;
}

const COLORS = [
  "#4A90D9",
  "#D94A4A",
  "#4AD9A7",
  "#D9A74A",
  "#9B59B6",
  "#E67E22",
  "#1ABC9C",
  "#E74C3C",
];

function hashColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = ((hash << 5) - hash + name.charCodeAt(i)) | 0;
  }
  return COLORS[Math.abs(hash) % COLORS.length];
}

export function WorkspaceIcon({ workspace, active, onClick, size = 40 }: WorkspaceIconProps) {
  const letter = (workspace.name || "?")[0].toUpperCase();
  const bg = workspace.logo_url ? undefined : hashColor(workspace.name);
  const isPersonal = workspace.type === "personal";
  const badgeSize = Math.max(Math.round(size * 0.4), 12);

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") onClick();
      }}
      title={workspace.name}
      style={{
        position: "relative",
        width: size,
        height: size,
      }}
    >
      <div
        aria-hidden="true"
        style={{
          width: size,
          height: size,
          borderRadius: active ? 12 : size / 2,
          border: "none",
          cursor: "pointer",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          fontSize: size * 0.45,
          fontWeight: 700,
          color: "#fff",
          backgroundColor: bg || "transparent",
          backgroundImage: workspace.logo_url ? `url(${workspace.logo_url})` : undefined,
          backgroundSize: "cover",
          transition: "border-radius 0.2s ease",
          outline: active ? "2px solid var(--accent, #4A90D9)" : "none",
          outlineOffset: 2,
        }}
      >
        {!workspace.logo_url && letter}
      </div>
      {isPersonal && (
        <span
          data-testid="personal-indicator"
          style={{
            position: "absolute",
            bottom: -2,
            right: -2,
            width: badgeSize,
            height: badgeSize,
            borderRadius: "50%",
            backgroundColor: "var(--background, #fff)",
            border: "1.5px solid var(--border, #e5e7eb)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontSize: Math.max(Math.round(size * 0.28), 9),
            fontWeight: 700,
            lineHeight: 1,
            color: "var(--foreground, #111)",
          }}
        >
          @
        </span>
      )}
    </div>
  );
}
