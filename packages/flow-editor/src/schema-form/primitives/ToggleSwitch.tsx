import { theme } from "../utils";

export function ToggleSwitch({ checked, onToggle }: { checked: boolean; onToggle: (v: boolean) => void }) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onToggle(!checked)}
      style={{
        width: 28,
        height: 16,
        borderRadius: 8,
        border: "none",
        cursor: "pointer",
        position: "relative",
        flexShrink: 0,
        background: checked ? theme.accent : theme.bgMuted,
        transition: "background 150ms",
      }}
    >
      <div
        style={{
          width: 12,
          height: 12,
          borderRadius: 6,
          background: theme.primaryFg,
          position: "absolute",
          top: 2,
          left: checked ? 14 : 2,
          transition: "left 150ms",
        }}
      />
    </button>
  );
}
