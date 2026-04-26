export interface FormatDoc {
  id: string;
  displayName: string;
  description: string;
  extensions: string[];
  mimeTypes: string[];
  hasReader: boolean;
  hasWriter: boolean;
  groups?: GroupDoc[];
  properties?: Record<string, PropDoc>;
  presets?: PresetDoc[];
}

export interface GroupDoc {
  id: string;
  label: string;
  description?: string;
  collapsed?: boolean;
  fields: string[];
}

export interface PropDoc {
  type: string;
  description: string;
  default?: any;
  widget?: string;
}

export interface PresetDoc {
  id: string;
  name: string;
  description: string;
  parameters: Record<string, any>;
}

export interface FormatsData {
  formats: FormatDoc[];
}
