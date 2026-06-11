import type { ToolDocParam } from "./types";

/**
 * Presentation props shared by every leaf field renderer and forwarded to
 * FieldWrapper: label/description chrome, preset-modified marker, parameter
 * docs, layout orientation, disabled state, and error display.
 */
export interface FieldChromeProps {
  label: string;
  description?: string;
  compact?: boolean;
  isModified?: boolean;
  docParam?: ToolDocParam;
  vertical?: boolean;
  disabled?: boolean;
  error?: string;
}
