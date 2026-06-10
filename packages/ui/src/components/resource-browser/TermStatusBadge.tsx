import { t } from "@neokapi/kapi-react/runtime";
interface TermStatusBadgeProps {
  status: string;
  className?: string;
}

/** Status style keyed by [bgHue, fgHue] pairs — lightness comes from CSS custom properties for dark mode. */
// `get` accessors defer the t() dictionary lookup to render time, so
// translations loaded after module evaluation still apply.
const STATUS_CONFIG: Record<string, { hue: number; label: string }> = {
  preferred: {
    hue: 160,
    get label() {
      return t("preferred", "term status");
    },
  },
  approved: {
    hue: 250,
    get label() {
      return t("approved", "term status");
    },
  },
  admitted: {
    hue: 260,
    get label() {
      return t("admitted", "term status");
    },
  },
  proposed: {
    hue: 85,
    get label() {
      return t("proposed", "term status");
    },
  },
  deprecated: {
    hue: 85,
    get label() {
      return t("deprecated", "term status");
    },
  },
  forbidden: {
    hue: 27,
    get label() {
      return t("forbidden", "term status");
    },
  },
};

/**
 * Semantic status badge for term lifecycle status.
 * preferred = emerald, approved = blue, deprecated = amber, forbidden = red.
 * Uses CSS custom properties for dark mode lightness adjustment.
 */
export function TermStatusBadge({ status, className }: TermStatusBadgeProps) {
  const config = STATUS_CONFIG[status];
  const hue = config?.hue ?? 260;
  const label = config?.label ?? status;
  const isStrikethrough = status === "deprecated" || status === "forbidden";

  return (
    <span
      className={`inline-flex items-center px-1.5 py-px rounded text-[10px] font-medium ${isStrikethrough ? "line-through" : ""} ${className ?? ""}`}
      style={{
        backgroundColor: `oklch(var(--badge-bg-l, 0.92) 0.04 ${hue})`,
        color: `oklch(var(--badge-fg-l, 0.4) 0.12 ${hue})`,
      }}
    >
      {label}
    </span>
  );
}
