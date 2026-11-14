// The words of the project feed, in the UI language. The server sends kinds,
// states and typed counts; each is worded here, with the language's plural
// rules, and nothing is built from a server sentence.
import { t } from "@neokapi/i18n-react/runtime";
import type { FeedActorKind, FeedCount, FeedEntry } from "@neokapi/bowrain-client";
import { mainCount } from "@neokapi/bowrain-client";

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
    const names = new Intl.DisplayNames([locale, "en"], { type: "language", fallback: "none" });
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

/** Who did it, as the row names them. */
export function actorName(e: FeedEntry): string {
  return e.actor.name || e.on_behalf_of?.name || t("Someone");
}

/** The count a row leads with, worded, or undefined. */
export function mainCountText(e: FeedEntry, locale: string): string | undefined {
  if (e.kind === "coverage_milestone") return undefined;
  const c = mainCount(e);
  return c ? countText(c, locale) : undefined;
}
