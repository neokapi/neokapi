// The detail pane of the Content memory list.
//
// A row shows its source and the wording in each of its languages in full,
// with the inline codes as chips. When several entries share the source
// wording, each one's languages, provenance and use follow the source in
// turn. A match from Find similar adds its score above the wording. Marked
// entries replace the row with what can be done to all of them at once.

import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { FileText, Upload, User, Wrench } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import type { Run } from "@neokapi/kapi-format";
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
  EntityAnnotationDialog,
  InlineCodeEditor,
  Input,
  Label,
  ListDetailPane,
  ListDetailToolbar,
  LocaleLabel,
  LocaleSelect,
  MatchScoreBar,
  Separator,
  SimpleTooltip,
  Textarea,
  When,
  codedToRuns,
  type LocaleInfo,
  type MemoryAdapter,
  type MemoryEntryDTO,
  type MemoryMatchDTO,
  type MemoryPointDTO,
  type OriginDTO,
  type SpanInfo,
  type VariantDTO,
} from "@neokapi/ui-primitives";
import { editable, sourceLocaleOf, type MemoryRow } from "../lib/memoryPairs";

export interface RowDetailProps {
  row: MemoryRow;
  adapter: MemoryAdapter;
  /** The search in the list, the pattern Mark entities starts from. */
  search: string;
  /** The lookup match this row came from, in Find similar. */
  match?: MemoryMatchDTO;
  /** Saves one language of one entry. */
  onSave: (entry: MemoryEntryDTO, locale: string, runs: Run[]) => Promise<void>;
  /** Deletes every entry in the row. */
  onDelete: () => void;
  onAnnotated: () => void;
}

/** The selected row: the source in full, each language's wording, provenance, and where it is used. */
export function RowDetail({
  row,
  adapter,
  search,
  match,
  onSave,
  onDelete,
  onAnnotated,
}: RowDetailProps) {
  const { source, entries, targets } = row;
  const first = entries[0];
  // An editor is open on one language of one entry at a time.
  const [editing, setEditing] = useState<string | null>(null);
  const [marking, setMarking] = useState(false);
  const sourceSpans = useMemo(
    () => editable(first?.variants[source]?.runs ?? []).spans,
    [first, source],
  );
  const save = async (entry: MemoryEntryDTO, locale: string, runs: Run[]) => {
    await onSave(entry, locale, runs);
    setEditing(null);
  };

  return (
    <ListDetailPane
      data-slot="memory-pair-detail"
      data-entry={first?.id}
      toolbar={
        <ListDetailToolbar
          title={
            <span className="flex min-w-0 items-center gap-1.5">
              <LocaleLabel locale={source} variant="short" />
              {targets.length > 0 && (
                <span className="text-xs font-normal text-muted-foreground tabular-nums">
                  {t("{count, plural, one {# language} other {# languages}}", {
                    count: targets.length,
                  })}
                </span>
              )}
            </span>
          }
          actions={
            <>
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
      {match && <MatchPanel match={match} />}
      {first && (
        <VariantBlock
          label={<>Source</>}
          locale={source}
          variant={first.variants[source]}
          editing={editing === `${first.id}::${source}`}
          sourceSpans={[]}
          onEdit={() => setEditing(`${first.id}::${source}`)}
          onCancel={() => setEditing(null)}
          onSave={async (runs) => {
            // The row is one wording: a change to it is a change to every entry.
            for (const entry of entries) await onSave(entry, source, runs);
            setEditing(null);
          }}
        />
      )}

      {entries.map((entry, i) => (
        <Fragment key={entry.id}>
          {i > 0 && <Separator />}
          {targets
            .filter((locale) => entry.variants[locale])
            .map((locale) => (
              <VariantBlock
                key={locale}
                locale={locale}
                variant={entry.variants[locale]}
                editing={editing === `${entry.id}::${locale}`}
                sourceSpans={sourceSpans}
                onEdit={() => setEditing(`${entry.id}::${locale}`)}
                onCancel={() => setEditing(null)}
                onSave={(runs) => save(entry, locale, runs)}
              />
            ))}

          {entry.note && (
            <p className="max-w-prose text-xs text-muted-foreground" translate="no">
              {entry.note}
            </p>
          )}

          <Provenance entry={entry} />
          <WhereUsed entry={entry} adapter={adapter} />
        </Fragment>
      ))}

      {adapter.annotateEntities && (
        <EntityAnnotationDialog
          open={marking}
          onClose={() => setMarking(false)}
          selectedCount={entries.length}
          initialPattern={search}
          onApply={async (patterns) => {
            const result = await adapter.annotateEntities!({
              entry_ids: entries.map((e) => e.id),
              patterns,
            });
            onAnnotated();
            return result;
          }}
        />
      )}
    </ListDetailPane>
  );
}

/** One language of the row in full: its language, its wording, and the editor when open. */
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
  /** Names the source; a translation is named by its language alone. */
  label?: React.ReactNode;
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
        {label && <span>{label}</span>}
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
export function AddEntryDialog({
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

  const languageField = (id: string, value: string, onChange: (v: string) => void, label: string) =>
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

/** How well a Find similar match answered, and the names it adapts. */
function MatchPanel({ match }: { match: MemoryMatchDTO }) {
  const adaptations = match.entity_adaptations ?? [];
  return (
    <section data-slot="memory-match" aria-label={t("How well it matches")} className="space-y-1.5">
      <MatchScoreBar score={match.score} matchType={match.match_type} className="max-w-xs" />
      {adaptations.length > 0 && (
        <ul className="space-y-0.5 text-xs text-muted-foreground">
          {adaptations.map((a) => (
            <li key={a.placeholder_id}>
              <span className="text-foreground" translate="no">
                {a.current_value}
              </span>{" "}
              takes the place of{" "}
              <span className="text-foreground" translate="no">
                {a.stored_value}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

export interface BulkDetailProps {
  /** The marked entries, in the order the list shows them. */
  entries: MemoryEntryDTO[];
  adapter: MemoryAdapter;
  /** The search in the list, the pattern Mark entities starts from. */
  search: string;
  deleting: boolean;
  onDelete: () => void;
  onClear: () => void;
  onAnnotated: () => void;
}

/** What can be done to every marked entry at once: delete them, or mark the names in them. */
export function BulkDetail({
  entries,
  adapter,
  search,
  deleting,
  onDelete,
  onClear,
  onAnnotated,
}: BulkDetailProps) {
  const [marking, setMarking] = useState(false);
  const shown = entries.slice(0, 50);
  return (
    <ListDetailPane
      data-slot="memory-bulk-detail"
      toolbar={
        <ListDetailToolbar
          title={t("{count, plural, one {# entry marked} other {# entries marked}}", {
            count: entries.length,
          })}
          actions={
            <>
              {adapter.annotateEntities && (
                <Button
                  variant="ghost"
                  size="sm"
                  data-slot="memory-bulk-mark-entities"
                  onClick={() => setMarking(true)}
                >
                  Mark entities
                </Button>
              )}
              <Button variant="ghost" size="sm" data-slot="memory-bulk-clear" onClick={onClear}>
                Clear
              </Button>
              {!deleting && <ConfirmDeleteButton onDelete={onDelete} mode="text" size="sm" />}
            </>
          }
        />
      }
    >
      <ul data-slot="memory-bulk-entries" className="space-y-1">
        {shown.map((entry) => {
          const source = sourceLocaleOf(entry);
          const variant = entry.variants[source];
          return (
            <li key={entry.id} className="truncate text-sm">
              <CodedTextDisplay
                runs={variant?.runs}
                text={variant?.text}
                locale={source}
                className="[&_*]:whitespace-nowrap"
              />
            </li>
          );
        })}
      </ul>
      {entries.length > shown.length && (
        <p className="text-[11px] text-muted-foreground">
          {t("and {count} more", {
            count: entries.length - shown.length,
          })}
        </p>
      )}
      {adapter.annotateEntities && (
        <EntityAnnotationDialog
          open={marking}
          onClose={() => setMarking(false)}
          selectedCount={entries.length}
          initialPattern={search}
          onApply={async (patterns) => {
            const result = await adapter.annotateEntities!({
              entry_ids: entries.map((e) => e.id),
              patterns,
            });
            onAnnotated();
            return result;
          }}
        />
      )}
    </ListDetailPane>
  );
}
