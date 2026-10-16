// The rules files kapi writes for a project, each with its section as
// written. An assistant reads AGENTS.md or CLAUDE.md as it starts, without a
// tool call, so this is the part of the context it always has.
//
// A file kapi has not written yet is marked, and Write them now writes every
// missing one, the way `kapi context sync --files-only` does. Until one is
// written, the empty state says where the rules go.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import { ChevronRight, FileText } from "lucide-react";
import {
  Badge,
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  EmptyState,
  ErrorNotice,
  LoadingSpinner,
} from "@neokapi/ui-primitives";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type { RulesFileView, RulesFilesResult } from "../types/api";

export interface RulesFilesViewProps {
  tabID: string;
  /** Pre-loaded files for Storybook and tests, which reach no backend. */
  rulesFiles?: RulesFileView[];
}

export function RulesFilesView({ tabID, rulesFiles: given }: RulesFilesViewProps) {
  const qc = useQueryClient();
  const key = qk.rulesFiles(tabID);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.rulesFiles(tabID),
    enabled: !given,
    placeholderData: (previous) => previous,
  });
  // A rule kept anywhere changes the section kapi would write.
  useInvalidateOnEvent("workspace:changed", [key]);

  const write = useMutation({
    mutationFn: () => api.writeRulesFiles(tabID),
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });

  const files = given ?? query.data ?? null;
  const missing = (files ?? []).some((f) => !f.exists);
  // Nothing written yet: the empty state carries the one action.
  const noneWritten = !!files && files.length > 0 && files.every((f) => !f.exists);
  const writeButton = (
    <Button
      size="sm"
      variant="outline"
      data-slot="rules-files-write"
      disabled={write.isPending}
      onClick={() => write.mutate()}
    >
      {write.isPending && <LoadingSpinner size="sm" />}
      Write them now
    </Button>
  );

  return (
    <div data-slot="rules-files">
      {query.error && !given ? (
        <ErrorNotice error={query.error} title="The rules files could not be read" />
      ) : !files ? (
        query.isLoading && <LoadingSpinner />
      ) : files.length === 0 ? (
        <div data-slot="rules-files-none">
          <EmptyState
            icon={<FileText size={20} />}
            title="No rules files"
            description="The project's context holds no rules yet, so kapi writes no file."
          />
        </div>
      ) : (
        <>
          {noneWritten && (
            <div data-slot="rules-files-empty" className="mb-3">
              <EmptyState
                icon={<FileText size={20} />}
                title="No rules file written yet"
                description={rulesFilesHelp()}
                action={writeButton}
              />
            </div>
          )}
          <ul className="divide-y divide-border/60 border-y border-border/60">
            {files.map((f) => (
              <RulesFileRow key={f.path} file={f} />
            ))}
          </ul>
          {missing && !noneWritten && <div className="mt-3">{writeButton}</div>}
        </>
      )}

      {write.data && <WroteRulesFiles result={write.data} />}
      {write.error && (
        <ErrorNotice
          error={write.error}
          title="The rules files could not be written"
          className="mt-2"
        />
      )}
    </div>
  );
}

/** Where kapi writes the rules: the empty state's text and the tab's tooltip. */
export function rulesFilesHelp(): string {
  return t(
    "kapi writes the rules into AGENTS.md and CLAUDE.md, which an assistant reads as it starts.",
  );
}

/** One file: its path, whether kapi has written it, and its section. */
function RulesFileRow({ file }: { file: RulesFileView }) {
  return (
    <li data-slot="rules-file" data-path={file.path} data-exists={file.exists || undefined}>
      <Collapsible>
        <CollapsibleTrigger className="flex w-full items-center gap-2 py-2 text-left text-sm [&[data-state=open]>svg]:rotate-90">
          <ChevronRight size={13} className="shrink-0 text-muted-foreground transition-transform" />
          <code className="font-mono text-xs" translate="no">
            {file.path}
          </code>
          {!file.exists && (
            <Badge
              variant="outline"
              data-slot="rules-file-not-written"
              className="text-[11px] text-muted-foreground"
            >
              not written yet
            </Badge>
          )}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <pre
            data-slot="rules-file-section"
            translate="no"
            className="mb-3 max-h-80 overflow-auto whitespace-pre-wrap rounded-md bg-muted/50 p-3 font-mono text-[11px] leading-relaxed"
          >
            {file.section}
          </pre>
        </CollapsibleContent>
      </Collapsible>
    </li>
  );
}

/** What writing the rules files did, one file per line. */
function WroteRulesFiles({ result }: { result: RulesFilesResult }) {
  return (
    <ul
      data-slot="rules-files-wrote"
      className="mt-2 space-y-0.5 text-[11px] text-muted-foreground"
    >
      {result.files.map((f) => (
        <li key={f.path}>
          <code className="font-mono" translate="no">
            {f.path}
          </code>{" "}
          · <RulesFileAction action={f.action} />
        </li>
      ))}
    </ul>
  );
}

/** What happened to one rules file, in words. */
function RulesFileAction({ action }: { action: string }) {
  switch (action) {
    case "created":
      return <>created</>;
    case "updated":
      return <>updated</>;
    case "unchanged":
      return <>unchanged</>;
    case "removed":
      return <>section removed</>;
  }
  return <span translate="no">{action}</span>;
}
