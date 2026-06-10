/**
 * Form layout primitives — composable building blocks for form UIs.
 *
 * Follows the shadcn naming convention (FormItem, FormLabel, etc.) but does NOT
 * require react-hook-form. These are pure layout/presentation components that can
 * be composed freely by SchemaForm, custom forms, or any config editor.
 *
 * Usage:
 *   <FormItem>
 *     <FormLabel>Field Name</FormLabel>
 *     <FormDescription>Help text</FormDescription>
 *     <FormControl><Input /></FormControl>
 *     <FormMessage>Error message</FormMessage>
 *   </FormItem>
 *
 *   <FormToggle checked={v} onCheckedChange={set} label="Enable" description="..." />
 *
 *   <FormInputAction>
 *     <Input />
 *     <Button>Browse</Button>
 *   </FormInputAction>
 *
 *   <FormFieldGroup label="Section" collapsible defaultCollapsed={false}>
 *     {children}
 *   </FormFieldGroup>
 */

import * as React from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { cn } from "../../lib/utils";
import { Label } from "./label";
import { Switch } from "./switch";

// ── FormItem ─────────────────────────────────────────────────────────
// Vertical stack container for a single form field.

function FormItem({
  className,
  disabled,
  modified,
  ...props
}: React.ComponentProps<"div"> & { disabled?: boolean; modified?: boolean }) {
  return (
    <div
      data-slot="form-item"
      className={cn(
        "space-y-1 pl-2.5 border-l-2",
        modified ? "border-primary" : "border-transparent",
        disabled && "opacity-50",
        className,
      )}
      {...props}
    />
  );
}

// ── FormLabel ────────────────────────────────────────────────────────
// Field label.

function FormLabel({
  className,
  disabled,
  children,
  ...props
}: React.ComponentProps<typeof Label> & {
  disabled?: boolean;
}) {
  return (
    <Label
      data-slot="form-label"
      className={cn("text-xs font-medium", disabled && "text-muted-foreground", className)}
      {...props}
    >
      {children}
    </Label>
  );
}

// ── FormDescription ──────────────────────────────────────────────────
// Help text shown below the label.

function FormDescription({ className, ...props }: React.ComponentProps<"p">) {
  return (
    <p
      data-slot="form-description"
      className={cn("text-xs text-muted-foreground", className)}
      {...props}
    />
  );
}

// ── FormMessage ──────────────────────────────────────────────────────
// Validation error message shown below the control.

function FormMessage({ className, children, ...props }: React.ComponentProps<"p">) {
  if (!children) return null;
  return (
    <p data-slot="form-message" className={cn("text-xs text-destructive", className)} {...props}>
      {children}
    </p>
  );
}

// ── FormControl ──────────────────────────────────────────────────────
// Slot wrapper for the actual input/control element.

function FormControl({
  className,
  vertical,
  ...props
}: React.ComponentProps<"div"> & { vertical?: boolean }) {
  return (
    <div
      data-slot="form-control"
      className={cn(vertical && "flex flex-col gap-1", className)}
      {...props}
    />
  );
}

// ── FormToggle ───────────────────────────────────────────────────────
// Boolean toggle: Switch + inline label + optional description.
// Common pattern for checkbox/toggle fields.

function FormToggle({
  checked,
  onCheckedChange,
  label,
  description,
  modified,
  disabled,
  compact,
  className,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  label: string;
  description?: string;
  modified?: boolean;
  disabled?: boolean;
  compact?: boolean;
  className?: string;
}) {
  return (
    <label
      data-slot="form-toggle"
      className={cn(
        "flex items-center gap-2 py-0.5 pl-2.5 border-l-2 cursor-pointer",
        modified ? "border-primary" : "border-transparent",
        disabled && "opacity-50 cursor-not-allowed",
        className,
      )}
    >
      <Switch
        size="sm"
        checked={checked}
        disabled={disabled}
        onCheckedChange={(v: boolean) => !disabled && onCheckedChange(v)}
      />
      <div className="flex-1">
        <span className="text-xs font-medium">{label}</span>
        {!compact && description && <p className="text-xs text-muted-foreground">{description}</p>}
      </div>
    </label>
  );
}

// ── FormInputAction ──────────────────────────────────────────────────
// Horizontal input + action button layout (e.g., path input + Browse).

function FormInputAction({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="form-input-action" className={cn("flex gap-1.5", className)} {...props} />;
}

// ── FormFieldGroup ───────────────────────────────────────────────────
// Collapsible section header + content container for grouping fields.

function FormFieldGroup({
  label,
  description,
  collapsible = false,
  defaultCollapsed = false,
  className,
  children,
}: {
  label: string;
  description?: string;
  collapsible?: boolean;
  defaultCollapsed?: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  return (
    <div data-slot="form-field-group" className={className}>
      {!collapsible ? (
        <div className="pb-1.5 mb-2 border-b border-border/40">
          <span className="text-xs font-semibold text-muted-foreground tracking-wide">{label}</span>
          {description && <p className="text-xs text-muted-foreground/70 mt-0.5">{description}</p>}
        </div>
      ) : (
        <>
          <button
            type="button"
            onClick={() => setCollapsed(!collapsed)}
            className={cn(
              "flex items-center gap-1 w-full text-left pb-1.5 border-b border-border/40",
              !collapsed && "mb-0.5",
            )}
          >
            {collapsed ? (
              <ChevronRight className="size-3 text-muted-foreground" />
            ) : (
              <ChevronDown className="size-3 text-muted-foreground" />
            )}
            <span className="text-xs font-semibold text-muted-foreground tracking-wide">
              {label}
            </span>
          </button>
          {!collapsed && description && (
            <p className="text-xs text-muted-foreground/70 mb-2">{description}</p>
          )}
        </>
      )}
      {!collapsed && children}
    </div>
  );
}

// ── FormHelpText ─────────────────────────────────────────────────────
// Expandable documentation/help section for a field.

function FormHelpText({
  description,
  notes,
  dependencies,
  className,
}: {
  description?: string;
  notes?: string[];
  dependencies?: Array<{ property: string; condition: string }>;
  className?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  if (!description) return null;

  const hasExtra = (notes && notes.length > 0) || (dependencies && dependencies.length > 0);

  return (
    <div data-slot="form-help-text" className={cn("text-xs text-muted-foreground mt-1", className)}>
      <p className="line-clamp-2">{description}</p>
      {hasExtra && (
        <>
          <button
            type="button"
            className="block ml-auto text-xs text-primary hover:underline mt-0.5"
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? t("Show less") : t("Show more")}
          </button>
          {expanded && (
            <div className="mt-1 space-y-1">
              {notes?.map((note, i) => (
                <p key={i} className="text-xs text-muted-foreground italic">
                  {note}
                </p>
              ))}
              {dependencies?.map((dep, i) => (
                <p key={i} className="text-xs text-muted-foreground">
                  Depends on: <code className="text-xs">{dep.property}</code> ({dep.condition})
                </p>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

export {
  FormItem,
  FormLabel,
  FormDescription,
  FormMessage,
  FormControl,
  FormToggle,
  FormInputAction,
  FormFieldGroup,
  FormHelpText,
};
