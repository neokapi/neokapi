import * as React from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { Languages, Package, Palette, Radio, Tag, X } from "lucide-react";

import { cn } from "../../lib/utils";

/**
 * CoordinateChip: one axis of a context coordinate, drawn as a chip.
 *
 * Content sits at a point: a product, a channel, a brand, a language. Written
 * out as `channel:reference` the axis and the value compete for the reader's
 * attention and neither wins, and the same point reads differently on every
 * surface that spells it. A chip gives the axis a hue and an icon and puts the
 * value in words, so a row of them scans as an address rather than as a string.
 *
 * The axis name is the accessible name and the tooltip ("Channel: reference"),
 * because the hue is the shorthand and a shorthand needs somewhere to be spelt
 * out. The value is rendered exactly as it was given: a coordinate value is an
 * identifier, and casing is part of it.
 */

/** The axes a coordinate is built from. */
export const AXIS_IDS = ["product", "channel", "brand", "language"] as const;

export type AxisId = (typeof AXIS_IDS)[number];

/** What an adopter needs to draw an axis without re-declaring any of it. */
export interface AxisMeta {
  /** The axis id as a recipe and the context graph spell it. */
  id: string;
  /** Icon for the chip, and for any list that wants the same mark. */
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  /** Human-readable axis name in the active UI language. */
  readonly label: string;
  /** The CSS custom property holding this axis's tint. */
  token: string;
  /** Tailwind tint and ink for the chip. Spelt out so the scanner sees them. */
  className: string;
}

/**
 * The axis vocabulary: icon, name and colour for every axis the product knows.
 *
 * `label` is a getter so the name is looked up at render time, which is what
 * lets a locale switch repaint a chip that was mounted before the dictionary
 * arrived.
 */
export const AXES: Record<AxisId, AxisMeta> = {
  product: {
    id: "product",
    icon: Package,
    get label() {
      return t("Product", "context axis");
    },
    token: "--axis-product",
    className: "bg-axis-product text-axis-product-foreground",
  },
  channel: {
    id: "channel",
    icon: Radio,
    get label() {
      return t("Channel", "context axis");
    },
    token: "--axis-channel",
    className: "bg-axis-channel text-axis-channel-foreground",
  },
  brand: {
    id: "brand",
    icon: Palette,
    get label() {
      return t("Brand", "context axis");
    },
    token: "--axis-brand",
    className: "bg-axis-brand text-axis-brand-foreground",
  },
  language: {
    id: "language",
    icon: Languages,
    get label() {
      return t("Language", "context axis");
    },
    token: "--axis-language",
    className: "bg-axis-language text-axis-language-foreground",
  },
};

/**
 * The fallback for an axis nobody has named. A recipe may declare any axis it
 * likes, so an unknown one is ordinary rather than an error: it renders neutral
 * under a generic mark, keeping its own id as the name.
 */
export function unknownAxis(axis: string): AxisMeta {
  return {
    id: axis,
    icon: Tag,
    label: axis,
    token: "--axis-unknown",
    className: "bg-axis-unknown text-axis-unknown-foreground",
  };
}

/** Resolve an axis id to its vocabulary entry, or to the neutral fallback. */
export function axisMeta(axis: string): AxisMeta {
  return (AXES as Record<string, AxisMeta>)[axis] ?? unknownAxis(axis);
}

export interface CoordinateChipProps extends Omit<React.ComponentProps<"span">, "onSelect"> {
  /** Axis id, e.g. `channel`. An id outside AXES renders neutral. */
  axis: string;
  /** The coordinate value, rendered as given. */
  value: string;
  /** `sm` (default) matches the Badge height; `md` suits a heading or a form. */
  size?: "sm" | "md";
  /** Show a remove control and call this when it is used. */
  onRemove?: () => void;
  /** Override the axis name in the tooltip and the accessible name. */
  label?: string;
  /** Accessible name for the remove control. */
  removeLabel?: string;
}

/** One axis of a coordinate: an icon, a hue, and the value. */
export function CoordinateChip({
  axis,
  value,
  size = "sm",
  onRemove,
  label,
  removeLabel,
  className,
  ...props
}: CoordinateChipProps) {
  const meta = axisMeta(axis);
  const Icon = meta.icon;
  const axisName = label ?? meta.label;
  const description = t("{axis}: {value}", { axis: axisName, value });

  return (
    <span
      data-slot="coordinate-chip"
      data-axis={axis}
      // A chip with no control is a compound graphic, and `img` lets its label
      // stand for the icon and the value together. A chip that can be removed
      // has to keep that button reachable, so it is a group instead.
      role={onRemove ? "group" : "img"}
      aria-label={description}
      title={description}
      className={cn(
        "inline-flex w-fit max-w-full shrink-0 items-center gap-1 overflow-hidden rounded-4xl font-medium whitespace-nowrap",
        size === "sm" ? "h-5 px-2 text-xs" : "h-6 px-2.5 text-[0.8rem]",
        meta.className,
        className,
      )}
      {...props}
    >
      <Icon aria-hidden className={size === "sm" ? "size-3 shrink-0" : "size-3.5 shrink-0"} />
      <span className="truncate">{value}</span>
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          aria-label={removeLabel ?? t("Remove {axis}: {value}", { axis: axisName, value })}
          className="-mr-1 shrink-0 cursor-pointer rounded-full p-0.5 opacity-70 transition-opacity hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-current"
        >
          <X aria-hidden className="size-3" />
        </button>
      )}
    </span>
  );
}
