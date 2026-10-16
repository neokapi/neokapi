// The detail pane of the Content memory page: the selected pair, or what can
// be done to every entry marked in the list, and the dialog that adds a pair.
//
// A pair shows its source and its target in full, then where it came from: the
// project it is held for or the whole workspace, its stream when it is not the
// main line, and when it last changed. Edit opens both sides for editing in
// place; Delete asks once more before it removes the entry.
import { useEffect, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  ConfirmDeleteButton,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Label,
  ListDetailPane,
  ListDetailToolbar,
  LocaleLabel,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Textarea,
  When,
} from "@neokapi/ui-primitives";
import { ArrowRight, Pencil } from "../../components/icons";
import { ErrorNotice } from "../../errors";
import type { MemoryEntryInfo } from "../../types/api";
import { entryStream } from "./memoryRows";

/** The two sides of a pair, as the editor hands them back. */
export interface PairEdit {
  source: string;
  target: string;
}

export interface PairDetailProps {
  entry: MemoryEntryInfo;
  /** The project the entry is held for, by name, when it is held for one. */
  projectName?: string;
  saving?: boolean;
  /** Why the last save or delete failed, when it did. */
  error?: unknown;
  onSave: (edit: PairEdit) => Promise<unknown>;
  onDelete: () => void;
}

/** The selected pair: both sides in full, where it came from, and its acts in the toolbar. */
export function PairDetail({
  entry,
  projectName,
  saving,
  error,
  onSave,
  onDelete,
}: PairDetailProps) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<PairEdit>({ source: entry.source, target: entry.target });
  useEffect(() => {
    setDraft({ source: entry.source, target: entry.target });
  }, [entry.source, entry.target]);
  const stream = entryStream(entry);
  const changed = draft.source !== entry.source || draft.target !== entry.target;

  const save = async () => {
    await onSave(draft);
    setEditing(false);
  };

  return (
    <ListDetailPane
      data-slot="memory-pair-detail"
      data-entry={entry.id}
      toolbar={
        <ListDetailToolbar
          title={
            <span className="flex min-w-0 items-center gap-1.5">
              <LocaleLabel locale={entry.source_language} variant="short" />
              {entry.target_language && (
                <>
                  <ArrowRight aria-hidden className="size-3.5 shrink-0 text-muted-foreground" />
                  <LocaleLabel locale={entry.target_language} variant="short" />
                </>
              )}
            </span>
          }
          actions={
            <>
              {!editing && (
                <Button
                  variant="outline"
                  size="sm"
                  data-slot="memory-edit"
                  disabled={saving}
                  onClick={() => setEditing(true)}
                >
                  <Pencil size={13} />
                  Edit
                </Button>
              )}
              <ConfirmDeleteButton onDelete={onDelete} mode="text" size="sm" />
            </>
          }
        />
      }
    >
      {error ? (
        <ErrorNotice
          variant="inline"
          error={error}
          title={t("The entry was not changed")}
          hint={t("Changing content memory needs permission to edit it in this workspace.")}
        />
      ) : null}

      {editing ? (
        <div data-slot="memory-pair-editor" className="max-w-prose space-y-3">
          <Side label={<>Source</>} locale={entry.source_language}>
            <Textarea
              value={draft.source}
              onChange={(e) => setDraft((d) => ({ ...d, source: e.target.value }))}
              rows={3}
              translate="no"
              aria-label={t("Source")}
              data-slot="memory-edit-source"
            />
          </Side>
          <Side label={<>Target</>} locale={entry.target_language}>
            <Textarea
              value={draft.target}
              onChange={(e) => setDraft((d) => ({ ...d, target: e.target.value }))}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  e.preventDefault();
                  setDraft({ source: entry.source, target: entry.target });
                  setEditing(false);
                }
              }}
              rows={3}
              translate="no"
              aria-label={t("Target")}
              data-slot="memory-edit-target"
              autoFocus
            />
          </Side>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="secondary"
              data-slot="memory-save"
              disabled={saving || !changed || !draft.source.trim() || !draft.target.trim()}
              onClick={() => void save()}
            >
              Save
            </Button>
            <Button
              variant="ghost"
              size="sm"
              aria-keyshortcuts="Escape"
              onClick={() => {
                setDraft({ source: entry.source, target: entry.target });
                setEditing(false);
              }}
            >
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <>
          <Side label={<>Source</>} locale={entry.source_language}>
            <p
              data-slot="memory-source"
              className="max-w-prose text-sm leading-relaxed whitespace-pre-wrap"
              translate="no"
            >
              {entry.source}
            </p>
          </Side>
          {entry.target_language && (
            <Side label={<>Target</>} locale={entry.target_language}>
              <p
                data-slot="memory-target"
                className="max-w-prose text-sm leading-relaxed whitespace-pre-wrap"
                translate="no"
              >
                {entry.target}
              </p>
            </Side>
          )}
        </>
      )}

      <section data-slot="memory-provenance" aria-label={t("Where it came from")}>
        <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Where it came from</h3>
        <dl className="grid grid-cols-[minmax(7rem,max-content)_minmax(0,1fr)] gap-x-4 gap-y-1 text-sm">
          <dt className="text-[11px] text-muted-foreground">Held for</dt>
          <dd data-slot="memory-scope" className="min-w-0 break-words">
            {entry.project_id ? (
              <span translate="no">{projectName ?? entry.project_id}</span>
            ) : (
              <>Every project in the workspace</>
            )}
          </dd>
          {stream && (
            <>
              <dt className="text-[11px] text-muted-foreground">Stream</dt>
              <dd className="min-w-0 font-mono text-xs" translate="no">
                {stream}
              </dd>
            </>
          )}
          {entry.updated_at && (
            <>
              <dt className="text-[11px] text-muted-foreground">Changed</dt>
              <dd className="min-w-0">
                <When iso={entry.updated_at} relative />
              </dd>
            </>
          )}
        </dl>
      </section>
    </ListDetailPane>
  );
}

/** One side of a pair: its label and language above its wording. */
function Side({
  label,
  locale,
  children,
}: {
  label: React.ReactNode;
  locale: string;
  children: React.ReactNode;
}) {
  return (
    <section data-slot="memory-variant" data-locale={locale} className="space-y-1">
      <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
        <span>{label}</span>
        {locale && <LocaleLabel locale={locale} compact />}
      </div>
      {children}
    </section>
  );
}

export interface BulkDetailProps {
  entries: MemoryEntryInfo[];
  deleting?: boolean;
  /** Why the last delete failed, when it did. */
  error?: unknown;
  onDelete: () => void;
  onClear: () => void;
}

/** What can be done to every entry marked in the list. */
export function BulkDetail({ entries, deleting, error, onDelete, onClear }: BulkDetailProps) {
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
              <Button variant="ghost" size="sm" data-slot="memory-bulk-clear" onClick={onClear}>
                Clear
              </Button>
              {!deleting && <ConfirmDeleteButton onDelete={onDelete} mode="text" size="sm" />}
            </>
          }
        />
      }
    >
      {error ? (
        <ErrorNotice variant="inline" error={error} title={t("Not every entry was deleted")} />
      ) : null}
      <ul data-slot="memory-bulk-entries" className="space-y-1">
        {shown.map((entry) => (
          <li key={entry.id} className="truncate text-sm" translate="no">
            {entry.source}
          </li>
        ))}
      </ul>
      {entries.length > shown.length && (
        <p className="text-[11px] text-muted-foreground">
          {t("and {count} more", { count: entries.length - shown.length })}
        </p>
      )}
    </ListDetailPane>
  );
}

/** The fields of a pair added by hand. */
export interface NewPair {
  source: string;
  target: string;
  sourceLocale: string;
  targetLocale: string;
}

export interface AddEntryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The languages to choose from: the workspace's. */
  languages: string[];
  defaultSource: string;
  defaultTarget: string;
  busy?: boolean;
  error?: unknown;
  onSubmit: (pair: NewPair) => void;
}

/** Add a pair by hand: the source and the target, each with its language. */
export function AddEntryDialog({
  open,
  onOpenChange,
  languages,
  defaultSource,
  defaultTarget,
  busy,
  error,
  onSubmit,
}: AddEntryDialogProps) {
  const [source, setSource] = useState("");
  const [target, setTarget] = useState("");
  const [sourceLocale, setSourceLocale] = useState(defaultSource);
  const [targetLocale, setTargetLocale] = useState(defaultTarget);
  // The defaults arrive with the workspace's projects; a choice already made stands.
  useEffect(() => setSourceLocale((v) => v || defaultSource), [defaultSource]);
  useEffect(() => setTargetLocale((v) => v || defaultTarget), [defaultTarget]);
  useEffect(() => {
    if (!open) {
      setSource("");
      setTarget("");
    }
  }, [open]);
  const ready =
    source.trim() && target.trim() && sourceLocale && targetLocale && sourceLocale !== targetLocale;

  const languageField = (
    id: string,
    value: string,
    onChange: (v: string) => void,
    label: string,
  ) => (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger id={id} aria-label={label} className="w-full">
        <SelectValue placeholder={t("Choose a language")} />
      </SelectTrigger>
      <SelectContent>
        {languages.map((code) => (
          <SelectItem key={code} value={code}>
            <LocaleLabel locale={code} />
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
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
          {languageField(
            "memory-add-src-locale",
            sourceLocale,
            setSourceLocale,
            t("Source language"),
          )}
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
          {languageField(
            "memory-add-tgt-locale",
            targetLocale,
            setTargetLocale,
            t("Target language"),
          )}
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
        {error ? (
          <ErrorNotice variant="inline" error={error} title={t("The entry was not added")} />
        ) : null}
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            size="sm"
            data-slot="memory-add-confirm"
            disabled={!ready || busy}
            onClick={() => onSubmit({ source, target, sourceLocale, targetLocale })}
          >
            Add entry
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
