// Who recorded a rule or a suggestion: a person, an assistant, or a kapi tool.

import { t } from "@neokapi/i18n-react/runtime";
import { ActorMark } from "@neokapi/ui-primitives";
import type { DigestItem } from "./types";

type Actor = DigestItem["noticed_by"];

/**
 * The mark for who recorded an item: a person by their initials, an assistant
 * by its host's letters and hue, a kapi tool by its own name.
 */
export function NoticedMark({ who, decorative }: { who: Actor; decorative?: boolean }) {
  if (who.kind === "person") {
    return <ActorMark actor="person" name={who.name} size="sm" decorative={decorative} />;
  }
  const actor = who.kind === "tool" ? "kapi" : who.name || "agent";
  return <ActorMark actor={actor} size="sm" decorative={decorative} />;
}

/**
 * The name a kapi tool is shown by: "kapi check" for the check, and "kapi"
 * alone for kapi itself, which files what it sees in the files under the
 * name "kapi".
 */
export function toolName(name?: string): string {
  return name && name !== "kapi" ? `kapi ${name}` : "kapi";
}

/**
 * Who recorded an item: their mark, their name, and the session it came from.
 * nameOf gives the name a person is shown by; a person it names nothing for,
 * and with no name of their own, is "you".
 */
export function NoticedBy({
  who,
  nameOf,
}: {
  who: Actor;
  nameOf?: (who: Actor) => string | undefined;
}) {
  const mark = <NoticedMark who={who} decorative />;
  let name: React.ReactNode;
  if (who.kind === "agent") name = who.name || t("an assistant");
  else if (who.kind === "tool") name = toolName(who.name);
  else name = nameOf?.(who) || who.name || t("you");
  return (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5">
      {mark}
      <span translate="no">{name}</span>
      {who.session && (
        <span className="font-mono text-[10px] text-muted-foreground" translate="no">
          {who.session}
        </span>
      )}
    </span>
  );
}
