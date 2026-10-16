// The workspace's content memory as a list of pairs beside a detail pane: the
// platform's twin of Kapi Desktop's Content memory page.
//
// The server reads each entry as a pair, its source and its wording in one
// target language, so the list has one row per pair: the source on one line
// and the target's language at the right. The search at the top of the list
// asks the server, a page at a time, and the language picker beside it chooses
// the target language when the workspace translates into more than one.
//
// The box at the start of a row marks its entry, as does x on the selected
// row, and Mark all shown marks every entry in view. While any entry is marked
// the detail pane holds what can be done to all of them; otherwise it holds
// the selected pair (MemoryPairDetail). Add entry is the page's own act.
import { useMemo, useState } from "react";
import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { t, useNeokapi } from "@neokapi/i18n-react/runtime";
import {
  Button,
  EmptyState,
  Input,
  ListDetail,
  ListDetailEmpty,
  ListDetailItem,
  ListDetailList,
  LoadingSpinner,
  LocaleLabel,
  PageHeader,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
  formatLocale,
  useDebounced,
} from "@neokapi/ui-primitives";
import { Check, Languages, Plus, Search } from "../../components/icons";
import { ErrorNotice } from "../../errors";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { useProjects } from "../../hooks/useProjectApi";
import type { MemoryEntryInfo } from "../../types/api";
import { pairId, pairTestId, workspaceLanguages } from "./memoryRows";
import {
  AddEntryDialog,
  BulkDetail,
  PairDetail,
  type NewPair,
  type PairEdit,
} from "./MemoryPairDetail";

/** How many entries one read of the content memory carries. */
export const MEMORY_PAGE = 100;

/** The bulk-delete route takes at most this many ids per request. */
const MAX_BULK_DELETE_IDS = 500;

const NO_MARKS: ReadonlyMap<string, MemoryEntryInfo> = new Map();

export interface MemorySectionProps {
  /** The pair to open on, by its row id. Absent opens with nothing selected. */
  defaultSelected?: string;
  /** Entries to open with marked, by id, once they load. For Storybook. */
  defaultMarked?: string[];
}

export function MemorySection({ defaultSelected, defaultMarked }: MemorySectionProps) {
  const api = useApi();
  const qc = useQueryClient();
  const { locale } = useNeokapi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const { data: projects } = useProjects();
  const languages = useMemo(() => workspaceLanguages(projects), [projects]);
  const projectNames = useMemo(
    () => new Map((projects ?? []).map((p) => [p.id, p.name])),
    [projects],
  );

  const [text, setText] = useState("");
  const q = useDebounced(text.trim());
  const [picked, setPicked] = useState("");
  // The language a reader picked stands while the workspace still offers it.
  const target = languages.targets.includes(picked) ? picked : (languages.targets[0] ?? "");
  const [selectedId, setSelectedId] = useState<string | null>(defaultSelected ?? null);
  const [marked, setMarked] = useState<ReadonlyMap<string, MemoryEntryInfo>>(NO_MARKS);
  const [adding, setAdding] = useState(false);

  const count = useQuery({
    queryKey: ["memory", ws, "count"],
    queryFn: () => api.getMemoryCount(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });

  const list = useInfiniteQuery({
    queryKey: ["memory", ws, "browse", q, languages.source, target],
    queryFn: ({ pageParam }) =>
      api.getMemoryEntries(ws, q, languages.source, target, pageParam, MEMORY_PAGE),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + (p.entries?.length ?? 0), 0);
      return (last.entries?.length ?? 0) > 0 && loaded < last.total_count ? loaded : undefined;
    },
    enabled: !!ws,
    // The previous answer stays on screen while the next search is read.
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
  const entries = useMemo(
    () => list.data?.pages.flatMap((p) => p.entries ?? []) ?? [],
    [list.data],
  );
  const total = list.data?.pages[0]?.total_count ?? 0;
  const selected = entries.find((e) => pairId(e) === selectedId) ?? null;

  // The entries a story asks to open marked, once the first page has them.
  const [seeded, setSeeded] = useState(false);
  if (!seeded && defaultMarked?.length && entries.length > 0) {
    setSeeded(true);
    const wanted = new Set(defaultMarked);
    setMarked(new Map(entries.filter((e) => wanted.has(e.id)).map((e) => [e.id, e])));
  }

  const refresh = () => void qc.invalidateQueries({ queryKey: ["memory", ws] });
  const update = useMutation({
    mutationFn: ({ entry, edit }: { entry: MemoryEntryInfo; edit: PairEdit }) =>
      api.updateMemoryEntry(ws, {
        project_id: entry.project_id ?? "",
        entry_id: entry.id,
        source: edit.source,
        target: edit.target,
        source_locale: entry.source_language,
        target_locale: entry.target_language,
      }),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: (entry: MemoryEntryInfo) => api.deleteMemoryEntry(ws, entry.id),
    onSuccess: () => setSelectedId(null),
    onSettled: refresh,
  });
  const removeMarked = useMutation({
    // One request per batch instead of one per row. Each id carries its own
    // outcome, so a selection that partly fails has really partly deleted, and
    // the error says how many are left.
    mutationFn: async (ids: string[]) => {
      let failed = 0;
      let reason: string | undefined;
      for (let i = 0; i < ids.length; i += MAX_BULK_DELETE_IDS) {
        const result = await api.bulkDeleteMemoryEntries(ws, ids.slice(i, i + MAX_BULK_DELETE_IDS));
        for (const r of result.results) {
          if (r.deleted) continue;
          failed++;
          reason ??= r.error;
        }
      }
      if (failed > 0) {
        const head = `${failed} of ${ids.length} entries could not be deleted`;
        throw new Error(reason ? `${head}: ${reason}` : `${head}.`);
      }
    },
    onSuccess: (_r, ids) => {
      setMarked(NO_MARKS);
      if (selected && ids.includes(selected.id)) setSelectedId(null);
    },
    onSettled: refresh,
  });
  const add = useMutation({
    mutationFn: (pair: NewPair) =>
      api.addMemoryEntry(ws, pair.source, pair.target, pair.sourceLocale, pair.targetLocale),
    onSuccess: () => setAdding(false),
    onSettled: refresh,
  });

  const toggleMark = (entry: MemoryEntryInfo) =>
    setMarked((prev) => {
      const next = new Map(prev);
      if (next.has(entry.id)) next.delete(entry.id);
      else next.set(entry.id, entry);
      return next;
    });
  const allMarked = entries.length > 0 && entries.every((e) => marked.has(e.id));
  const toggleAll = () =>
    setMarked((prev) => {
      const next = new Map(prev);
      if (allMarked) for (const e of entries) next.delete(e.id);
      else for (const e of entries) next.set(e.id, e);
      return next;
    });

  const allLanguages = useMemo(
    () => [...new Set([languages.source, ...languages.targets].filter(Boolean))],
    [languages],
  );
  const addDialog = (
    <AddEntryDialog
      open={adding}
      onOpenChange={(open) => {
        if (!open) add.reset();
        setAdding(open);
      }}
      languages={allLanguages}
      defaultSource={languages.source}
      defaultTarget={target}
      busy={add.isPending}
      error={add.error}
      onSubmit={(pair) => add.mutate(pair)}
    />
  );

  const header = (
    <PageHeader
      className="mb-0 shrink-0"
      title="Content memory"
      subtitle={
        count.data !== undefined
          ? t("{count, plural, one {# entry} other {# entries}}", { count: count.data })
          : undefined
      }
      actions={
        <Button size="sm" data-slot="memory-add" onClick={() => setAdding(true)}>
          <Plus size={13} />
          Add entry
        </Button>
      }
    />
  );

  let body: React.ReactNode;
  if (list.error && !list.data) {
    body = <ErrorNotice error={list.error} title={t("The content memory could not be read")} />;
  } else if (list.isLoading || !ws) {
    body = <LoadingSpinner />;
  } else if (entries.length === 0 && !q) {
    body = (
      <EmptyState
        className="mx-auto w-full max-w-md"
        icon={<Languages className="mx-auto size-5" />}
        title={
          target
            ? t("No approved wording into {language} yet", {
                language: formatLocale(target, { uiLocale: locale, variant: "short" }).name,
              })
            : t("No approved wording yet")
        }
        description={t(
          "Each translation a reviewer approves is kept here, so every project in the workspace can reuse it.",
        )}
        action={
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            Add entry
          </Button>
        }
      />
    );
  } else {
    const listPane = (
      <>
        <div className="sticky top-0 z-10 space-y-1.5 bg-background px-2 pt-2 pb-1">
          <div className="relative">
            <Search
              aria-hidden
              className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              type="search"
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                // Down from the search goes into the list, onto its first row.
                if (e.key === "ArrowDown" && entries.length > 0) {
                  e.preventDefault();
                  setSelectedId((id) => id ?? pairId(entries[0]));
                  e.currentTarget
                    .closest("[data-pane='list']")
                    ?.querySelector<HTMLElement>("[role='listbox']")
                    ?.focus();
                }
              }}
              placeholder={t("Search wording")}
              aria-label={t("Search content memory")}
              data-testid="tm-search"
              className="h-8 pl-8 text-sm"
            />
          </div>
          {languages.targets.length > 1 && (
            <Select value={target} onValueChange={setPicked}>
              <SelectTrigger
                size="sm"
                data-slot="memory-language"
                aria-label={t("Target language")}
                className="h-8 w-full text-xs"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {languages.targets.map((code) => (
                  <SelectItem key={code} value={code}>
                    <LocaleLabel locale={code} />
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <div className="flex items-center gap-2 px-1 text-[11px] text-muted-foreground">
            {entries.length > 0 && (
              <button
                type="button"
                data-slot="memory-mark-all"
                onClick={toggleAll}
                className="rounded hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
              >
                {allMarked ? <>Unmark all shown</> : <>Mark all shown</>}
              </button>
            )}
            {q && (
              <span className="tabular-nums">
                {t("{count, plural, one {# entry matches} other {# entries match}}", {
                  count: total,
                })}
              </span>
            )}
          </div>
        </div>
        {entries.length === 0 ? (
          <p className="px-4 py-6 text-center text-xs text-muted-foreground">
            No entry matches this search.
          </p>
        ) : (
          <ListDetailList
            selectedId={selectedId}
            onSelect={setSelectedId}
            aria-label={t("Entries")}
            className="pt-0"
            onKeyDown={(e) => {
              // x marks the selected row's entry, as the box at its start does.
              if (e.key === "x" && !e.metaKey && !e.ctrlKey && !e.altKey && selected) {
                e.preventDefault();
                toggleMark(selected);
              }
            }}
          >
            {entries.map((entry) => (
              <PairRow
                key={pairId(entry)}
                entry={entry}
                marked={marked.has(entry.id)}
                onToggleMark={() => toggleMark(entry)}
              />
            ))}
          </ListDetailList>
        )}
        {list.hasNextPage && (
          <div className="px-2 pb-3">
            <Button
              variant="ghost"
              size="sm"
              data-slot="memory-more"
              className="w-full text-muted-foreground"
              disabled={list.isFetchingNextPage}
              onClick={() => void list.fetchNextPage()}
            >
              {t("Show {count} more entries", {
                count: Math.min(MEMORY_PAGE, total - entries.length),
              })}
            </Button>
          </div>
        )}
      </>
    );

    let detail: React.ReactNode;
    if (marked.size > 0) {
      detail = (
        <BulkDetail
          entries={[...marked.values()]}
          deleting={removeMarked.isPending}
          error={removeMarked.error}
          onDelete={() => removeMarked.mutate([...marked.keys()])}
          onClear={() => {
            removeMarked.reset();
            setMarked(NO_MARKS);
          }}
        />
      );
    } else if (selected) {
      detail = (
        <PairDetail
          key={pairId(selected)}
          entry={selected}
          projectName={selected.project_id ? projectNames.get(selected.project_id) : undefined}
          saving={update.isPending}
          error={update.error ?? remove.error}
          onSave={(edit) => update.mutateAsync({ entry: selected, edit })}
          onDelete={() => remove.mutate(selected)}
        />
      );
    } else {
      detail = (
        <ListDetailEmpty
          title={t("Choose an entry to see it here")}
          description={t("The source and its translation in full, and where the entry came from.")}
        />
      );
    }

    body = (
      <div
        data-testid="tm-browser"
        data-slot="memory-browser"
        className="min-h-[420px] flex-1 overflow-hidden rounded-lg border border-border"
      >
        <ListDetail list={listPane} listWidth={360} detail={detail} />
      </div>
    );
  }

  return (
    <div data-slot="memory-page" className="flex min-h-0 flex-1 flex-col gap-3">
      {header}
      {body}
      {addDialog}
    </div>
  );
}

/** One pair in the list: its mark, the source text on one line, and the target's language. */
function PairRow({
  entry,
  marked,
  onToggleMark,
}: {
  entry: MemoryEntryInfo;
  marked: boolean;
  onToggleMark: () => void;
}) {
  return (
    <ListDetailItem
      id={pairId(entry)}
      data-slot="memory-pair"
      data-testid={pairTestId(entry)}
      data-marked={marked || undefined}
      // Rows off screen skip layout and paint, so a memory of thousands of
      // entries scrolls as lightly as a short one.
      className="[contain-intrinsic-size:auto_36px] [content-visibility:auto]"
      leading={<MarkBox marked={marked} onToggle={onToggleMark} />}
      trailing={
        entry.target_language ? <LocaleLabel locale={entry.target_language} compact /> : undefined
      }
    >
      <span translate="no">{entry.source}</span>
      {marked && <span className="sr-only">{t("marked", "content memory entry")}</span>}
    </ListDetailItem>
  );
}

/**
 * The box that marks a row's entry. A click on it marks and leaves the
 * selection where it is; the list's x does the same from the keyboard, so the
 * box itself stays out of the tab order and out of the row's name.
 */
function MarkBox({ marked, onToggle }: { marked: boolean; onToggle: () => void }) {
  return (
    <span
      aria-hidden
      data-slot="memory-mark"
      onClick={(e) => {
        // A handled click leaves the row's own selection alone.
        e.preventDefault();
        onToggle();
      }}
      className={cn(
        "flex size-3.5 cursor-pointer items-center justify-center rounded-[4px] border",
        marked
          ? "border-primary bg-primary text-primary-foreground"
          : "border-muted-foreground/40 hover:border-foreground",
      )}
    >
      {marked && <Check className="size-3" />}
    </span>
  );
}
