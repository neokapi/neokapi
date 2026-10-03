import { useEffect, useState, type ReactNode } from "react";
import { Check, Loader2, Undo2 } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
import type { CodeRead } from "@neokapi/contract-types";
import { t } from "@neokapi/i18n-react/runtime";

import { type EditionContent, type EditionEdit, setContentOps } from "../../lib/changes";
import { EditionEditor } from "./EditionEditor";
import { EditTextDisplay } from "./EditTextDisplay";
import { StalePrompt } from "./StalePrompt";
import type { ChangeSender } from "./useChangeSender";

export interface EditionEditPanelProps {
  /** Sends the change sets, and holds a stale refusal for the prompt. */
  sender: ChangeSender;
  /** The edition as read, or null while it loads. */
  content: EditionContent | null;
  /** The language the edition is written in. */
  locale?: string;
  /** The content a translation answers to (its source), for the tag palette. */
  reference?: { text: string; codes?: Readonly<Record<string, CodeRead>> };
  /** Why the edition cannot be edited here; it is shown read-only with the reason. */
  readOnlyReason?: string;
  /** The label of the save button. */
  saveLabel?: string;
  /** The note a saved change set carries into history. */
  note?: string;
  /** Actions beside save and revert (the AI actions on a translation). */
  actions?: ReactNode;
  autoFocus?: boolean;
  compact?: boolean;
  /** Called whenever the person's edits change; none means the editor holds what was read. */
  onEditsChange?: (edits: EditionEdit[]) => void;
  "data-slot"?: string;
}

/**
 * One edition in an editor, with the save that sends it to the change service.
 *
 * Save sends a set_content for each edit with the revision the editor was
 * given, so a change made to the content after it was read is never
 * overwritten unseen: the change service refuses it as stale, and the panel
 * shows the text as it stands and asks before applying the edit over it.
 * Revert starts the editor over from what was read.
 */
export function EditionEditPanel({
  sender,
  content,
  locale,
  reference,
  readOnlyReason,
  saveLabel,
  note,
  actions,
  autoFocus,
  compact,
  onEditsChange,
  "data-slot": dataSlot,
}: EditionEditPanelProps) {
  const [edits, setEdits] = useState<EditionEdit[]>([]);
  const [generation, setGeneration] = useState(0);
  const identity = content
    ? `${content.ref.doc}#${content.ref.block}@${content.ref.edition ?? ""}:${content.rev}`
    : "";

  // A new read (another unit, or the same one after a save) starts over.
  useEffect(() => {
    setEdits([]);
    onEditsChange?.([]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [identity]);

  const changed = edits.length > 0;

  const save = async () => {
    if (!content || !changed) return;
    await sender.send(setContentOps(content, edits), note);
  };

  const revert = () => {
    setGeneration((g) => g + 1);
    setEdits([]);
    onEditsChange?.([]);
    sender.clear();
  };

  // Apply the editor's edits as they stand now over the moved text, with
  // anything typed while the prompt was open. An editor taken back to what
  // was read has no change to apply, so the current text is kept.
  const reapply = async () => {
    if (!content || edits.length === 0) {
      revert();
      await sender.discard();
      return;
    }
    await sender.reapply(setContentOps(content, edits));
  };

  return (
    <div className="space-y-2" data-slot={dataSlot}>
      {!content ? (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 size={12} className="animate-spin" />
          {t("Reading…")}
        </div>
      ) : readOnlyReason ? (
        <div className="space-y-1">
          <EditTextDisplay text={content.text} codes={content.codes} locale={locale} />
          <p className="text-[11px] text-muted-foreground" data-slot="edition-read-only">
            {readOnlyReason}
          </p>
        </div>
      ) : (
        <EditionEditor
          key={`${identity}:${generation}`}
          content={content}
          locale={locale}
          reference={reference}
          autoFocus={autoFocus}
          compact={compact}
          onChange={(next) => {
            setEdits(next);
            onEditsChange?.(next);
          }}
          onSubmit={() => void save()}
          onCancel={revert}
          data-slot={dataSlot ? `${dataSlot}-editor` : undefined}
        />
      )}

      <div className="flex flex-wrap items-center gap-2">
        {changed && (
          <>
            <Button
              size="xs"
              onClick={() => void save()}
              disabled={sender.busy}
              data-slot={dataSlot ? `${dataSlot}-save` : "edition-save"}
            >
              {sender.busy ? <Loader2 size={12} className="animate-spin" /> : <Check size={12} />}
              {saveLabel ?? t("Save")}
            </Button>
            <Button
              variant="outline"
              size="xs"
              onClick={revert}
              disabled={sender.busy}
              data-slot={dataSlot ? `${dataSlot}-revert` : "edition-revert"}
            >
              <Undo2 size={12} />
              {t("Revert")}
            </Button>
          </>
        )}
        {actions}
      </div>

      {sender.stale && (
        <StalePrompt
          current={sender.stale.current}
          codes={content?.codes}
          locale={locale}
          busy={sender.busy}
          onReapply={() => void reapply()}
          onDiscard={() => {
            revert();
            void sender.discard();
          }}
        />
      )}
      {sender.error && (
        <p className="text-xs text-destructive" role="alert" data-slot="edition-refused">
          {sender.error}
        </p>
      )}
    </div>
  );
}
