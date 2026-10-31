// The Map: the page a project opens on. It is an overview (page-patterns
// "Overview"): the project's name and when kapi last read its files, four
// counts, the suggestions that wait for a person and the content files, each
// row leading to the place that acts on it. Nothing on it is edited in place.
//
// MapView draws one MapSummary already read. The host fills the parts only it
// has (the deeper look's row, the Connect your AI card, the add-files action,
// the file preview) as slots, and says which targets it opens: a target it
// cannot open hides the button that leads there. Without onRun the page draws
// no check action.

import { FileText, Play, Sparkles } from "lucide-react";
import {
  Badge,
  Button,
  EmptyState,
  ErrorNotice,
  KindIcon,
  LoadingSpinner,
  PageHeader,
  SectionHeading,
  SimpleTooltip,
  StatRow,
  When,
  digestKind,
  type StatItem,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { ConflictLine, RuleLine } from "./DigestRule";
import { opens, type ScreenNavigation, type ScreenTarget } from "./port";
import type { MapFileRow, MapSuggestionRow, MapSummary } from "./types";

/** The Decide view's key for a row: its group and its item's id. */
export function decideRowKey(row: MapSuggestionRow): string {
  return `${row.group}:${row.item.id}`;
}

export interface MapViewProps extends ScreenNavigation {
  projectName: string;
  summary: MapSummary;
  /** A checks run over the whole project is in progress. */
  running?: boolean;
  runError?: unknown;
  /** Run the checks over the whole project. Absent, the page draws no check action. */
  onRun?: () => void;
  /** The deeper look's row under "What your files do"; absent draws none. */
  deeperLook?: React.ReactNode;
  /** The Connect your AI card, drawn under the deeper look's row when open. */
  connectAI?: React.ReactNode;
  /** The action in the empty suggestions list; absent, Connect an assistant when the host opens it. */
  suggestionsAction?: React.ReactNode;
  /** The project waits to be set up: the files' empty state leads to setting it up. */
  setupPending?: boolean;
  /** The action in the empty files list, such as adding files. */
  addFiles?: React.ReactNode;
  /** Show a file the Reader does not open: one with no blocks, or any when the host opens no Reader. */
  onPreviewFile?: (row: MapFileRow) => void;
  /** The file preview, drawn after the page. */
  filePreview?: React.ReactNode;
  onShowFiles?: () => void;
  filesRef?: React.Ref<HTMLElement>;
}

/** The Map as it is drawn, from a summary already read. */
export function MapView({
  projectName,
  summary,
  running,
  runError,
  onRun,
  deeperLook,
  connectAI,
  suggestionsAction,
  setupPending,
  addFiles,
  onPreviewFile,
  filePreview,
  onShowFiles,
  filesRef,
  navigate,
  canNavigate,
}: MapViewProps) {
  const nav: ScreenNavigation = { navigate, canNavigate };
  const decides = opens(nav, { kind: "rules" });
  const onDecide = (row?: string) => navigate(row ? { kind: "rules", row } : { kind: "rules" });
  // A file opens in the Reader when the block cache holds blocks for it and
  // the host opens the Reader; otherwise in the preview, when there is one.
  const openFile = (row: MapFileRow): (() => void) | undefined => {
    const reader: ScreenTarget = { kind: "reader", path: row.relative };
    if (row.blocks > 0 && opens(nav, reader)) return () => navigate(reader);
    if (onPreviewFile) return () => onPreviewFile(row);
    return undefined;
  };
  const connect: ScreenTarget = { kind: "connect-assistant" };
  const emptySuggestionsAction =
    suggestionsAction ??
    (opens(nav, connect) && (
      <Button size="sm" variant="outline" onClick={() => navigate(connect)}>
        Connect an assistant
      </Button>
    ));
  return (
    <>
      <div data-slot="map" className="mx-auto max-w-4xl space-y-8 p-6">
        <div className="space-y-5">
          <PageHeader
            className="mb-0"
            title={projectName}
            subtitle={<ReadLine summary={summary} />}
            actions={
              <>
                {onRun && (
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
                )}
                {summary.awaiting > 0 && decides && (
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
            <MapCounts summary={summary} running={running} nav={nav} onShowFiles={onShowFiles} />
            {/* The Decide button carries the same number unless disagreements
                wait too, so the link shows only when the two differ. */}
            {summary.suggested > 0 && summary.suggested !== summary.awaiting && decides && (
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
                action={emptySuggestionsAction || undefined}
              />
            </div>
          ) : (
            <ul role="list" className="divide-y divide-border/60 border-y border-border/60">
              {summary.suggestions.map((row) => (
                <SuggestionRow
                  key={decideRowKey(row)}
                  row={row}
                  onDecide={decides ? onDecide : undefined}
                />
              ))}
            </ul>
          )}
          {deeperLook}
          {connectAI}
        </section>

        <section ref={filesRef} data-slot="map-files" className="space-y-2">
          <SectionHeading>Files</SectionHeading>
          {summary.file_rows.length === 0 ? (
            <div data-slot="map-files-empty">
              <EmptyState
                icon={<FileText size={20} className="mx-auto" />}
                title={t("No content files yet")}
                action={
                  setupPending
                    ? opens(nav, { kind: "setup" }) && (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => navigate({ kind: "setup" })}
                        >
                          Set up the project
                        </Button>
                      )
                    : addFiles
                }
              />
            </div>
          ) : (
            <ul role="list" className="divide-y divide-border/60 border-y border-border/60">
              {summary.file_rows.map((row) => (
                <FileRow key={row.relative} row={row} onOpen={openFile(row)} />
              ))}
            </ul>
          )}
        </section>
      </div>
      {filePreview}
    </>
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

/** The four counts: files, blocks, rules and findings. A count whose place the host does not open is not a link. */
function MapCounts({
  summary,
  running,
  nav,
  onShowFiles,
}: {
  summary: MapSummary;
  running?: boolean;
  nav: ScreenNavigation;
  onShowFiles?: () => void;
}) {
  const findings = summary.findings ?? 0;
  const checked = summary.findings !== null;
  const to = (target: ScreenTarget) =>
    opens(nav, target) ? () => nav.navigate(target) : undefined;
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
      onClick: to({ kind: "blocks" }),
    },
    {
      id: "rules",
      value: summary.rules,
      label: t("{count, plural, one {rule} other {rules}}", "a count on the Map", {
        count: summary.rules,
      }),
      onClick: to({ kind: "kept" }),
    },
    {
      id: "findings",
      value: findings,
      label: t("{count, plural, one {finding} other {findings}}", "a count on the Map", {
        count: findings,
      }),
      tone: !checked ? "muted" : findings > 0 ? "warning" : "default",
      hint: running ? t("Checking") : !checked ? t("Not checked yet") : undefined,
      onClick: to({ kind: "checks" }),
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
  onDecide?: (rowKey?: string) => void;
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
      {onDecide && (
        <Button
          size="sm"
          variant="outline"
          data-slot="map-suggestion-decide"
          onClick={() => onDecide(decideRowKey(row))}
        >
          Decide
        </Button>
      )}
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

/** One content file: its format, its name, its blocks and findings, and Open when it opens. */
function FileRow({ row, onOpen }: { row: MapFileRow; onOpen?: () => void }) {
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
      {onOpen && (
        <Button size="sm" variant="ghost" data-slot="map-file-open" onClick={onOpen}>
          Open
        </Button>
      )}
    </li>
  );
}
