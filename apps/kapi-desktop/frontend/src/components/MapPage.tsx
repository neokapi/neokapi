// The Map: the page a project tab opens on. It is an overview (page-patterns
// "Overview"): the project's name and when kapi last read its files, four
// counts, the suggestions that wait for a person and the content files, each
// row leading to the place that acts on it. Nothing on it is edited in place.
//
// The page renders from one read, MapSummary, which the backend answers from
// what it already holds. It reads again when the store sync finishes, when the
// files change and when the context changes. Findings come from the last
// checks run over the whole project: the page runs one the first time it opens
// a tab that has none, and again when the person asks.

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Play, Sparkles } from "lucide-react";
import {
  Badge,
  Button,
  EmptyState,
  ErrorNotice,
  KindIcon,
  LoadingSpinner,
  PageHeader,
  ScrollArea,
  SectionHeading,
  SimpleTooltip,
  StatRow,
  When,
  digestKind,
  type StatItem,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useStoreSyncState } from "../hooks/useProjectStoreSync";
import { ConflictLine, RuleLine } from "./DigestRule";
import { FilePreview } from "./FilePreview";
import type { MapFileRow, MapSuggestionRow, MapSummary } from "../types/api";

/** The Decide view's key for a row: its group and its item's id. */
export function decideRowKey(row: MapSuggestionRow): string {
  return `${row.group}:${row.item.id}`;
}

/** The tabs the Map has run a first check for in this session. */
const autoChecked = new Set<string>();

export interface MapPageProps {
  tabID: string;
  /** The project's name, the page title. */
  projectName: string;
  /** Pre-loaded summary for Storybook and tests, which reach no backend. */
  summary?: MapSummary;
  /** Open the Decide view, on one row when a key is given. */
  onDecide: (rowKey?: string) => void;
  /** Go to another view of the project: "checks", "context:atlas", … */
  onNavigate: (view: string) => void;
  /**
   * The project waits to be set up: its folder is empty, or it matches a
   * preset. The files' empty state then leads to the page that sets it up.
   */
  setupPending?: boolean;
}

export function MapPage({
  tabID,
  projectName,
  summary: given,
  onDecide,
  onNavigate,
  setupPending,
}: MapPageProps) {
  const qc = useQueryClient();
  const key = qk.mapSummary(tabID);
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.mapSummary(tabID),
    enabled: !given,
    placeholderData: (previous) => previous,
  });
  useInvalidateOnEvent(["project:extracted", "project-files-changed", "workspace:changed"], [key]);

  // The store sync records when it found the cache current, so the page reads
  // again each time one settles.
  const { syncing } = useStoreSyncState(tabID);
  const wasSyncing = useRef(syncing);
  useEffect(() => {
    if (wasSyncing.current && !syncing) void qc.invalidateQueries({ queryKey: key });
    wasSyncing.current = syncing;
  }, [syncing, qc, key]);

  const summary = given ?? query.data ?? null;

  const run = useMutation({
    mutationFn: () =>
      api.runChecks(tabID, {
        id: "",
        name: "",
        languages: summary?.target_languages ?? [],
      }),
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });

  // A tab that has not been checked in this session is checked once, as soon
  // as the files are read, so the findings count is there when the page is.
  useEffect(() => {
    if (given || !summary || syncing || run.isPending) return;
    if (summary.findings !== null || summary.files === 0 || autoChecked.has(tabID)) return;
    autoChecked.add(tabID);
    run.mutate();
  }, [given, summary, syncing, run, tabID]);

  const [preview, setPreview] = useState<MapFileRow | null>(null);
  const filesRef = useRef<HTMLElement>(null);

  if (!summary) {
    return query.error ? (
      <div className="p-6">
        <ErrorNotice error={query.error} title={t("The project could not be read")} />
      </div>
    ) : (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    );
  }

  return (
    <ScrollArea className="h-full">
      <MapView
        projectName={projectName}
        summary={summary}
        running={run.isPending}
        runError={run.error}
        onRun={() => run.mutate()}
        onDecide={onDecide}
        onNavigate={onNavigate}
        setupPending={setupPending}
        onShowFiles={() => filesRef.current?.scrollIntoView({ behavior: "smooth" })}
        onOpenFile={setPreview}
        onAddFiles={async () => {
          const added = await api.addFilesDialog(tabID, "");
          if (added && added.length > 0) void qc.invalidateQueries({ queryKey: key });
        }}
        filesRef={filesRef}
      />
      <FilePreview
        tabID={tabID}
        filePath={preview?.path ?? null}
        filename={preview?.relative ?? ""}
        onClose={() => setPreview(null)}
      />
    </ScrollArea>
  );
}

export interface MapViewProps {
  projectName: string;
  summary: MapSummary;
  /** A checks run over the whole project is in progress. */
  running?: boolean;
  runError?: unknown;
  onRun: () => void;
  onDecide: (rowKey?: string) => void;
  onNavigate: (view: string) => void;
  /** The project waits to be set up (MapPageProps.setupPending). */
  setupPending?: boolean;
  onShowFiles?: () => void;
  onOpenFile: (row: MapFileRow) => void;
  onAddFiles?: () => void;
  filesRef?: React.Ref<HTMLElement>;
}

/** The Map as it is drawn, from a summary already read. */
export function MapView({
  projectName,
  summary,
  running,
  runError,
  onRun,
  onDecide,
  onNavigate,
  setupPending,
  onShowFiles,
  onOpenFile,
  onAddFiles,
  filesRef,
}: MapViewProps) {
  return (
    <div data-slot="map" className="mx-auto max-w-4xl space-y-8 p-6">
      <div className="space-y-5">
        <PageHeader
          className="mb-0"
          title={projectName}
          subtitle={<ReadLine summary={summary} />}
          actions={
            <>
              <Button
                size="sm"
                variant="outline"
                data-slot="map-run"
                disabled={running || summary.files === 0}
                onClick={onRun}
              >
                {running ? <LoadingSpinner size="sm" /> : <Play size={14} />}
                {running ? <>Checking</> : <>Run on everything</>}
              </Button>
              {summary.awaiting > 0 && (
                <Button size="sm" data-slot="map-decide" onClick={() => onDecide()}>
                  {t("Decide {count}", "a button: the items awaiting a decision", {
                    count: summary.awaiting,
                  })}
                </Button>
              )}
            </>
          }
        />
        <div className="flex flex-wrap items-end gap-x-7 gap-y-3">
          <MapCounts
            summary={summary}
            running={running}
            onNavigate={onNavigate}
            onShowFiles={onShowFiles}
          />
          {/* The Decide button carries the same number unless disagreements
              wait too, so the link shows only when the two differ. */}
          {summary.suggested > 0 && summary.suggested !== summary.awaiting && (
            <Button
              variant="link"
              size="sm"
              data-slot="map-suggests"
              className="h-auto px-0 text-xs"
              onClick={() => onDecide()}
            >
              {t("kapi suggests {count}", "a link to the suggestions awaiting a decision", {
                count: summary.suggested,
              })}
            </Button>
          )}
        </div>
        {!!runError && <ErrorNotice error={runError} title={t("The checks did not run")} />}
      </div>

      <section data-slot="map-suggestions" className="space-y-2">
        <SectionHeading>What your files do</SectionHeading>
        {summary.suggestions.length === 0 ? (
          <div data-slot="map-suggestions-empty">
            <EmptyState
              icon={<Sparkles size={20} className="mx-auto" />}
              title={t("No suggestions yet.")}
              action={
                <Button size="sm" variant="outline" onClick={() => onNavigate("context:agent")}>
                  Connect an assistant
                </Button>
              }
            />
          </div>
        ) : (
          <ul role="list" className="divide-y divide-border/60 border-y border-border/60">
            {summary.suggestions.map((row) => (
              <SuggestionRow key={decideRowKey(row)} row={row} onDecide={onDecide} />
            ))}
          </ul>
        )}
      </section>

      <section ref={filesRef} data-slot="map-files" className="space-y-2">
        <SectionHeading>Files</SectionHeading>
        {summary.file_rows.length === 0 ? (
          <div data-slot="map-files-empty">
            <EmptyState
              icon={<FileText size={20} className="mx-auto" />}
              title={t("No content files yet")}
              action={
                setupPending ? (
                  <Button size="sm" variant="outline" onClick={() => onNavigate("project-home")}>
                    Set up the project
                  </Button>
                ) : (
                  onAddFiles && (
                    <Button size="sm" variant="outline" onClick={onAddFiles}>
                      Add files
                    </Button>
                  )
                )
              }
            />
          </div>
        ) : (
          <ul role="list" className="divide-y divide-border/60 border-y border-border/60">
            {summary.file_rows.map((row) => (
              <FileRow key={row.relative} row={row} onOpen={() => onOpenFile(row)} />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

/** When kapi last read the project's files, under the title. */
// The files count beside it says how many there are, so the line names a
// number only when kapi read a different number of them (a file no reader
// opens, or a file waiting for the next read).
function ReadLine({ summary }: { summary: MapSummary }) {
  const sameCount = summary.read_files === summary.files;
  if (sameCount && !summary.read_at) return null;
  const when = summary.read_at && <When iso={summary.read_at} relative />;
  if (sameCount) {
    return <span data-slot="map-read">Read {when}</span>;
  }
  const read = t("Read {count, plural, one {# file} other {# files}}", "under the project title", {
    count: summary.read_files,
  });
  return (
    <span data-slot="map-read">
      {read}
      {when && <> {when}</>}
    </span>
  );
}

/** The four counts: files, blocks, rules and findings. */
function MapCounts({
  summary,
  running,
  onNavigate,
  onShowFiles,
}: {
  summary: MapSummary;
  running?: boolean;
  onNavigate: (view: string) => void;
  onShowFiles?: () => void;
}) {
  const findings = summary.findings ?? 0;
  const checked = summary.findings !== null;
  const items: StatItem[] = [
    {
      id: "files",
      value: summary.files,
      label: t("{count, plural, one {file} other {files}}", "a count on the Map", {
        count: summary.files,
      }),
      onClick: onShowFiles,
    },
    {
      id: "blocks",
      value: summary.blocks,
      label: t("{count, plural, one {block} other {blocks}}", "a count on the Map", {
        count: summary.blocks,
      }),
      onClick: () => onNavigate("context:explorer"),
    },
    {
      id: "rules",
      value: summary.rules,
      label: t("{count, plural, one {rule} other {rules}}", "a count on the Map", {
        count: summary.rules,
      }),
      onClick: () => onNavigate("context:atlas"),
    },
    {
      id: "findings",
      value: findings,
      label: t("{count, plural, one {finding} other {findings}}", "a count on the Map", {
        count: findings,
      }),
      tone: !checked ? "muted" : findings > 0 ? "warning" : "default",
      hint: running ? t("Checking") : !checked ? t("Not checked yet") : undefined,
      onClick: () => onNavigate("checks"),
    },
  ];
  return <StatRow data-slot="map-counts" items={items} />;
}

/** One item awaiting a decision: its kind, the rule, its evidence, and Decide. */
function SuggestionRow({
  row,
  onDecide,
}: {
  row: MapSuggestionRow;
  onDecide: (rowKey?: string) => void;
}) {
  const quote = row.item.quote?.quote;
  return (
    <li
      data-slot="map-suggestion"
      data-id={row.item.id}
      data-group={row.group}
      className="flex min-h-11 items-center gap-3 py-1.5"
    >
      <KindIcon kind={digestKind(row.item)} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm">
          {row.conflict ? <ConflictLine conflict={row.conflict} /> : <RuleLine item={row.item} />}
        </div>
        <EvidenceMeta quote={quote} blocks={row.blocks} files={row.files} />
      </div>
      <Button
        size="sm"
        variant="outline"
        data-slot="map-suggestion-decide"
        onClick={() => onDecide(decideRowKey(row))}
      >
        Decide
      </Button>
    </li>
  );
}

/** The evidence on one meta line: the words it was seen in, and where. */
function EvidenceMeta({ quote, blocks, files }: { quote?: string; blocks: number; files: number }) {
  if (!quote && blocks === 0 && files === 0) return null;
  const facts: string[] = [];
  if (blocks > 0) {
    facts.push(
      t("{count, plural, one {# block} other {# blocks}}", "evidence on a Map row", {
        count: blocks,
      }),
    );
  }
  if (files > 0) {
    facts.push(
      t("{count, plural, one {# file} other {# files}}", "evidence on a Map row", {
        count: files,
      }),
    );
  }
  return (
    <div className="flex min-w-0 items-baseline gap-1 text-[11px] text-muted-foreground">
      {quote && (
        <q data-slot="map-suggestion-quote" className="min-w-0 truncate">
          {quote}
        </q>
      )}
      {facts.length > 0 && (
        <span className="shrink-0 tabular-nums">
          {quote ? " · " : ""}
          {facts.join(" · ")}
        </span>
      )}
    </div>
  );
}

/** One content file: its format, its name, its blocks and findings, and Open. */
function FileRow({ row, onOpen }: { row: MapFileRow; onOpen: () => void }) {
  const slash = row.relative.lastIndexOf("/");
  const name = slash >= 0 ? row.relative.slice(slash + 1) : row.relative;
  const folder = slash >= 0 ? row.relative.slice(0, slash) : "";
  return (
    <li
      data-slot="map-file"
      data-path={row.relative}
      className="flex min-h-11 items-center gap-3 py-1.5"
    >
      <Badge
        variant="outline"
        translate="no"
        className="w-20 shrink-0 justify-center truncate font-mono text-[10px] font-normal text-muted-foreground"
      >
        {row.format || "?"}
      </Badge>
      <div className="min-w-0 flex-1">
        <SimpleTooltip content={row.relative}>
          <span translate="no" className="block truncate text-sm">
            {name}
          </span>
        </SimpleTooltip>
        <div className="flex min-w-0 items-baseline gap-1 text-[11px] text-muted-foreground">
          {folder && (
            <span translate="no" className="min-w-0 truncate">
              {folder}
              {" · "}
            </span>
          )}
          <span className="shrink-0 tabular-nums">
            {t("{count, plural, one {# block} other {# blocks}}", "a file's blocks on the Map", {
              count: row.blocks,
            })}
          </span>
          {row.findings > 0 && (
            <span data-slot="map-file-findings" className="shrink-0 tabular-nums">
              {" · "}
              {t(
                "{count, plural, one {# finding} other {# findings}}",
                "a file's findings on the Map",
                { count: row.findings },
              )}
            </span>
          )}
        </div>
      </div>
      <Button size="sm" variant="ghost" data-slot="map-file-open" onClick={onOpen}>
        Open
      </Button>
    </li>
  );
}
