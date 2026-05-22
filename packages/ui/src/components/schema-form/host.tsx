// Host-injection context for the shared schema-form renderer.
//
// The form runs in multiple hosts with different capabilities:
//   - kapi-desktop (Wails): native file/folder dialogs, a credential vault.
//   - the docs website / Storybook: no filesystem, no credential store.
//
// Widgets that need host capabilities (file/folder pickers, credential
// pickers) read them from this context. When a host does not provide a
// capability the widget degrades gracefully (a labelled text input, no
// broken button) rather than assuming a browser/filesystem.

import { createContext, useContext } from "react";

/** A single file-dialog filter, mirroring the framework's FileFilter. */
export interface SchemaFormFileFilter {
  name: string;
  /** Space-delimited glob list, e.g. "*.tmx" or "*.html *.htm". */
  extensions: string;
}

/** Describes a browse request handed to the host's file/folder dialog. */
export interface SchemaFormBrowseRequest {
  /** "file" opens a file dialog; "directory" opens a folder dialog. */
  kind: "file" | "directory";
  /** Property name driving the request (useful for host logging/routing). */
  field: string;
  /** Current value of the field, so the host can seed the dialog. */
  currentValue?: string;
  /** Dialog title, when the schema supplies one. */
  title?: string;
  /** When true, the host should open a save-as dialog. */
  forSaveAs?: boolean;
  /** File-extension filters (file dialogs only). */
  filters?: SchemaFormFileFilter[];
  /** Bare extension hints (e.g. ["html", "txt"]) from x-path.accepts. */
  accepts?: string[];
}

/** A selectable credential, as surfaced to the credential-picker widget. */
export interface SchemaFormCredential {
  /** Stable identifier persisted into the form value. */
  value: string;
  /** Human-readable label shown in the picker. */
  label: string;
}

export interface SchemaFormHost {
  /**
   * Opens a host-native file/folder dialog and resolves to the chosen path,
   * or null if the user cancelled. When omitted, file/folder widgets render a
   * plain labelled text input with no Browse button.
   */
  onBrowse?: (request: SchemaFormBrowseRequest) => Promise<string | null>;
  /**
   * Returns the credentials available for a credential-picker field. The
   * optional resourceKind lets a host scope the list (e.g. only AI providers).
   * When omitted (and the schema carries no inline options), the
   * credential-picker degrades to a text input.
   */
  credentials?: (resourceKind?: string) => SchemaFormCredential[];
}

const SchemaFormHostContext = createContext<SchemaFormHost>({});

export function SchemaFormHostProvider({
  host,
  children,
}: {
  host: SchemaFormHost | undefined;
  children: React.ReactNode;
}) {
  return (
    <SchemaFormHostContext.Provider value={host ?? {}}>{children}</SchemaFormHostContext.Provider>
  );
}

export function useSchemaFormHost(): SchemaFormHost {
  return useContext(SchemaFormHostContext);
}
