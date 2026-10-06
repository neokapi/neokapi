// The versions of a .kpz's document that did not land, for every .kpz on this
// machine whose working cache holds edits, inside a project or in none.
//
// The workspace home shows them so a .kpz that sits outside every project is
// not left with nowhere to settle it. Each document is named by the .kpz's
// absolute path, and every choice goes through the change service of the
// project the .kpz sits in, or of its directory when it sits in none.

import type { ChangeResult } from "@neokapi/contract-types";

import { api } from "../hooks/useApi";
import type { ChangeClient } from "../lib/changes";
import { qk } from "../lib/queryKeys";
import type { DocumentRebase, KeptConflict } from "../types/api";
import { KeptConflicts, type ConflictSource } from "./KeptConflicts";

/** The change service of a .kpz's document named by its absolute path. */
const workspaceClient: ChangeClient = {
  read: async () => null,
  apply: async (set) => {
    const raw = await api.applyWorkspaceDocument(JSON.stringify(set));
    return raw ? (JSON.parse(raw) as ChangeResult) : null;
  },
  describe: async () => null,
  history: async () => null,
};

const workspaceSource: ConflictSource = {
  key: qk.workspaceDocumentConflicts(),
  load: () => api.getWorkspaceDocumentConflicts(),
};

export interface WorkspaceConflictsProps {
  /** Pre-loaded for Storybook and tests; read from the workspace otherwise. */
  conflicts?: KeptConflict[];
  /** The change service; the workspace home's when absent. */
  client?: ChangeClient;
  rebase?: (doc: string, edit: string) => Promise<DocumentRebase | null>;
  discard?: (doc: string, edit: string) => Promise<void>;
}

export function WorkspaceConflicts({
  conflicts,
  client,
  rebase,
  discard,
}: WorkspaceConflictsProps) {
  return (
    <KeptConflicts
      tabID=""
      conflicts={conflicts}
      source={workspaceSource}
      client={client ?? workspaceClient}
      rebase={rebase ?? ((doc, edit) => api.rebaseWorkspaceDocument(doc, edit))}
      discard={
        discard ??
        (async (doc, edit) => {
          await api.discardWorkspaceDocument(doc, edit);
        })
      }
      release={async () => {}}
    />
  );
}
