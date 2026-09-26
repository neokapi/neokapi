// What kapi learned about how a project writes since the person last looked.
//
// The digest is news, never a queue. The few things that need a person come
// first (conflicts), then the rules that came into force and on what, then
// the suggestions grouped by theme, then content drifting away from a rule,
// and last the project in numbers. A suggestion nobody answers keeps
// advising, so nothing here counts unread work and nothing has to be cleared:
// what the person has already seen stays, under "Earlier".
//
// The keys act on the item under the cursor: j and k move, a keeps (or
// chooses a side of a conflict), c changes the rule before keeping it, d
// drops, g keeps the whole group, u reverts a rule in force, o opens the file
// the item was seen in.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, FileText, Undo2 } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button, ErrorNotice, Input, LoadingSpinner, cn } from "@neokapi/ui-primitives";
import type { ContextDigest, DigestItem, DigestTermRule } from "../types/api";

export interface ContextDigestViewProps {
  /** The digest, null while the first read is in flight. */
  digest: ContextDigest | null;
  loading?: boolean;
  error?: unknown;
  /**
   * When the person last looked, as it stood when they arrived. Absent when
   * they never had.
   */
  since?: string;
  /** Decisions can be written: a checkout of the project is on this machine. */
  canDecide?: boolean;
  /** Take the keyboard. */
  keyboard?: boolean;
  /** Keep a suggestion, changing what it says to write when a form is given. */
  onKeep: (item: DigestItem, replacement?: string) => void | Promise<void>;
  /** Keep every suggestion of one group. */
  onKeepGroup: (items: DigestItem[]) => void | Promise<void>;
  onDrop: (item: DigestItem) => void | Promise<void>;
  /** Choose one side of a conflict, setting the others aside. */
  onChoose: (item: DigestItem, replacement?: string) => void | Promise<void>;
  onRevert: (item: DigestItem) => void;
  /** Open the file an item was seen in. Absent hides the links. */
  onOpenFile?: (path: string) => void;
}

/** One item the cursor can stand on, with what its keys do. */
interface Stop {
  item: DigestItem;
  /** "conflict", "established", "suggested" or "drift". */
  section: string;
  group?: DigestItem[];
}

export function ContextDigestView({
  digest,
  loading,
  error,
  since,
  canDecide = true,
  keyboard = true,
  onKeep,
  onKeepGroup,
  onDrop,
  onChoose,
  onRevert,
  onOpenFile,
}: ContextDigestViewProps) {
  const stops = useMemo<Stop[]>(() => {
    if (!digest) return [];
    const out: Stop[] = [];
    for (const c of digest.conflicts)
      for (const side of c.sides) out.push({ item: side, section: "conflict" });
    for (const it of digest.established) out.push({ item: it, section: "established" });
    for (const theme of digest.suggested)
      for (const g of theme.groups)
        for (const it of g.items) out.push({ item: it, section: "suggested", group: g.items });
    for (const d of digest.drift) out.push({ item: d.rule, section: "drift" });
    return out;
  }, [digest]);

  const [cursor, setCursor] = useState(0);
  const [changing, setChanging] = useState<{ id: string; form: string } | null>(null);
  const active = stops[Math.min(cursor, stops.length - 1)];

  const keep = useCallback(
    (stop: Stop, replacement?: string) => {
      setChanging(null);
      return stop.section === "conflict"
        ? onChoose(stop.item, replacement)
        : onKeep(stop.item, replacement);
    },
    [onChoose, onKeep],
  );

  useEffect(() => {
    if (!keyboard) return;
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName ?? "";
      if (tag === "TEXTAREA" || tag === "INPUT" || tag === "SELECT") {
        if (e.key === "Escape") {
          el?.blur();
          setChanging(null);
        }
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey || !active) return;
      const { item } = active;
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          setCursor((at) => Math.min(at + 1, stops.length - 1));
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          setCursor((at) => Math.max(at - 1, 0));
          break;
        case "a":
          if (canDecide && item.keepable) {
            e.preventDefault();
            void keep(active);
          }
          break;
        case "c":
          if (canDecide && item.keepable && item.subject.term) {
            e.preventDefault();
            setChanging({ id: item.id, form: item.subject.term.replacement ?? "" });
          }
          break;
        case "d":
          if (canDecide && item.droppable) {
            e.preventDefault();
            void onDrop(item);
          }
          break;
        case "g":
          if (canDecide && active.group) {
            e.preventDefault();
            void onKeepGroup(active.group.filter((g) => g.keepable));
          }
          break;
        case "u":
          if (canDecide && item.revertible) {
            e.preventDefault();
            onRevert(item);
          }
          break;
        case "o":
          if (onOpenFile && item.quote?.path) {
            e.preventDefault();
            onOpenFile(item.quote.path);
          }
          break;
        case "Escape":
          setChanging(null);
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, canDecide, keep, keyboard, onDrop, onKeepGroup, onOpenFile, onRevert, stops.length]);

  if (error) {
    return <ErrorNotice error={error} title="The digest could not be read" />;
  }
  if (!digest) {
    return loading ? <LoadingSpinner /> : null;
  }

  const quiet =
    digest.numbers.new === 0 && digest.conflicts.length === 0 && digest.drift.length === 0;
  const row = (stop: Stop) => (
    <DigestRow
      key={stop.item.id}
      stop={stop}
      active={active?.item.id === stop.item.id}
      canDecide={canDecide}
      changing={changing?.id === stop.item.id ? changing.form : null}
      onFocus={() => setCursor(stops.indexOf(stop))}
      onChangeForm={(form) => setChanging({ id: stop.item.id, form })}
      onStartChange={() =>
        setChanging({ id: stop.item.id, form: stop.item.subject.term?.replacement ?? "" })
      }
      onCancelChange={() => setChanging(null)}
      onKeep={(replacement) => void keep(stop, replacement)}
      onDrop={() => void onDrop(stop.item)}
      onRevert={() => onRevert(stop.item)}
      onOpenFile={onOpenFile}
    />
  );
  const stopsOf = (section: string) => stops.filter((s) => s.section === section);

  return (
    <div data-slot="context-digest" className="space-y-8">
      <DigestHeading since={since} quiet={quiet} />
      {stops.length === 0 && digest.numbers.rules === 0 && (
        <p data-slot="digest-empty" className="text-sm text-muted-foreground">
          Nothing is recorded yet. When you or an agent working here notice a name, a word the
          project avoids or how it addresses its readers, it appears here, and it advises agents and
          checks from the moment it is recorded.
        </p>
      )}

      {digest.conflicts.length > 0 && (
        <section data-slot="digest-conflicts" aria-labelledby="digest-conflicts">
          <h3 id="digest-conflicts" className="text-base font-semibold">
            Needs you
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">
            These rules disagree. Choose the one the project goes by, and the others are set aside.
          </p>
          <div className="mt-3 space-y-4">
            {digest.conflicts.map((c) => (
              <div
                key={c.sides[0]?.id}
                className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3"
              >
                <p className="text-xs text-muted-foreground">{c.reason}</p>
                <div className="mt-2 divide-y divide-border/60">
                  {stopsOf("conflict")
                    .filter((s) => c.sides.some((side) => side.id === s.item.id))
                    .map(row)}
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {digest.established.length > 0 && (
        <section data-slot="digest-established" aria-labelledby="digest-established">
          <h3 id="digest-established" className="text-base font-semibold">
            Established
          </h3>
          <NewThenEarlier stops={stopsOf("established")} row={row} />
        </section>
      )}

      {digest.suggested.length > 0 && (
        <section data-slot="digest-suggested" aria-labelledby="digest-suggested">
          <h3 id="digest-suggested" className="text-base font-semibold">
            Suggested
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Each suggestion already advises agents and checks. Keeping one makes it a rule that
            checks enforce.
          </p>
          {digest.suggested.map((theme) => (
            <div key={theme.theme} data-slot="digest-theme" className="mt-4">
              <ThemeTitle theme={theme.theme} />
              {theme.groups.map((g) => (
                <div key={g.collection ?? ""} data-slot="digest-group" className="mt-2">
                  <div className="flex items-center justify-between gap-2">
                    {g.collection ? (
                      <p className="text-xs text-muted-foreground">
                        In <span className="font-medium text-foreground">{g.collection}</span>
                      </p>
                    ) : (
                      <span />
                    )}
                    {canDecide && g.items.filter((i) => i.keepable).length > 1 && (
                      <Button
                        variant="ghost"
                        size="sm"
                        data-slot="keep-group"
                        onClick={() => void onKeepGroup(g.items.filter((i) => i.keepable))}
                      >
                        <Check size={13} />
                        Keep all {g.items.filter((i) => i.keepable).length}
                        <kbd className="ml-1 rounded border border-border/60 px-1 text-[10px]">
                          g
                        </kbd>
                      </Button>
                    )}
                  </div>
                  <NewThenEarlier
                    stops={stopsOf("suggested").filter((s) => s.group === g.items)}
                    row={row}
                  />
                </div>
              ))}
            </div>
          ))}
        </section>
      )}

      {digest.drift.length > 0 && (
        <section data-slot="digest-drift" aria-labelledby="digest-drift">
          <h3 id="digest-drift" className="text-base font-semibold">
            Drift
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Content is moving away from these rules. Open the files, or revert the rule if the
            project has changed its mind.
          </p>
          <div className="mt-2 divide-y divide-border/60">{stopsOf("drift").map(row)}</div>
        </section>
      )}

      <DigestNumbers digest={digest} />
    </div>
  );
}

/** The line that says what "new" is measured from. */
function DigestHeading({ since, quiet }: { since?: string; quiet: boolean }) {
  if (!since) {
    return (
      <p className="text-sm text-muted-foreground">
        Everything kapi has learned about how this project writes.
      </p>
    );
  }
  const when = formatLook(since);
  return quiet ? (
    <p data-slot="digest-quiet" className="text-sm">
      Nothing new since {when}.
    </p>
  ) : (
    <p className="text-sm text-muted-foreground">Since you last looked, {when}.</p>
  );
}

/** A theme's title, written here so it is translated with the rest. */
function ThemeTitle({ theme }: { theme: string }) {
  const cls = "text-sm font-medium";
  switch (theme) {
    case "names":
      return <h4 className={cls}>Names and spellings</h4>;
    case "words":
      return <h4 className={cls}>Words to avoid</h4>;
    case "writing":
      return <h4 className={cls}>How the project writes</h4>;
    case "memory":
      return <h4 className={cls}>Wording in other languages</h4>;
  }
  return <h4 className={cls}>{theme}</h4>;
}

/** The new items, then the ones the person has already seen under "Earlier". */
function NewThenEarlier({ stops, row }: { stops: Stop[]; row: (s: Stop) => React.ReactNode }) {
  const fresh = stops.filter((s) => s.item.new);
  const seen = stops.filter((s) => !s.item.new);
  return (
    <>
      {fresh.length > 0 && <div className="mt-2 divide-y divide-border/60">{fresh.map(row)}</div>}
      {seen.length > 0 && (
        <div data-slot="digest-earlier" className="mt-3">
          <p className="text-xs text-muted-foreground">Earlier</p>
          <div className="mt-1 divide-y divide-border/60 opacity-80">{seen.map(row)}</div>
        </div>
      )}
    </>
  );
}

/** The project in numbers. */
function DigestNumbers({ digest }: { digest: ContextDigest }) {
  const name = digest.project_name || digest.project;
  const { rules, new_this_week: week, suggested } = digest.numbers;
  return (
    <p
      data-slot="digest-numbers"
      className="border-t border-border pt-3 text-sm text-muted-foreground"
    >
      {t("kapi knows {rules, plural, one {# rule} other {# rules}} for {name}", { rules, name })}
      {week > 0 && t("; {week, plural, one {# is} other {# are}} new this week", { week })}.{" "}
      {suggested > 0 &&
        t("{suggested, plural, one {# suggestion is} other {# suggestions are}} advising.", {
          suggested,
        })}
    </p>
  );
}

/** One item: the rule as a sentence, where it was seen, and what backs it. */
function DigestRow({
  stop,
  active,
  canDecide,
  changing,
  onFocus,
  onChangeForm,
  onStartChange,
  onCancelChange,
  onKeep,
  onDrop,
  onRevert,
  onOpenFile,
}: {
  stop: Stop;
  active: boolean;
  canDecide: boolean;
  changing: string | null;
  onFocus: () => void;
  onChangeForm: (form: string) => void;
  onStartChange: () => void;
  onCancelChange: () => void;
  onKeep: (replacement?: string) => void;
  onDrop: () => void;
  onRevert: () => void;
  onOpenFile?: (path: string) => void;
}) {
  const { item, section } = stop;
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (changing !== null) inputRef.current?.focus();
  }, [changing]);
  const kbd = (key: string) =>
    active ? (
      <kbd className="ml-1 rounded border border-border/60 px-1 text-[10px]">{key}</kbd>
    ) : null;

  return (
    <div
      data-slot="digest-item"
      data-active={active || undefined}
      onClick={onFocus}
      className={cn("relative py-2.5 pl-4 pr-1", active && "rounded-md bg-accent/50")}
    >
      {item.new && (
        <span
          aria-label={t("New since you last looked")}
          className="absolute left-1 top-4 size-1.5 rounded-full bg-primary"
        />
      )}
      <p className="text-sm leading-snug">
        <RuleSentence item={item} />
      </p>
      {item.quote && (item.quote.quote || item.quote.path) && (
        <p className="mt-1 text-xs text-muted-foreground">
          {item.quote.quote && (
            <q className="italic" translate="no">
              {item.quote.quote}
            </q>
          )}{" "}
          {item.quote.path &&
            (onOpenFile ? (
              <button
                type="button"
                data-slot="open-file"
                className="inline-flex items-center gap-1 text-foreground underline-offset-2 hover:underline"
                onClick={(e) => {
                  e.stopPropagation();
                  onOpenFile(item.quote!.path!);
                }}
              >
                <FileText size={11} />
                <span translate="no">{item.quote.path}</span>
                {kbd("o")}
              </button>
            ) : (
              <span className="font-mono" translate="no">
                {item.quote.path}
              </span>
            ))}
        </p>
      )}
      {item.usage && (
        <p data-slot="digest-usage" className="mt-1 text-xs">
          {item.usage.line}
        </p>
      )}
      <p className="mt-1 text-xs text-muted-foreground">
        {item.how && item.how.length > 0 ? item.how.join(" · ") : <Noticed item={item} />}
        {item.standing && <> · {item.standing}</>}
      </p>

      {canDecide && changing !== null && (
        <div data-slot="digest-change" className="mt-2 flex items-center gap-2">
          <Input
            ref={inputRef}
            value={changing}
            onChange={(e) => onChangeForm(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                onKeep(changing);
              }
            }}
            className="h-8 max-w-64 font-mono"
            translate="no"
            aria-label={t("What to write instead")}
          />
          <Button size="sm" data-slot="keep-changed" onClick={() => onKeep(changing)}>
            <Check size={13} />
            Keep with this form
          </Button>
          <Button variant="ghost" size="sm" onClick={onCancelChange}>
            Cancel
          </Button>
        </div>
      )}

      {canDecide && changing === null && (
        <div data-slot="digest-actions" className="mt-2 flex flex-wrap items-center gap-2">
          {item.keepable && (
            <Button size="sm" data-slot="keep-item" onClick={() => onKeep()}>
              <Check size={13} />
              {section === "conflict" ? <>Choose this</> : <>Keep</>}
              {kbd("a")}
            </Button>
          )}
          {item.keepable && item.subject.term && (
            <Button variant="outline" size="sm" data-slot="change-item" onClick={onStartChange}>
              Change
              {kbd("c")}
            </Button>
          )}
          {item.droppable && section !== "conflict" && (
            <Button variant="ghost" size="sm" data-slot="drop-item" onClick={onDrop}>
              Drop
              {kbd("d")}
            </Button>
          )}
          {item.revertible && section !== "conflict" && (
            <Button variant="ghost" size="sm" data-slot="revert-item" onClick={onRevert}>
              <Undo2 size={13} />
              Revert
              {kbd("u")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

/** Who recorded an item. */
function Noticed({ item }: { item: DigestItem }) {
  const who = item.noticed_by;
  if (who.kind === "agent") return <>Noticed by {who.name || "an agent"}</>;
  if (who.kind === "tool") return <>Noticed by kapi {who.name}</>;
  return who.name ? <>Noticed by {who.name}</> : <>Noticed by you</>;
}

/** The rule as a sentence, with the forms set apart from the words around them. */
function RuleSentence({ item }: { item: DigestItem }) {
  const s = item.subject;
  if (s.kind === "term" && s.term) {
    const avoid = <Forms rule={s.term} />;
    return s.term.replacement ? (
      <>
        Write <Form>{s.term.replacement}</Form>, not {avoid}
      </>
    ) : (
      <>Avoid {avoid}</>
    );
  }
  if (s.kind === "memory" && s.memory) {
    return (
      <>
        Translate <Form>{s.memory.source}</Form> into {s.memory.target_locale} as{" "}
        <Form>{s.memory.target}</Form>
      </>
    );
  }
  return <>{s.text || item.sentence}</>;
}

function Form({ children }: { children: React.ReactNode }) {
  return (
    <span className="font-medium" translate="no">
      {children}
    </span>
  );
}

/** The forms a rule avoids, as "a, b or c". */
function Forms({ rule }: { rule: DigestTermRule }) {
  const forms = [rule.term ?? "", ...(rule.forms ?? [])].filter(Boolean);
  return (
    <span translate="no" className="text-muted-foreground">
      {forms.map((f, i) => (
        <span key={f}>
          {i > 0 && (i === forms.length - 1 ? " / " : ", ")}
          <span className="line-through decoration-muted-foreground/50">{f}</span>
        </span>
      ))}
    </span>
  );
}

/** When the person last looked, as a reader says it: "Tuesday", "12 September". */
export function formatLook(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return iso;
  const days = (Date.now() - at.getTime()) / 86_400_000;
  if (days < 1) return at.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  if (days < 7) return at.toLocaleDateString(undefined, { weekday: "long" });
  return at.toLocaleDateString(undefined, { day: "numeric", month: "long" });
}
