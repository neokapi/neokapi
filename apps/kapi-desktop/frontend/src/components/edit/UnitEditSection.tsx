import { useCallback, useEffect, useState } from "react";
import { X } from "lucide-react";
import { Button, LocaleLabel } from "@neokapi/ui-primitives";
import type { BlockRead, ChangeResult } from "@neokapi/contract-types";
import { t } from "@neokapi/i18n-react/runtime";

import {
  type ChangeClient,
  type EditionContent,
  editionContent,
  readBlock,
} from "../../lib/changes";
import { EditionEditPanel } from "./EditionEditPanel";
import { useChangeSender } from "./useChangeSender";

export interface UnitEditSectionProps {
  client: ChangeClient;
  /** The document, as a path the change service resolves. */
  doc: string;
  /** The unit's key. */
  unitKey: string;
  /** The edition to edit first: "source", or a target locale key. */
  side?: string;
  /** The editions the reader may switch between: "source" and the target locales. */
  sides?: string[];
  /** Called after an edit landed, so the host draws the document again. */
  onApplied?: (res: ChangeResult) => void | Promise<void>;
  onClose: () => void;
}

/** The edition named by side, matched by key whatever its case. */
function contentOn(read: BlockRead, side: string | undefined): EditionContent | null {
  if (!side || side === "source") return editionContent(read);
  const key = Object.keys(read.editions ?? {}).find((k) => k.toLowerCase() === side.toLowerCase());
  return key ? editionContent(read, key) : null;
}

/**
 * Edit one unit of a document from the document view: its source, or the
 * translation the view shows. It reads the unit through the change service and
 * saves a set_content with the revision it read, as the review pane does; the
 * document view only draws, and this section is the commit.
 */
export function UnitEditSection({
  client,
  doc,
  unitKey,
  side: initialSide,
  sides,
  onApplied,
  onClose,
}: UnitEditSectionProps) {
  const [read, setRead] = useState<BlockRead | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [side, setSide] = useState(initialSide ?? "source");
  const target = side !== "source" ? side : undefined;

  const load = useCallback(async () => {
    setError(null);
    try {
      setRead(await readBlock(client, doc, unitKey, target ? [target] : undefined));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [client, doc, unitKey, target]);
  useEffect(() => {
    setRead(null);
    void load();
  }, [load]);

  const sender = useChangeSender(client, {
    onApplied: async (res) => {
      await load();
      await onApplied?.(res);
    },
    onReload: load,
  });

  const content = read ? contentOn(read, side) : null;

  return (
    <div className="w-full space-y-2 rounded-md border bg-muted/30 p-3" data-slot="unit-edit">
      <div className="flex items-center gap-2 text-xs">
        <span className="font-medium">{t("Edit")}</span>
        <span className="font-mono text-[11px] text-muted-foreground" translate="no">
          {unitKey}
        </span>
        {sides && sides.length > 1 ? (
          <div className="flex flex-wrap items-center gap-1" data-slot="unit-edit-sides">
            {sides.map((s) => (
              <Button
                key={s}
                variant={s === side ? "default" : "outline"}
                size="xs"
                aria-pressed={s === side}
                onClick={() => setSide(s)}
              >
                {s === "source" ? t("Source") : <LocaleLabel locale={s} compact />}
              </Button>
            ))}
          </div>
        ) : target ? (
          <LocaleLabel locale={target} compact />
        ) : (
          <span className="text-muted-foreground">{t("Source")}</span>
        )}
        <Button
          variant="ghost"
          size="icon-xs"
          className="ml-auto"
          onClick={onClose}
          aria-label={t("Close the editor")}
          data-slot="unit-edit-close"
        >
          <X size={12} />
        </Button>
      </div>
      {error ? (
        <p className="text-xs text-destructive" role="alert">
          {error}
        </p>
      ) : read && !content ? (
        <p className="text-xs text-muted-foreground" data-slot="unit-edit-no-edition">
          {t("This block has no translation in this language yet.")}
        </p>
      ) : (
        <EditionEditPanel
          sender={sender}
          content={content}
          locale={target}
          reference={target && read ? { text: read.text, codes: read.codes } : undefined}
          readOnlyReason={
            read && !read.ops.includes("set_content")
              ? t("This format does not write a change to this text.")
              : undefined
          }
          autoFocus
          compact
          data-slot="unit-edit"
        />
      )}
    </div>
  );
}
