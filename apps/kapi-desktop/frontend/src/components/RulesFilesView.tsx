// The rules files kapi writes for a project, each with its section as
// written. An assistant reads AGENTS.md or CLAUDE.md as it starts, without a
// tool call, so this is the part of the context it always has.
//
// A file kapi has not written yet is marked, and Write them now writes every
// missing one, the way `kapi context sync --files-only` does.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight } from "lucide-react";
import {
  Badge,
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
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

  return (
    <div data-slot="rules-files">
      <p className="text-sm text-muted-foreground">
        kapi writes the rules into <code translate="no">AGENTS.md</code> and{" "}
        <code translate="no">CLAUDE.md</code>, which an assistant reads as it starts.
      </p>

      {query.error && !given ? (
        <ErrorNotice
          error={query.error}
          title="The rules files could not be read"
          className="mt-3"
        />
      ) : !files ? (
        query.isLoading && <LoadingSpinner className="mt-3" />
      ) : files.length === 0 ? (
        <p data-slot="rules-files-none" className="mt-3 text-sm text-muted-foreground">
          No rules files: the project&apos;s context holds no rules yet.
        </p>
      ) : (
        <ul className="mt-3 divide-y divide-border/60 border-y border-border/60">
          {files.map((f) => (
            <RulesFileRow key={f.path} file={f} />
          ))}
        </ul>
      )}

      {missing && (
        <div className="mt-3 flex items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            data-slot="rules-files-write"
            disabled={write.isPending}
            onClick={() => write.mutate()}
          >
            Write them now
          </Button>
          {write.isPending && <LoadingSpinner size="sm" />}
        </div>
      )}
      {write.data && <WroteRulesFiles result={write.data} />}
      {write.error && (
        <ErrorNotice
          error={write.error}
          title="The rules files could not be written"
          className="mt-2"
        />
      )}

      <p className="mt-3 text-[11px] text-muted-foreground">
        The same as <code translate="no">kapi context sync --files-only</code>.
      </p>
    </div>
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
