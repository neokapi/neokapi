// Fixed Source/Sink endpoint pickers (AD-026).
//
// A flow is composition only and owns no I/O — reader/writer are no longer
// draggable graph nodes. Where content enters (source) and leaves (sink) is a
// *binding*, surfaced here as two fixed terminal controls pinned to the ends of
// the canvas. Each is a dropdown offering the binding kinds: file · store ·
// interchange · none. They are deliberately styled as anchored, dashed-outline
// terminals so they never read as one of the solid, draggable tool nodes.

import {
  FileInput,
  FileOutput,
  Database,
  ArrowLeftRight,
  CircleSlash,
  Pin,
  ChevronDown,
  type LucideIcon,
} from "lucide-react";
import {
  cn,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { FlowBinding, FlowBindingKind } from "../types";

type Role = "source" | "sink";

interface BindingKindMeta {
  kind: FlowBindingKind;
  label: string;
  icon: LucideIcon;
  description: string;
}

/** Binding kinds offered by the source picker. */
const SOURCE_KINDS: BindingKindMeta[] = [
  {
    kind: "file",
    get label() {
      return t("Files", "flow binding kind");
    },
    icon: FileInput,
    get description() {
      return t("Read the project's source files");
    },
  },
  {
    kind: "store",
    get label() {
      return t("Content store", "flow binding kind");
    },
    icon: Database,
    get description() {
      return t("Read from the content store");
    },
  },
  {
    kind: "interchange",
    get label() {
      return t("Interchange", "flow binding kind");
    },
    icon: ArrowLeftRight,
    get description() {
      return t("Read a bilingual interchange file (XLIFF, PO, …)");
    },
  },
  {
    kind: "none",
    get label() {
      return t("None", "flow binding kind");
    },
    icon: CircleSlash,
    get description() {
      return t("No input binding");
    },
  },
];

/** Binding kinds offered by the sink picker. */
const SINK_KINDS: BindingKindMeta[] = [
  {
    kind: "file",
    get label() {
      return t("Files", "flow binding kind");
    },
    icon: FileOutput,
    get description() {
      return t("Write back to the project's files");
    },
  },
  {
    kind: "store",
    get label() {
      return t("Content store", "flow binding kind");
    },
    icon: Database,
    get description() {
      return t("Write to the content store");
    },
  },
  {
    kind: "interchange",
    get label() {
      return t("Interchange", "flow binding kind");
    },
    icon: ArrowLeftRight,
    get description() {
      return t("Write a bilingual interchange file (XLIFF, PO, …)");
    },
  },
  {
    kind: "none",
    get label() {
      return t("None", "flow binding kind");
    },
    icon: CircleSlash,
    get description() {
      return t("No output binding");
    },
  },
];

const ROLE_META: Record<Role, { typeLabel: string; defaultIcon: LucideIcon; accent: string }> = {
  // Reader green / writer amber, matching the legacy terminal nodes.
  source: {
    get typeLabel() {
      return t("Source", "flow endpoint");
    },
    defaultIcon: FileInput,
    accent: "oklch(0.7 0.17 145)",
  },
  sink: {
    get typeLabel() {
      return t("Sink", "flow endpoint");
    },
    defaultIcon: FileOutput,
    accent: "oklch(0.7 0.13 85)",
  },
};

/** The effective binding when one is omitted: `file` is the default (AD-026). */
const DEFAULT_BINDING: FlowBinding = { kind: "file" };

function metaFor(role: Role, kind: FlowBindingKind): BindingKindMeta {
  const list = role === "source" ? SOURCE_KINDS : SINK_KINDS;
  return list.find((m) => m.kind === kind) ?? list[0];
}

interface EndpointPickerProps {
  role: Role;
  binding?: FlowBinding;
  /** Called with the new binding when the user picks a kind. */
  onChange?: (binding: FlowBinding) => void;
  /** Read-only flows (built-in) show the binding but disable editing. */
  readOnly?: boolean;
}

/**
 * One fixed endpoint terminal — rendered by FlowEditor at the start (source) or
 * end (sink) of the tool chain. Not a draggable node and not part of the
 * deletable node set.
 */
export function EndpointPicker({ role, binding, onChange, readOnly }: EndpointPickerProps) {
  const meta = ROLE_META[role];
  const effective = binding ?? DEFAULT_BINDING;
  const kindMeta = metaFor(role, effective.kind);
  const KindIcon = kindMeta.icon;

  const detail =
    effective.kind === "interchange" && effective.format
      ? effective.format.toUpperCase()
      : kindMeta.label;

  const kinds = role === "source" ? SOURCE_KINDS : SINK_KINDS;

  const handleSelect = (value: string) => {
    if (readOnly || !onChange) return;
    const kind = value as FlowBindingKind;
    // Preserve the chosen interchange format when toggling within interchange;
    // otherwise drop the format field.
    onChange(kind === "interchange" ? { kind, format: effective.format } : { kind });
  };

  const trigger = (
    <button
      type="button"
      disabled={readOnly}
      aria-label={`${meta.typeLabel}: ${detail}`}
      className={cn(
        // Fixed h-12 (48px) so the handle center sits at a known offset; the
        // serpentine layout nudges the endpoint down by (toolHeight-48)/2 so its
        // connector to the tool row is perfectly horizontal.
        "group flex h-12 min-w-[150px] items-center gap-2 rounded-full border border-dashed bg-card/80 px-3",
        "shadow-[0_1px_4px_oklch(0_0_0/0.15)] backdrop-blur-sm transition-colors duration-150",
        !readOnly && "cursor-pointer hover:bg-card",
        readOnly && "cursor-default opacity-90",
      )}
      style={{ borderColor: meta.accent }}
    >
      {/* Pin marker — signals "fixed terminal", not a movable node. */}
      <span
        className="flex size-5 shrink-0 items-center justify-center rounded-full"
        style={{ background: `color-mix(in oklch, ${meta.accent} 18%, transparent)` }}
      >
        <Pin size={11} style={{ color: meta.accent }} />
      </span>

      <span className="flex flex-col items-start leading-none">
        <span
          className="text-[8px] font-bold uppercase tracking-[0.12em]"
          style={{ color: meta.accent }}
        >
          {meta.typeLabel}
        </span>
        <span className="mt-0.5 flex items-center gap-1 text-[12px] font-semibold text-foreground">
          <KindIcon size={11} className="text-muted-foreground" />
          {detail}
        </span>
      </span>

      {!readOnly && (
        <ChevronDown
          size={12}
          className="ml-auto text-muted-foreground transition-transform duration-150 group-data-[state=open]:rotate-180"
        />
      )}
    </button>
  );

  if (readOnly || !onChange) {
    return trigger;
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        side={role === "source" ? "bottom" : "top"}
        className="w-60"
      >
        <DropdownMenuLabel className="text-[10px] uppercase tracking-wide text-muted-foreground">
          {role === "source" ? t("Where content enters") : t("Where content leaves")}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuRadioGroup value={effective.kind} onValueChange={handleSelect}>
          {kinds.map((k) => {
            const Icon = k.icon;
            return (
              <DropdownMenuRadioItem key={k.kind} value={k.kind} className="items-start py-1.5">
                <span className="flex flex-col gap-0.5">
                  <span className="flex items-center gap-1.5 text-[12px] font-medium">
                    <Icon size={12} className="text-muted-foreground" />
                    {k.label}
                  </span>
                  <span className="pl-[18px] text-[10px] leading-tight text-muted-foreground">
                    {k.description}
                  </span>
                </span>
              </DropdownMenuRadioItem>
            );
          })}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** The Source endpoint terminal — pinned at the start of the tool chain. */
export function SourcePicker(props: Omit<EndpointPickerProps, "role">) {
  return <EndpointPicker role="source" {...props} />;
}

/** The Sink endpoint terminal — pinned at the end of the tool chain. */
export function SinkPicker(props: Omit<EndpointPickerProps, "role">) {
  return <EndpointPicker role="sink" {...props} />;
}
