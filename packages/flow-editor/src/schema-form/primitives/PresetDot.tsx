import { theme } from "../utils";

/** Small colored dot shown before a field label when the value differs from the active preset. */
export function PresetDot({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <span
      style={{
        display: "inline-block",
        width: 5,
        height: 5,
        borderRadius: "50%",
        background: theme.accent,
        marginRight: 4,
        flexShrink: 0,
        verticalAlign: "middle",
      }}
      title="Modified from preset"
    />
  );
}
