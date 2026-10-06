// The conflicts `kapi status` lists, where a person decides them.
//
// Two kinds reach the desktop. An edit another machine made to a translation
// the workspace keeps, which a pull merged and which could not land because
// both machines changed the same block: the workspace holds one wording, the
// other edit another. And wording a person wrote into a kept translation whose
// file has appeared since without it: the file holds one wording, the
// workspace another.
//
// Each block is decided on its own. Every decision that writes is a change set
// sent through the change service, guarded by the revision the conflict shows
// as held, so a block that moved meanwhile is refused as stale with the text it
// holds now. For a translation with a file, the workspace's copy is released
// once the file holds the decided wording.

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { GitMerge, TriangleAlert } from "lucide-react";
import {
  Alert,
  AlertDescription,
  Badge,
  Button,
  Card,
  CardContent,
  Textarea,
} from "@neokapi/ui-primitives";
import type { ChangeResult } from "@neokapi/contract-types";
import { t } from "@neokapi/i18n-react/runtime";

import { api, call } from "../hooks/useApi";
import { tabChanges, type ChangeClient } from "../lib/changes";
import { qk } from "../lib/queryKeys";
import type { KeptConflict, KeptConflictBlock } from "../types/api";
import { EditTextDisplay } from "./edit/EditTextDisplay";

export interface KeptConflictsProps {
  tabID: string;
  /** Pre-loaded for Storybook and tests; read from the project otherwise. */
  conflicts?: KeptConflict[];
  /** The change service; the tab's when absent. */
  client?: ChangeClient;
  /** Drops the workspace's copy of blocks beside their file; the binding when absent. */
  release?: (doc: string, locale: string, blocks: string[]) => Promise<void>;
}

/** The key a block of a conflict is known by on screen. */
function blockKey(c: KeptConflict, b: KeptConflictBlock): string {
  return `${c.kind}|${c.doc}|${c.locale}|${b.block}`;
}

/** Why a change set did not land, as a person reads it. */
function refusalOf(res: ChangeResult): string {
  const op = res.ops.find((o) => o.error);
  if (op?.error?.code === "stale") {
    return t("The wording changed since this conflict was read. It is shown again as it stands.");
  }
  return op?.error?.message ?? t("The change was not applied.");
}

export function KeptConflicts({
  tabID,
  conflicts: propConflicts,
  client: propClient,
  release: propRelease,
}: KeptConflictsProps) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: qk.keptConflicts(tabID),
    queryFn: () => call<KeptConflict[]>("GetKeptConflicts", tabID),
    enabled: !propConflicts && !!tabID,
  });
  const [decided, setDecided] = useState<Set<string>>(new Set());
  const [editing, setEditing] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const conflicts = (propConflicts ?? query.data ?? [])
    .map((c) => ({ ...c, blocks: c.blocks.filter((b) => !decided.has(blockKey(c, b))) }))
    .filter((c) => c.blocks.length > 0);
  if (conflicts.length === 0) return null;

  const client = propClient ?? tabChanges(tabID);
  const release =
    propRelease ??
    (async (doc: string, locale: string, blocks: string[]) => {
      await api.releaseKeptWording(tabID, doc, locale, blocks);
    });

  /**
   * Decide one block: write text when it is given, then, for a translation
   * whose file exists, release the workspace's copy.
   */
  async function decide(c: KeptConflict, b: KeptConflictBlock, text: string | null) {
    const key = blockKey(c, b);
    setBusy(key);
    setErrors((e) => ({ ...e, [key]: "" }));
    try {
      if (text !== null) {
        const res = await client.apply({
          note: t("Decide a conflict"),
          ops: [
            {
              op: "set_content",
              at: { doc: c.doc, block: b.block, edition: c.locale },
              if_match: b.held.rev,
              text,
            },
          ],
        });
        if (!res) return;
        if (res.status !== "applied") {
          setErrors((e) => ({ ...e, [key]: refusalOf(res) }));
          await queryClient.invalidateQueries({ queryKey: qk.keptConflicts(tabID) });
          return;
        }
      }
      if (c.kind === "file") await release(c.doc, c.locale, [b.block]);
      setDecided((d) => new Set(d).add(key));
      setEditing((e) => {
        const next = { ...e };
        delete next[key];
        return next;
      });
      await queryClient.invalidateQueries({ queryKey: qk.projectStatus(tabID) });
    } catch (err) {
      setErrors((e) => ({ ...e, [key]: err instanceof Error ? err.message : String(err) }));
    } finally {
      setBusy(null);
    }
  }

  return (
    <Card data-testid="kept-conflicts">
      <CardContent className="space-y-4 p-4">
        <h2 className="flex items-center gap-1.5 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          <GitMerge size={12} />
          {t("Conflicts to decide")}
          <Badge variant="outline" className="font-normal">
            {conflicts.reduce((n, c) => n + c.blocks.length, 0)}
          </Badge>
        </h2>
        {conflicts.map((c) => (
          <section
            key={`${c.kind}|${c.doc}|${c.locale}|${c.edit ?? c.file ?? ""}`}
            className="space-y-2"
            data-testid="kept-conflict"
            data-kind={c.kind}
          >
            <p className="text-sm">
              {c.kind === "edit"
                ? t(
                    "The {locale} draft of {doc} holds another edit made on another machine, which did not land.",
                    { locale: c.locale, doc: c.doc },
                  )
                : t("The workspace keeps {locale} wording of {doc} that {file} does not hold.", {
                    locale: c.locale,
                    doc: c.doc,
                    file: c.file ?? "",
                  })}
            </p>
            {c.blocks.map((b) => {
              const key = blockKey(c, b);
              const draft = editing[key];
              const heldLabel =
                c.kind === "edit"
                  ? t("In the workspace now")
                  : t("In {file}", { file: c.file ?? "" });
              const otherLabel =
                c.kind === "edit" ? t("The edit that did not land") : t("Kept in the workspace");
              return (
                <div
                  key={key}
                  className="space-y-2 rounded-md border p-3"
                  data-testid="kept-conflict-block"
                >
                  <div className="text-xs text-muted-foreground">
                    <span className="font-mono" translate="no">
                      {b.block}
                    </span>
                    {b.source && (
                      <>
                        {" · "}
                        <EditTextDisplay text={b.source} className="inline text-xs" />
                      </>
                    )}
                  </div>
                  <div className="grid gap-2 sm:grid-cols-2">
                    <Wording
                      label={heldLabel}
                      text={b.held.absent ? null : b.held.text}
                      locale={c.locale}
                      action={
                        c.kind === "edit" ? t("Keep this wording") : t("Keep the file's wording")
                      }
                      disabled={busy !== null}
                      onChoose={() => decide(c, b, c.kind === "edit" ? b.held.text : null)}
                      slot="held"
                    />
                    <Wording
                      label={otherLabel}
                      text={b.other.absent ? null : b.other.text}
                      locale={c.locale}
                      action={
                        c.kind === "edit" ? t("Use this wording") : t("Write it into the file")
                      }
                      disabled={busy !== null || b.other.absent === true}
                      onChoose={() => decide(c, b, b.other.text)}
                      slot="other"
                    />
                  </div>
                  {draft === undefined ? (
                    <Button
                      variant="ghost"
                      size="xs"
                      disabled={busy !== null}
                      onClick={() =>
                        setEditing((e) => ({
                          ...e,
                          [key]: b.held.absent ? b.other.text : b.held.text,
                        }))
                      }
                      data-slot="kept-conflict-write"
                    >
                      {t("Write another wording")}
                    </Button>
                  ) : (
                    <div className="space-y-2">
                      <Textarea
                        value={draft}
                        lang={c.locale}
                        onChange={(ev) => setEditing((e) => ({ ...e, [key]: ev.target.value }))}
                        aria-label={t("New wording")}
                        data-slot="kept-conflict-editor"
                      />
                      <div className="flex gap-2">
                        <Button
                          size="xs"
                          disabled={busy !== null || draft.trim() === ""}
                          onClick={() => decide(c, b, draft)}
                          data-slot="kept-conflict-save"
                        >
                          {t("Save this wording")}
                        </Button>
                        <Button
                          variant="outline"
                          size="xs"
                          disabled={busy !== null}
                          onClick={() =>
                            setEditing((e) => {
                              const next = { ...e };
                              delete next[key];
                              return next;
                            })
                          }
                        >
                          {t("Cancel")}
                        </Button>
                      </div>
                    </div>
                  )}
                  {errors[key] && (
                    <Alert className="border-warning/40" data-slot="kept-conflict-error">
                      <TriangleAlert className="text-warning" aria-hidden />
                      <AlertDescription>{errors[key]}</AlertDescription>
                    </Alert>
                  )}
                </div>
              );
            })}
          </section>
        ))}
      </CardContent>
    </Card>
  );
}

/** One side of a contested block, with the button that chooses it. */
function Wording({
  label,
  text,
  locale,
  action,
  disabled,
  onChoose,
  slot,
}: {
  label: string;
  text: string | null;
  locale: string;
  action: string;
  disabled: boolean;
  onChoose: () => void;
  slot: string;
}) {
  return (
    <div
      className="flex flex-col gap-2 rounded-md bg-muted/40 p-2"
      data-slot={`kept-conflict-${slot}`}
    >
      <span className="text-xs text-muted-foreground">{label}</span>
      {text === null ? (
        <span className="text-sm text-muted-foreground italic">{t("no wording")}</span>
      ) : (
        <EditTextDisplay text={text} locale={locale} />
      )}
      <Button
        variant="outline"
        size="xs"
        className="self-start"
        disabled={disabled}
        onClick={onChoose}
        data-slot={`kept-conflict-choose-${slot}`}
      >
        {action}
      </Button>
    </div>
  );
}
