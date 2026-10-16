// A content memory as a list of pairs beside a detail pane.
//
// An entry holds one sentence in several languages. The list has one row per
// pair, the entry's source text and one of its other languages, so an entry in
// three languages is two rows. The search at the top of the list filters as
// you type; the store answers it, a page at a time. A language picker beside it
// narrows the rows to one target language when the store holds several.
//
// The detail pane holds the selected pair: the source and the target in full,
// with their inline codes as chips, the entry's other languages, where the
// entry came from, and the unit it was approved for. Edit, Mark entities and
// Delete sit in the pane's toolbar; Add entry is the page's.

import { useEffect, useMemo, useRef, useState } from "react";
import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ArrowRight, Database, FileText, Search, Upload, User, Wrench } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { flattenRuns, type Run } from "@neokapi/kapi-format";
import {
  ActorMark,
  Button,
  CodedTextDisplay,
  ConfirmDeleteButton,
  CoordinateChip,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  EmptyState,
  EntityAnnotationDialog,
  ErrorNotice,
  InlineCodeEditor,
  Input,
  Label,
  ListDetail,
  ListDetailEmpty,
  ListDetailItem,
  ListDetailList,
  ListDetailPane,
  ListDetailToolbar,
  LoadingSpinner,
  LocaleLabel,
  LocaleSelect,
  SimpleTooltip,
  Textarea,
  When,
  codedToRuns,
  runsToCoded,
  useDebounced,
  type LocaleInfo,
  type MemoryAdapter,
  type MemoryEntryDTO,
  type MemoryPointDTO,
  type OriginDTO,
  type SpanInfo,
  type VariantDTO,
  type VariantInputDTO,
} from "@neokapi/ui-primitives";

/** How many entries one read of the store carries. */
export const MEMORY_PAGE = 100;

/** One row of the list: an entry's source and one of its other languages. */
export interface MemoryPair {
  /** `<entry id>::<target locale>`, unique in the list. */
  id: string;
  entry: MemoryEntryDTO;
  source: string;
  /** Empty for an entry that holds its source language alone. */
  target: string;
}

/** The language an entry is read from: its hint, else the first it holds. */
export function sourceLocaleOf(entry: MemoryEntryDTO): string {
  if (entry.hint_src_lang && entry.variants[entry.hint_src_lang]) return entry.hint_src_lang;
  return Object.keys(entry.variants)[0] ?? "";
}

/** The rows an entry contributes: one per language other than its source. */
export function pairsOf(entry: MemoryEntryDTO, targets?: ReadonlySet<string>): MemoryPair[] {
  const source = sourceLocaleOf(entry);
  const others = Object.keys(entry.variants).filter((l) => l !== source);
  if (others.length === 0) {
    return targets ? [] : [{ id: `${entry.id}::`, entry, source, target: "" }];
  }
  return others
    .filter((l) => !targets || targets.has(l))
    .map((target) => ({ id: `${entry.id}::${target}`, entry, source, target }));
}

/** A variant as the store takes it back: the runs, and the text they flatten to. */
function variantInput(runs: Run[]): VariantInputDTO {
  const input: VariantInputDTO = { text: flattenRuns(runs) };
  if (runs.length > 0) input.runs = runs;
  return input;
}

/** The editor's shape for a variant. A plural or a select opens as plain text. */
function editable(runs: Run[]): { codedText: string; spans: SpanInfo[] } {
  try {
    return runsToCoded(runs);
  } catch {
    return { codedText: "", spans: [] };
  }
}

export interface MemoryListDetailProps {
  /** The store, through the host's adapter. */
  adapter: MemoryAdapter;
  /** Names the store in query keys, so two open stores never share a cache. */
  handle: string;
  /** The target languages to show, from the Active Filter. Absent shows every one. */
  targetLocales?: string[];
  /** Languages for the add-entry form. */
  locales?: LocaleInfo[];
  /** The project's source language, the add-entry form's default. */
  sourceLocale?: string;
  /** The add-entry dialog is open; the page header's button opens it. */
  adding?: boolean;
  onAddingChange?: (open: boolean) => void;
  /** The pair to open on, by its id. Absent opens with nothing selected. */
  defaultSelected?: string;
  onError?: (message: string, details?: unknown) => void;
}

export function MemoryListDetail({
  adapter,
  handle,
  targetLocales,
  locales = [],
  sourceLocale = "",
  adding = false,
  onAddingChange,
  defaultSelected,
  onError,
}: MemoryListDetailProps) {
  const qc = useQueryClient();
  const [text, setText] = useState("");
  const query = useDebounced(text.trim());
  const [language, setLanguage] = useState("");

  // The languages the store holds, for the picker beside the search.
  const facets = useQuery({
    queryKey: ["tm", "facets", handle],
    queryFn: () => (adapter.getFacets ? adapter.getFacets() : null),
  });
  const storeLocales = useMemo(
    () => (facets.data?.locales ?? []).map((l) => l.locale),
    [facets.data],
  );

  // A target picked here wins; the Active Filter's languages apply otherwise.
  const targets = useMemo<ReadonlySet<string> | undefined>(() => {
    if (language) return new Set([language]);
    return targetLocales?.length ? new Set(targetLocales) : undefined;
  }, [language, targetLocales]);
  const requireLocale = targets?.size === 1 ? [...targets][0] : "";

  const list = useInfiniteQuery({
    queryKey: ["tm", "entries", handle, query, requireLocale],
    queryFn: ({ pageParam }) => adapter.search(query, "", requireLocale, pageParam, MEMORY_PAGE),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + (p.entries?.length ?? 0), 0);
      return (last.entries?.length ?? 0) > 0 && loaded < last.total_count ? loaded : undefined;
    },
    // The previous answer stays on screen while the next search is read.
    placeholderData: keepPreviousData,
  });
  const entries = useMemo(
    () => list.data?.pages.flatMap((p) => p.entries ?? []) ?? [],
    [list.data],
  );
  const total = list.data?.pages[0]?.total_count ?? 0;
  const pairs = useMemo(() => entries.flatMap((e) => pairsOf(e, targets)), [entries, targets]);

  const [selectedId, setSelectedId] = useState<string | null>(defaultSelected ?? null);
  const selected = pairs.find((p) => p.id === selectedId) ?? null;

  const refresh = () => void qc.invalidateQueries({ queryKey: ["tm"] });
  const update = useMutation({
    mutationFn: ({
      entry,
      locale,
      runs,
    }: {
      entry: MemoryEntryDTO;
      locale: string;
      runs: Run[];
    }) => {
      const variants: Record<string, VariantInputDTO> = {};
      for (const [l, v] of Object.entries(entry.variants)) {
        variants[l] = variantInput(l === locale ? runs : v.runs);
      }
      return adapter.updateEntry({
        entry_id: entry.id,
        variants,
        hint_src_lang: entry.hint_src_lang,
        project_id: entry.project_id,
        note: entry.note,
      });
    },
    onError: (err) => onError?.(t("The entry could not be saved"), err),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: (entry: MemoryEntryDTO) => adapter.deleteEntry(entry.id),
    onSuccess: () => setSelectedId(null),
    onError: (err) => onError?.(t("The entry could not be deleted"), err),
    onSettled: refresh,
  });
  const add = useMutation({
    mutationFn: (req: { source: string; target: string; srcLocale: string; tgtLocale: string }) =>
      adapter.addEntry({
        variants: {
          [req.srcLocale]: { text: req.source },
          [req.tgtLocale]: { text: req.target },
        },
        hint_src_lang: req.srcLocale,
      }),
    onSuccess: () => onAddingChange?.(false),
    onError: (err) => onError?.(t("The entry could not be added"), err),
    onSettled: refresh,
  });

  const addDialog = (
    <AddEntryDialog
      open={adding}
      onOpenChange={(open) => onAddingChange?.(open)}
      locales={locales}
      defaultSource={sourceLocale || storeLocales[0] || ""}
      defaultTarget={
        language || targetLocales?.[0] || storeLocales.find((l) => l !== sourceLocale) || ""
      }
      busy={add.isPending}
      onSubmit={(req) => add.mutate(req)}
    />
  );

  if (list.error && !list.data) {
    return (
      <div className="p-6">
        <ErrorNotice error={list.error} title="The content memory could not be read" />
      </div>
    );
  }
  if (list.isLoading) {
    return (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    );
  }
  if (entries.length === 0 && !query && !language) {
    return (
      <div className="p-6">
        <EmptyState
          icon={<Database size={20} />}
          title="No approved wording yet"
          description="Each translation you approve is kept here with the file it came from, so the next run can reuse it."
          action={
            onAddingChange && (
              <Button size="sm" variant="outline" onClick={() => onAddingChange(true)}>
                Add entry
              </Button>
            )
          }
        />
        {addDialog}
      </div>
    );
  }

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
              if (e.key === "ArrowDown" && pairs.length > 0) {
                e.preventDefault();
                setSelectedId((id) => id ?? pairs[0].id);
                e.currentTarget
                  .closest("[data-pane='list']")
                  ?.querySelector<HTMLElement>("[role='listbox']")
                  ?.focus();
              }
            }}
            placeholder={t("Search wording")}
            aria-label="Search content memory"
            data-testid="tm-search"
            className="h-8 pl-8 text-sm"
          />
        </div>
        {storeLocales.length > 2 && (
          <LocaleSelect
            value={language}
            onChange={setLanguage}
            locales={storeLocales.map((code) => ({
              code,
              displayName: locales.find((l) => l.code === code)?.displayName ?? code,
            }))}
            placeholder={t("Every language")}
            clearLabel={t("Every language")}
            aria-label={t("Target language")}
            data-slot="memory-language"
            className="h-8 w-full"
          />
        )}
        {(query || language) && (
          <p className="px-1 text-[11px] text-muted-foreground tabular-nums">
            {t("{count, plural, one {# entry matches} other {# entries match}}", { count: total })}
          </p>
        )}
      </div>
      {pairs.length === 0 ? (
        <p className="px-4 py-6 text-center text-xs text-muted-foreground">
          No entry matches this search.
        </p>
      ) : (
        <ListDetailList
          selectedId={selectedId}
          onSelect={setSelectedId}
          aria-label={t("Entries")}
          className="pt-0"
        >
          {pairs.map((pair) => (
            <PairRow key={pair.id} pair={pair} />
          ))}
        </ListDetailList>
      )}
      {list.hasNextPage && (
        <div className="px-2 pb-3">
          <Button
            variant="ghost"
            size="sm"
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

  return (
    <div data-testid="tm-browser" data-slot="memory-browser" className="h-full min-h-0">
      <ListDetail
        list={listPane}
        listWidth={340}
        detail={
          selected ? (
            <PairDetail
              key={selected.id}
              pair={selected}
              adapter={adapter}
              search={query}
              saving={update.isPending}
              onSelect={setSelectedId}
              onSave={(locale, runs) => update.mutateAsync({ entry: selected.entry, locale, runs })}
              onDelete={() => remove.mutate(selected.entry)}
              onAnnotated={refresh}
            />
          ) : (
            <ListDetailEmpty
              title={t("Choose an entry to see it here")}
              description={t(
                "The source and its translation in full, and where the entry came from.",
              )}
            />
          )
        }
      />
      {addDialog}
    </div>
  );
}

/** One pair in the list: the source text on one line, and the target's language. */
function PairRow({ pair }: { pair: MemoryPair }) {
  const variant = pair.entry.variants[pair.source];
  return (
    <ListDetailItem
      id={pair.id}
      data-slot="memory-pair"
      data-testid={`tm-entry-${pair.entry.id}${pair.target ? `-${pair.target}` : ""}`}
      // Rows off screen skip layout and paint, so a store of thousands of
      // entries scrolls as lightly as a short one.
      className="[contain-intrinsic-size:auto_36px] [content-visibility:auto]"
      trailing={pair.target ? <LocaleLabel locale={pair.target} compact /> : undefined}
    >
      <CodedTextDisplay
        runs={variant?.runs}
        text={variant?.text}
        locale={pair.source}
        className="[&_*]:whitespace-nowrap"
      />
    </ListDetailItem>
  );
}

interface PairDetailProps {
  pair: MemoryPair;
  adapter: MemoryAdapter;
  /** The search in the list, the pattern Mark entities starts from. */
  search: string;
  saving: boolean;
  onSelect: (pairId: string) => void;
  onSave: (locale: string, runs: Run[]) => Promise<void>;
  onDelete: () => void;
  onAnnotated: () => void;
}

/** The selected pair: both sides in full, the other languages, provenance, and where it is used. */
function PairDetail({
  pair,
  adapter,
  search,
  saving,
  onSelect,
  onSave,
  onDelete,
  onAnnotated,
}: PairDetailProps) {
  const { entry, source, target } = pair;
  const [editing, setEditing] = useState<string | null>(null);
  const [marking, setMarking] = useState(false);
  const others = Object.keys(entry.variants).filter((l) => l !== source && l !== target);
  const sourceSpans = useMemo(
    () => editable(entry.variants[source]?.runs ?? []).spans,
    [entry, source],
  );

  return (
    <ListDetailPane
      data-slot="memory-pair-detail"
      data-entry={entry.id}
      toolbar={
        <ListDetailToolbar
          title={
            <span className="flex min-w-0 items-center gap-1.5">
              <LocaleLabel locale={source} variant="short" />
              {target && (
                <>
                  <ArrowRight aria-hidden className="size-3.5 shrink-0 text-muted-foreground" />
                  <LocaleLabel locale={target} variant="short" />
                </>
              )}
            </span>
          }
          actions={
            <>
              {target && (
                <Button
                  variant="outline"
                  size="sm"
                  data-slot="memory-edit"
                  disabled={saving}
                  onClick={() => setEditing(target)}
                >
                  Edit
                </Button>
              )}
              {adapter.annotateEntities && (
                <Button
                  variant="ghost"
                  size="sm"
                  data-slot="memory-mark-entities"
                  onClick={() => setMarking(true)}
                >
                  Mark entities
                </Button>
              )}
              <ConfirmDeleteButton onDelete={onDelete} mode="text" size="sm" />
            </>
          }
        />
      }
    >
      <VariantBlock
        label={<>Source</>}
        locale={source}
        variant={entry.variants[source]}
        editing={editing === source}
        sourceSpans={[]}
        onEdit={() => setEditing(source)}
        onCancel={() => setEditing(null)}
        onSave={async (runs) => {
          await onSave(source, runs);
          setEditing(null);
        }}
      />
      {target && (
        <VariantBlock
          label={<>Target</>}
          locale={target}
          variant={entry.variants[target]}
          editing={editing === target}
          sourceSpans={sourceSpans}
          onEdit={() => setEditing(target)}
          onCancel={() => setEditing(null)}
          onSave={async (runs) => {
            await onSave(target, runs);
            setEditing(null);
          }}
        />
      )}

      {others.length > 0 && (
        <p
          data-slot="memory-other-languages"
          className="flex flex-wrap items-center gap-1.5 text-xs"
        >
          <span className="text-muted-foreground">Also in</span>
          {others.map((locale) => (
            <button
              key={locale}
              type="button"
              onClick={() => onSelect(`${entry.id}::${locale}`)}
              className="rounded focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <LocaleLabel locale={locale} compact className="hover:underline" />
            </button>
          ))}
        </p>
      )}

      {entry.note && (
        <p className="max-w-prose text-xs text-muted-foreground" translate="no">
          {entry.note}
        </p>
      )}

      <Provenance entry={entry} />
      <WhereUsed entry={entry} adapter={adapter} />

      {adapter.annotateEntities && (
        <EntityAnnotationDialog
          open={marking}
          onClose={() => setMarking(false)}
          selectedCount={1}
          initialPattern={search}
          onApply={async (patterns) => {
            const result = await adapter.annotateEntities!({ entry_ids: [entry.id], patterns });
            onAnnotated();
            return result;
          }}
        />
      )}
    </ListDetailPane>
  );
}

/** One side of the pair in full: its language, its wording, and the editor when open. */
function VariantBlock({
  label,
  locale,
  variant,
  editing,
  sourceSpans,
  onEdit,
  onCancel,
  onSave,
}: {
  label: React.ReactNode;
  locale: string;
  variant?: VariantDTO;
  editing: boolean;
  sourceSpans: SpanInfo[];
  onEdit: () => void;
  onCancel: () => void;
  onSave: (runs: Run[]) => Promise<void>;
}) {
  const initial = useMemo(() => editable(variant?.runs ?? []), [variant]);
  return (
    <section data-slot="memory-variant" data-locale={locale} className="group/variant space-y-1">
      <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
        <span>{label}</span>
        <LocaleLabel locale={locale} compact />
        {!editing && (
          <Button
            variant="ghost"
            size="xs"
            className="ml-auto h-5 px-1.5 text-[11px] text-muted-foreground opacity-0 group-hover/variant:opacity-100 focus-visible:opacity-100"
            onClick={onEdit}
            aria-label={t("Edit the {language} wording", { language: locale })}
          >
            Edit
          </Button>
        )}
      </div>
      {editing ? (
        <InlineCodeEditor
          initialCodedText={initial.codedText || variant?.text || ""}
          initialSpans={initial.spans}
          sourceSpans={sourceSpans}
          onSave={(codedText, spans) => void onSave(codedText ? codedToRuns(codedText, spans) : [])}
          onCancel={onCancel}
          compact
        />
      ) : (
        <CodedTextDisplay
          runs={variant?.runs}
          text={variant?.text}
          locale={locale}
          className="block max-w-prose text-sm leading-relaxed"
        />
      )}
    </section>
  );
}

const ORIGIN_ICONS: Record<string, React.ComponentType<{ className?: string }>> = {
  file: FileText,
  tool: Wrench,
  import: Upload,
  user: User,
};

/** Where an entry came from: each origin with its file, when, and who added it. */
function Provenance({ entry }: { entry: MemoryEntryDTO }) {
  const origins = entry.origins ?? [];
  return (
    <section data-slot="memory-provenance" aria-label={t("Where it came from")}>
      <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Where it came from</h3>
      {origins.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          Added <When iso={entry.created_at} relative />
          {entry.updated_at && entry.updated_at !== entry.created_at && (
            <>
              {", changed "}
              <When iso={entry.updated_at} relative />
            </>
          )}
        </p>
      ) : (
        <ul className="space-y-1.5">
          {origins.map((origin, i) => (
            <OriginLine key={`${origin.source}:${origin.key ?? ""}:${i}`} origin={origin} />
          ))}
        </ul>
      )}
      {entry.project_id && (
        <p data-slot="memory-project-scope" className="mt-1.5 text-[11px] text-muted-foreground">
          Scoped to this project
        </p>
      )}
    </section>
  );
}

function OriginLine({ origin }: { origin: OriginDTO }) {
  const Icon = ORIGIN_ICONS[origin.source] ?? FileText;
  return (
    <li
      data-slot="memory-origin"
      className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-sm"
    >
      <Icon aria-hidden className="size-3.5 shrink-0 text-muted-foreground" />
      {origin.key ? (
        <SimpleTooltip content={origin.key}>
          <span className="min-w-0 truncate font-mono text-xs" translate="no">
            {origin.key}
          </span>
        </SimpleTooltip>
      ) : (
        <span className="text-xs">{originWord(origin.source)}</span>
      )}
      <span className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
        <When iso={origin.added_at} relative />
        {origin.added_by && (
          <>
            <span aria-hidden>·</span>
            <ActorMark
              actor={origin.source === "user" ? "person" : origin.added_by}
              name={origin.added_by}
              size="sm"
              decorative
            />
            <span translate="no">{origin.added_by}</span>
          </>
        )}
      </span>
      {origin.reference && (
        <span
          className="basis-full truncate pl-5 font-mono text-[10px] text-muted-foreground"
          translate="no"
        >
          {origin.reference}
        </span>
      )}
    </li>
  );
}

/** What kind of origin an entry has, in the active UI language. */
function originWord(source: string): string {
  switch (source) {
    case "file":
      return t("From a file", "memory origin");
    case "tool":
      return t("From a tool", "memory origin");
    case "import":
      return t("Imported", "memory origin");
    case "user":
      return t("Added by hand", "memory origin");
    default:
      return source;
  }
}

/** The last segment of a unit id, which is the part that identifies it. */
function shortUnit(unit: string): string {
  const parts = unit.split(/[:/]/);
  return parts[parts.length - 1] || unit;
}

/**
 * The unit an entry was approved for and the point it sits at. A contested
 * entry records its own point; for the rest, a host holding the project's
 * recipe resolves where the unit sits, and one that cannot shows no point.
 */
function WhereUsed({ entry, adapter }: { entry: MemoryEntryDTO; adapter: MemoryAdapter }) {
  const stored = entry.point;
  const [resolved, setResolved] = useState<MemoryPointDTO | null>(null);
  const live = useRef(true);
  useEffect(() => {
    live.current = true;
    setResolved(null);
    if (stored || !entry.unit || !adapter.resolvePoint) return;
    void adapter
      .resolvePoint(entry)
      .then((p) => live.current && setResolved(p))
      .catch(() => live.current && setResolved(null));
    return () => {
      live.current = false;
    };
  }, [entry, stored, adapter]);

  if (!entry.unit) return null;
  const point = stored ?? resolved;
  const rungs: Array<[string, string]> = [];
  if (point?.profile) rungs.push(["profile", point.profile]);
  if (point?.channel) rungs.push(["channel", point.channel]);
  if (point?.collection) rungs.push(["collection", point.collection]);

  return (
    <section data-slot="memory-where-used" aria-label={t("Where it is used")}>
      <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Where it is used</h3>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        {entry.unit &&
          (adapter.openUnit ? (
            <SimpleTooltip content={entry.unit}>
              <Button
                variant="link"
                size="sm"
                data-testid="entry-unit"
                className="h-auto p-0 font-mono text-xs"
                onClick={() => adapter.openUnit!(entry)}
              >
                {shortUnit(entry.unit)}
              </Button>
            </SimpleTooltip>
          ) : (
            <SimpleTooltip content={entry.unit}>
              <span className="font-mono text-xs" data-testid="entry-unit" translate="no">
                {shortUnit(entry.unit)}
              </span>
            </SimpleTooltip>
          ))}
        {rungs.map(([axis, value]) => (
          <SimpleTooltip
            key={axis}
            content={
              stored ? t("This answer was approved at this point") : t("Where this block sits")
            }
          >
            <span>
              <CoordinateChip axis={axis} value={value} />
            </span>
          </SimpleTooltip>
        ))}
      </div>
    </section>
  );
}

/** Add a pair by hand: the source and the target, each with its language. */
function AddEntryDialog({
  open,
  onOpenChange,
  locales,
  defaultSource,
  defaultTarget,
  busy,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  locales: LocaleInfo[];
  defaultSource: string;
  defaultTarget: string;
  busy: boolean;
  onSubmit: (req: { source: string; target: string; srcLocale: string; tgtLocale: string }) => void;
}) {
  const [source, setSource] = useState("");
  const [target, setTarget] = useState("");
  const [srcLocale, setSrcLocale] = useState(defaultSource);
  const [tgtLocale, setTgtLocale] = useState(defaultTarget);
  // The defaults arrive with the store's languages; a choice already made stands.
  useEffect(() => setSrcLocale((v) => v || defaultSource), [defaultSource]);
  useEffect(() => setTgtLocale((v) => v || defaultTarget), [defaultTarget]);
  useEffect(() => {
    if (!open) {
      setSource("");
      setTarget("");
    }
  }, [open]);
  const ready = source.trim() && target.trim() && srcLocale && tgtLocale && srcLocale !== tgtLocale;

  const languageField = (
    id: string,
    value: string,
    onChange: (v: string) => void,
    label: string,
  ) =>
    locales.length > 0 ? (
      <LocaleSelect
        id={id}
        value={value}
        onChange={onChange}
        locales={locales}
        aria-label={label}
      />
    ) : (
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label={label}
        className="font-mono"
      />
    );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg" data-slot="memory-add-dialog">
        <DialogHeader>
          <DialogTitle>Add an entry</DialogTitle>
        </DialogHeader>
        <div className="grid grid-cols-[8rem_minmax(0,1fr)] items-start gap-x-3 gap-y-3">
          <Label htmlFor="memory-add-src-locale" className="pt-2 text-xs text-muted-foreground">
            Source language
          </Label>
          {languageField("memory-add-src-locale", srcLocale, setSrcLocale, t("Source language"))}
          <Label htmlFor="memory-add-source" className="pt-2 text-xs text-muted-foreground">
            Source
          </Label>
          <Textarea
            id="memory-add-source"
            value={source}
            onChange={(e) => setSource(e.target.value)}
            rows={2}
          />
          <Label htmlFor="memory-add-tgt-locale" className="pt-2 text-xs text-muted-foreground">
            Target language
          </Label>
          {languageField("memory-add-tgt-locale", tgtLocale, setTgtLocale, t("Target language"))}
          <Label htmlFor="memory-add-target" className="pt-2 text-xs text-muted-foreground">
            Target
          </Label>
          <Textarea
            id="memory-add-target"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            rows={2}
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            size="sm"
            data-slot="memory-add-confirm"
            disabled={!ready || busy}
            onClick={() => onSubmit({ source, target, srcLocale, tgtLocale })}
          >
            Add entry
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
