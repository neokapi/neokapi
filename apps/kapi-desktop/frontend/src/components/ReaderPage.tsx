// The Reader on one content file, drawn by @neokapi/project-screens'
// ReaderView. This page reads the file through the desktop backend: the
// blocks and findings of a checks run over this file alone, and the context
// answer for it. A fix is the finding's own change operation, which the
// backend applies through the change service as the person at the keyboard;
// the page then draws the file as the service's result left it.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { Button, ErrorNotice, LoadingSpinner, ScrollArea } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { ReaderView } from "@neokapi/project-screens";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { desktopView } from "../lib/screenTargets";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import type { ReaderFile, ReaderRefusal } from "../types/api";

export interface ReaderPageProps {
  tabID: string;
  /** The file, project-relative (or absolute inside the project). */
  path: string;
  /** Scroll to this block once the file is drawn, and mark it. */
  focusBlockID?: string;
  /** Pre-loaded file for Storybook and tests, which reach no backend. */
  reader?: ReaderFile;
  /** Return to the page the Reader was opened from. */
  onBack: () => void;
  /** The back button's label, naming that page. */
  backLabel?: string;
  /** Go to another view of the project: "context:rules", … */
  onNavigate?: (view: string) => void;
}

export function ReaderPage({
  tabID,
  path,
  focusBlockID,
  reader: given,
  onBack,
  backLabel,
  onNavigate,
}: ReaderPageProps) {
  const qc = useQueryClient();
  const key = qk.readerFile(tabID, path);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.readerFile(tabID, path),
    enabled: !given,
    placeholderData: (previous) => previous,
  });
  // A rule kept anywhere, or the file changing on disk, changes the answer.
  useInvalidateOnEvent(["project-files-changed", "workspace:changed"], [key]);

  // A fix answers with the file read again, which replaces the page's read.
  // Its refusals are kept apart from the read: the file changing on disk
  // reads it again, and that read carries no refusal of its own.
  const [refused, setRefused] = useState<ReaderRefusal[] | null>(null);
  const settle = (next: ReaderFile | null) => {
    if (!next) return;
    qc.setQueryData(key, next);
    setRefused(next.refused);
  };
  const fixOne = useMutation({
    mutationFn: (findingID: string) => api.fixFinding(tabID, path, findingID),
    onSuccess: settle,
  });
  const fixAll = useMutation({
    mutationFn: () => api.fixFile(tabID, path),
    onSuccess: settle,
  });

  const reader = given ?? query.data ?? null;
  const busy = fixOne.isPending || fixAll.isPending;

  if (!reader) {
    return (
      <div className="p-6">
        {query.error ? (
          <div className="space-y-3">
            <ErrorNotice error={query.error} title={t("The file could not be read")} />
            <Button size="sm" variant="ghost" onClick={onBack}>
              <ArrowLeft size={14} />
              {backLabel ?? t("Back")}
            </Button>
          </div>
        ) : (
          <LoadingSpinner />
        )}
      </div>
    );
  }

  return (
    <ScrollArea className="h-full">
      <ReaderView
        reader={reader}
        refused={refused ?? reader.refused}
        focusBlockID={focusBlockID}
        busy={busy}
        fixingID={fixOne.isPending ? fixOne.variables : undefined}
        fixingAll={fixAll.isPending}
        fixError={fixOne.error ?? fixAll.error}
        onFix={(id) => fixOne.mutate(id)}
        onFixAll={() => fixAll.mutate()}
        onBack={onBack}
        backLabel={backLabel}
        navigate={(target) => onNavigate?.(desktopView(target))}
        canNavigate={() => !!onNavigate}
      />
    </ScrollArea>
  );
}
