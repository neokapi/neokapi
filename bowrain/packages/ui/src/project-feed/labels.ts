// The words of the project feed, in the UI language. The server sends kinds,
// states and typed counts; each is worded here, with the language's plural
// rules, and nothing is built from a server sentence.
import { t } from "@neokapi/i18n-react/runtime";
import type { FeedActorKind, FeedCard, FeedCount, FeedEntry } from "@neokapi/bowrain-client";
import { ageOf, mainCount, targetChanges } from "@neokapi/bowrain-client";

/** The label of a feed kind: Push, Run, Review. */
export function kindLabel(kind: string): string {
  switch (kind) {
    case "push":
      return t("Push", "project feed kind");
    case "convergence_run":
      return t("Run", "project feed kind");
    case "review_summary":
      return t("Review", "project feed kind");
    case "recipe_change":
      return t("Recipe", "project feed kind");
    case "context_change":
      return t("Context", "project feed kind");
    case "change_set":
      return t("Change set", "project feed kind");
    case "coverage_milestone":
      return t("Coverage", "project feed kind");
    default:
      return t("Change", "project feed kind");
  }
}

/** A run's outcome or a change set's step; an unknown one as the server sent it. */
export function stateLabel(state: string): string {
  switch (state) {
    case "converged":
      return t("converged", "run outcome");
    case "parked":
      return t("parked", "run outcome");
    case "failed":
      return t("failed", "run outcome");
    case "canceled":
      return t("canceled", "run outcome");
    case "submitted":
      return t("submitted", "change set step");
    case "approved":
      return t("approved", "change set step");
    case "rejected":
      return t("rejected", "change set step");
    case "merged":
      return t("merged", "change set step");
    case "created":
      return t("opened", "change set step");
    case "abandoned":
      return t("abandoned", "change set step");
    case "superseded":
      return t("superseded", "change set step");
    default:
      return state;
  }
}

/** The label of an actor kind. */
export function actorKindLabel(kind: FeedActorKind): string {
  switch (kind) {
    case "agent":
      return t("Agent", "feed actor kind");
    case "tool":
      return t("Tool", "feed actor kind");
    default:
      return t("Person", "feed actor kind");
  }
}

/**
 * What a count counts, in the form that agrees with it, without the number:
 * the label of a `StatRow` item.
 */
export function unitLabel(key: string, count: number): string {
  switch (key) {
    case "files":
      return t("{count, plural, one {file} other {files}}", { count });
    case "blocks":
      return t("{count, plural, one {block} other {blocks}}", { count });
    case "passes":
      return t("{count, plural, one {pass} other {passes}}", { count });
    case "locales":
      return t("{count, plural, one {language} other {languages}}", { count });
    case "produced":
      return t("{count, plural, one {translation} other {translations}}", { count });
    case "decisions":
      return t("{count, plural, one {decision} other {decisions}}", { count });
    case "approved":
      return t("approved", "review count");
    case "rejected":
      return t("rejected", "review count");
    case "unreviewed":
      return t("sent back", "review count");
    case "fixed":
      return t("fixed", "review count");
    case "bulk_approved":
      return t("approved in bulk", "review count");
    case "collections_added":
      return t("{count, plural, one {collection added} other {collections added}}", { count });
    case "collections_removed":
      return t("{count, plural, one {collection removed} other {collections removed}}", { count });
    case "coordinates_changed":
      return t("{count, plural, one {collection moved} other {collections moved}}", { count });
    case "targets_added":
      return t("{count, plural, one {language added} other {languages added}}", { count });
    case "targets_removed":
      return t("{count, plural, one {language removed} other {languages removed}}", { count });
    case "settings_changed":
      return t("{count, plural, one {setting changed} other {settings changed}}", { count });
    case "term":
      return t("{count, plural, one {term} other {terms}}", { count });
    case "term_rule":
      return t("{count, plural, one {term rule} other {term rules}}", { count });
    case "voice_rule":
      return t("{count, plural, one {voice rule} other {voice rules}}", { count });
    case "voice_profile":
      return t("{count, plural, one {voice profile} other {voice profiles}}", { count });
    case "note":
      return t("{count, plural, one {note} other {notes}}", { count });
    case "decision":
      return t("{count, plural, one {decision} other {decisions}}", { count });
    case "memory":
      return t("{count, plural, one {memory pair} other {memory pairs}}", { count });
    case "binding":
      return t("{count, plural, one {binding} other {bindings}}", { count });
    case "items":
      return t("{count, plural, one {item} other {items}}", { count });
    case "add":
      return t("added", "context operation");
    case "edit":
      return t("edited", "context operation");
    case "retire":
      return t("retired", "context operation");
    case "keep":
      return t("kept", "context operation");
    case "drop":
      return t("dropped", "context operation");
    case "percent":
      return t("percent", "coverage");
    default:
      return key;
  }
}

function formatNumber(n: number, locale: string): string {
  try {
    return new Intl.NumberFormat(locale).format(n);
  } catch {
    return String(n);
  }
}

/** One count with its unit: "40 blocks", "90%". */
export function countText(c: FeedCount, locale: string): string {
  if (c.key === "percent") return `${formatNumber(c.n, locale)}%`;
  return `${formatNumber(c.n, locale)} ${unitLabel(c.key, c.n)}`;
}

/** A language as the reader names it, else its code. */
export function languageName(code: string, locale: string): string {
  try {
    const names = new Intl.DisplayNames([locale || "en", "en"], {
      type: "language",
      fallback: "none",
    });
    return names.of(code.replace(/_/g, "-")) ?? code;
  } catch {
    return code;
  }
}

/** The row's primary line: "Push · main", "Run · converged", "nb · 90%". */
export function entryTitle(e: FeedEntry, locale: string): string {
  if (e.kind === "coverage_milestone" && e.locales?.[0]) {
    const lang = languageName(e.locales[0], locale);
    const pct = e.counts.percent;
    return pct !== undefined ? `${lang} · ${countText({ key: "percent", n: pct }, locale)}` : lang;
  }
  let object: string | undefined;
  switch (e.kind) {
    case "push":
      object = e.stream;
      break;
    case "convergence_run":
    case "change_set":
      object = e.state ? stateLabel(e.state) : undefined;
      break;
    case "review_summary":
      object = e.locales?.[0] ? languageName(e.locales[0], locale) : undefined;
      break;
    case "recipe_change": {
      const cols = e.diff?.collections ?? [];
      object = cols.length === 1 ? cols[0].name : undefined;
      break;
    }
  }
  return object ? `${kindLabel(e.kind)} · ${object}` : kindLabel(e.kind);
}

/** Who did it, as a card names them. A tool's name is capitalised. */
export function actorName(e: FeedEntry): string {
  if (e.actor.kind === "tool" && e.actor.name) {
    return e.actor.name.charAt(0).toUpperCase() + e.actor.name.slice(1);
  }
  return e.actor.name || e.on_behalf_of?.name || t("Someone");
}

/** Items joined the way the UI language lists them: "A, B and C". */
export function listOf(items: string[], locale: string): string {
  if (items.length <= 1) return items[0] ?? "";
  // The package's lib predates Intl.ListFormat; every supported browser has it.
  const ListFormat = (
    Intl as unknown as {
      ListFormat?: new (
        locale: string,
        opts: { type: "conjunction" },
      ) => { format: (items: string[]) => string };
    }
  ).ListFormat;
  try {
    if (ListFormat) return new ListFormat(locale || "en", { type: "conjunction" }).format(items);
  } catch {
    // fall through
  }
  return items.join(", ");
}

/** The compact time of an entry: now, 5m, 2h, yesterday, or the date. */
export function ageText(iso: string, locale: string, now = new Date()): string {
  const age = ageOf(iso, now);
  switch (age.unit) {
    case "now":
      return t("now", "feed time");
    case "minutes":
      return t("{count}m", { count: age.n });
    case "hours":
      return t("{count}h", { count: age.n });
    case "yesterday":
      return t("yesterday", "feed time");
    case "date":
      try {
        return new Intl.DateTimeFormat(locale, { day: "numeric", month: "short" }).format(age.date);
      } catch {
        return age.date.toDateString();
      }
  }
}

const CONTEXT_OPS = ["add", "edit", "keep", "drop", "retire"];

function contextAction(op: string, agent: boolean, items: string): string {
  switch (op) {
    case "edit":
      return t("edited {items}", { items });
    case "keep":
      return t("kept {items}", { items });
    case "drop":
      return t("dropped {items}", { items });
    case "retire":
      return t("retired {items}", { items });
    default:
      return agent ? t("proposed {items}", { items }) : t("added {items}", { items });
  }
}

function changeSetAction(state: string, name: string): string {
  switch (state) {
    case "created":
      return t("opened {name}", { name });
    case "approved":
      return t("approved {name}", { name });
    case "rejected":
      return t("rejected {name}", { name });
    case "merged":
      return t("merged {name}", { name });
    case "abandoned":
      return t("abandoned {name}", { name });
    case "superseded":
      return t("replaced {name}", { name });
    default:
      return t("submitted {name} for review", { name });
  }
}

/**
 * What the actor did, as the sentence after their name: "added Japanese and
 * Korean", "proposed 6 term rules", "drafted 40 translations in 3 languages".
 */
export function actionText(e: FeedEntry, locale: string): string {
  const c = e.counts ?? {};
  switch (e.kind) {
    case "push": {
      if (c.blocks === undefined) return t("synced the repository");
      return e.stream
        ? t(
            "{count, plural, one {pushed # block to {stream}} other {pushed # blocks to {stream}}}",
            {
              count: c.blocks,
              stream: e.stream,
            },
          )
        : t("{count, plural, one {pushed # block} other {pushed # blocks}}", { count: c.blocks });
    }
    case "convergence_run": {
      if (e.state === "parked") return t("parked a run that needs attention");
      if (e.state === "failed") return t("could not finish a run");
      if (e.state === "canceled") return t("canceled a run");
      const produced = c.produced ?? 0;
      if (produced === 0) return t("had nothing to draft");
      const n = e.locales?.length ?? c.locales ?? 0;
      const languages =
        n === 1 && e.locales
          ? languageName(e.locales[0], locale)
          : t("{count, plural, one {# language} other {# languages}}", { count: n });
      return t(
        "{count, plural, one {drafted # translation in {languages}} other {drafted # translations in {languages}}}",
        { count: produced, languages },
      );
    }
    case "review_summary": {
      const parts: string[] = [];
      // Approvals one by one and in bulk read as one count; the split bar
      // keeps the detail.
      const approved = (c.approved ?? 0) + (c.bulk_approved ?? 0);
      if (approved) parts.push(t("approved {count}", { count: approved }));
      if (c.fixed) parts.push(t("fixed {count}", { count: c.fixed }));
      if (c.rejected) parts.push(t("rejected {count}", { count: c.rejected }));
      if (c.unreviewed) parts.push(t("sent back {count}", { count: c.unreviewed }));
      const language = e.locales?.[0] ? languageName(e.locales[0], locale) : "";
      return t("{parts} in {language}", { parts: listOf(parts, locale), language });
    }
    case "recipe_change": {
      const parts: string[] = [];
      const { added, removed } = targetChanges(e);
      if (added.length) {
        parts.push(
          t("added {languages}", {
            languages: listOf(
              added.map((l) => languageName(l, locale)),
              locale,
            ),
          }),
        );
      }
      if (removed.length) {
        parts.push(
          t("removed {languages}", {
            languages: listOf(
              removed.map((l) => languageName(l, locale)),
              locale,
            ),
          }),
        );
      }
      for (const col of e.diff?.collections ?? []) {
        if (col.change === "added")
          parts.push(t("added the {name} collection", { name: col.name }));
        else if (col.change === "removed") {
          parts.push(t("removed the {name} collection", { name: col.name }));
        } else parts.push(t("moved {name}", { name: col.name }));
      }
      if (e.diff?.settings?.length) {
        parts.push(
          t("{count, plural, one {changed # setting} other {changed # settings}}", {
            count: e.diff.settings.length,
          }),
        );
      }
      return parts.length ? listOf(parts, locale) : t("made a change");
    }
    case "context_change": {
      const op = CONTEXT_OPS.find((o) => (c[o] ?? 0) > 0) ?? "add";
      const main = mainCount(e);
      return contextAction(op, e.actor.kind === "agent", main ? countText(main, locale) : "");
    }
    case "change_set":
      return changeSetAction(e.state ?? "", e.name || t("a change set"));
    case "coverage_milestone": {
      const language = e.locales?.[0] ? languageName(e.locales[0], locale) : "";
      return t("{language} reached {percent}%", { language, percent: c.percent ?? 0 });
    }
    default:
      return t("made a change");
  }
}

/** The sentence of a folded card: "ran 3 times". */
export function foldedActionText(card: FeedCard, locale: string): string {
  const lead = card.entries[0];
  const count = card.entries.length;
  if (count < 2) return actionText(lead, locale);
  switch (lead.kind) {
    case "push":
      return t("{count, plural, other {pushed # times}}", { count });
    case "convergence_run":
      return t("{count, plural, other {ran # times}}", { count });
    case "context_change":
      return t("{count, plural, other {made # context changes}}", { count });
    case "change_set":
      return t("{count, plural, other {updated # change sets}}", { count });
    default:
      return actionText(lead, locale);
  }
}

/** The count a row leads with, worded, or undefined. */
export function mainCountText(e: FeedEntry, locale: string): string | undefined {
  if (e.kind === "coverage_milestone") return undefined;
  const c = mainCount(e);
  return c ? countText(c, locale) : undefined;
}
