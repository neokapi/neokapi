import { History, Loader2 } from "lucide-react";
import { Badge, LayerCard } from "@neokapi/ui-primitives";
import type { EditionHistory, HistoryEntry } from "@neokapi/contract-types";
import { t } from "@neokapi/i18n-react/runtime";

/** Who made a recorded change, as a person reads it. */
export function changeAuthor(entry: HistoryEntry): string {
  const actor = entry.actor;
  if (!actor) return t("Someone outside kapi");
  switch (actor.kind) {
    case "person":
      return actor.name || t("A person");
    case "agent":
      return actor.name ? t("Agent {name}", { name: actor.name }) : t("An agent");
    case "tool":
      return actor.name ? t("Tool {name}", { name: actor.name }) : t("A tool");
    default:
      return actor.kind;
  }
}

/** Where a recorded change was made, as a person reads it. */
export function changeSurface(origin: string | undefined): string | undefined {
  if (!origin) return undefined;
  if (origin.startsWith("flow:")) return t("in the {flow} flow", { flow: origin.slice(5) });
  switch (origin) {
    case "desktop":
      return t("in Kapi Desktop");
    case "apply":
      return t("with kapi apply");
    case "ksed":
      return t("with ksed");
    case "mcp":
      return t("through an agent's tools");
    case "merge":
      return t("by kapi merge");
    case "pull":
      return t("by kapi pull");
    case "observed":
      return t("outside kapi");
    default:
      return origin;
  }
}

/** What a recorded change did to the edition. */
function changeKind(entry: HistoryEntry): string {
  if (entry.before === "absent") return t("created");
  if (entry.after === "absent") return t("removed");
  return t("changed");
}

function formatWhen(at: string): string {
  const d = new Date(at);
  return Number.isNaN(d.getTime()) ? at : d.toLocaleString();
}

export interface ChangesCardProps {
  history?: EditionHistory | null;
  loading?: boolean;
  defaultOpen?: boolean;
  className?: string;
}

/**
 * The recorded changes to the edition under review, most recent first: who
 * made each, where, when, and whether it left the text in force. It reads the
 * project's block history, which every change made through kapi records.
 */
export function ChangesCard({
  history,
  loading,
  defaultOpen = false,
  className,
}: ChangesCardProps) {
  const entries = history?.entries ?? [];
  const latest = entries[0];
  const summary = loading ? (
    <Loader2 size={11} className="animate-spin" />
  ) : latest ? (
    <>
      <span className="text-foreground">
        {t("{count, plural, one {# recorded change} other {# recorded changes}}", {
          count: entries.length,
        })}
      </span>
      <span>{t("last by {author}", { author: changeAuthor(latest) })}</span>
    </>
  ) : (
    <span>{t("No change recorded")}</span>
  );

  return (
    <LayerCard
      title={t("Changes")}
      icon={<History size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />}
      summary={summary}
      dataSlot="review-changes"
      toggleLabel={t("The recorded changes to this text")}
      defaultOpen={defaultOpen}
      className={className}
    >
      {entries.length === 0 ? (
        <p className="text-xs text-muted-foreground" data-slot="review-changes-empty">
          {t(
            "Nothing has changed this text through kapi yet. A change made in an editor is recorded once kapi reads it.",
          )}
        </p>
      ) : (
        <ol className="space-y-1.5" data-slot="review-changes-list">
          {entries.map((entry) => {
            const surface = changeSurface(entry.origin);
            const inForce = entry.after === history?.rev;
            return (
              <li
                key={entry.record}
                className="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 text-xs"
                data-slot="review-change"
              >
                <span className="text-foreground">{changeAuthor(entry)}</span>
                <span className="text-muted-foreground">{changeKind(entry)}</span>
                {surface && <span className="text-muted-foreground">{surface}</span>}
                <time className="text-muted-foreground" dateTime={entry.at}>
                  {formatWhen(entry.at)}
                </time>
                {inForce && (
                  <Badge
                    variant="outline"
                    className="text-[10px]"
                    data-slot="review-change-current"
                  >
                    {t("in force")}
                  </Badge>
                )}
              </li>
            );
          })}
        </ol>
      )}
    </LayerCard>
  );
}
