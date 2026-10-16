// Names a reader sees for axes, values, states and event groups. Every string
// here is data rather than JSX, so each goes through `t()`; the getters read
// the dictionary at render time, so a language switch repaints them.

import { t } from "@neokapi/i18n-react/runtime";
import { formatLocale, uiLocaleTag } from "@neokapi/ui-primitives";
import { type Atlas, DERIVED_AXES, NOT_SET } from "./model/atlas";
import type { EventGroup } from "./model/histogram";
import type { FlowLens, QuestionId } from "./model/query";
import { ABSENT, UNKNOWN } from "./model/state";
import { dayStart } from "./model/time";

/** The name of an axis: the data's label, else a built-in name, else its id. */
export function axisLabel(atlas: Atlas | undefined, axis: string): string {
  const label = atlas?.axisById.get(axis)?.label;
  if (label) return label;
  switch (axis) {
    case "family":
      return t("Family", "atlas axis: rule, note, memory, term or content");
    case "product":
      return t("Product", "context axis");
    case "channel":
      return t("Channel", "context axis");
    case "brand":
      return t("Brand", "context axis");
    case "market":
      return t("Market", "context axis");
    case "locale":
      return t("Locale", "atlas axis");
    case "region":
      return t("Region", "atlas axis: geography from the locale's region");
    case "actor":
      return t("Actor", "atlas axis: person, agent or tool");
    case "project":
      return t("Project", "atlas axis");
    case DERIVED_AXES.statusStart:
      return t("State at start", "atlas axis: state at the window's first instant");
    case DERIVED_AXES.statusSnapshot:
      return t("State at snapshot", "atlas axis: state at the snapshot");
    case DERIVED_AXES.lifetime:
      return t("Lifetime", "atlas axis: where the snapshot falls in a rule's window");
    case DERIVED_AXES.age:
      return t("Age of record", "atlas axis: time since recorded");
    case DERIVED_AXES.activity:
      return t("Activity in window", "atlas axis: whether an event touched the record");
    default:
      return axis;
  }
}

/** The name of a state. */
export function stateLabel(state: string): string {
  switch (state) {
    case "established":
      return t("Established", "context state");
    case "suggested":
      return t("Suggested", "context state");
    case "contested":
      return t("Contested", "context state");
    case "stale":
      return t("Stale", "context state: a decision cites a superseded source");
    case "translated":
      return t("Translated", "content state");
    case "draft":
      return t("Draft", "content state");
    case "not-started":
      return t("Not started", "content state");
    case "withdrawn":
      return t("Withdrawn", "context state");
    case "dropped":
      return t("Dropped", "context state");
    case "reset":
      return t("Reset", "context state");
    case UNKNOWN:
      return t("Unknown", "context state: the record carries no history");
    case ABSENT:
      return t("Not recorded yet", "context state: before the record's first entry");
    default:
      return state;
  }
}

function familyLabel(id: string): string | undefined {
  switch (id) {
    case "rule":
      return t("Rule", "context family");
    case "note":
      return t("Note", "context family");
    case "memory":
      return t("Content memory", "context family");
    case "term":
      return t("Term", "context family");
    case "content":
      return t("Content", "context family");
    case "import":
      return t("Imported file", "context family");
    default:
      return undefined;
  }
}

function derivedValueLabel(axis: string, value: string): string | undefined {
  if (axis === DERIVED_AXES.statusStart || axis === DERIVED_AXES.statusSnapshot) {
    return stateLabel(value);
  }
  if (axis === DERIVED_AXES.lifetime) {
    switch (value) {
      case "upcoming":
        return t("Upcoming", "lifetime: the window has not opened");
      case "in-force":
        return t("In force", "lifetime: inside the window");
      case "expired":
        return t("Expired", "lifetime: the window has closed");
      case "open":
        return t("No window", "lifetime: the record carries no window");
    }
  }
  if (axis === DERIVED_AXES.age) {
    switch (value) {
      case "lt-7d":
        return t("Under 7 days", "age of record");
      case "7-30d":
        return t("7 to 30 days", "age of record");
      case "30-90d":
        return t("30 to 90 days", "age of record");
      case "gt-90d":
        return t("Over 90 days", "age of record");
      case "unknown":
        return t("Unknown", "age of record: recorded time unknown");
    }
  }
  if (axis === DERIVED_AXES.activity) {
    return value === "changed"
      ? t("Changed in window", "activity: an event touched the record")
      : t("No change in window", "activity: no event touched the record");
  }
  return undefined;
}

function rangeLabel(value: string): string | undefined {
  const m = /^(\d+(?:\.\d+)?)-(\d+(?:\.\d+)?)?$/u.exec(value);
  if (!m) return undefined;
  if (!m[2]) return t("{min} and over", "a numeric range with no upper bound", { min: m[1] });
  return t("{min} to {max}", "a numeric range", { min: m[1], max: m[2] });
}

/** The name of one value on an axis. */
export function valueLabel(atlas: Atlas | undefined, axis: string, value: string): string {
  if (value === NOT_SET) return t("Not set", "an axis value a record carries no coordinate for");
  const derived = derivedValueLabel(axis, value);
  if (derived) return derived;
  const info = atlas?.axisById.get(axis);
  const listed = info?.values.find((v) => v.id === value)?.label;
  if (listed) return listed;
  if (axis === "family") return familyLabel(value) ?? value;
  if (axis === "actor") {
    if (value === "person") return t("Person", "actor kind");
    if (value === "agent") return t("Agent", "actor kind");
    if (value === "tool") return t("Tool", "actor kind");
  }
  if (axis === "locale") return formatLocale(value).text;
  if (info?.kind === "range") return rangeLabel(value) ?? value;
  return value;
}

/** A value's label with its axis, for chips and accessible names. */
export function pickLabel(atlas: Atlas | undefined, axis: string, value: string): string {
  return t("{axis}: {value}", {
    axis: axisLabel(atlas, axis),
    value: valueLabel(atlas, axis, value),
  });
}

/** The name of an event group in the time brush. */
export function eventGroupLabel(group: EventGroup): string {
  switch (group) {
    case "added":
      return t("Recorded", "event group: observed, corrected, imported, edited");
    case "decided":
      return t("Decided", "event group: kept, established, widened, decided");
    case "closed":
      return t("Closed", "event group: dropped, withdrawn, reset");
    case "window":
      return t("Window", "event group: a lifetime window opens or closes");
    case "other":
      return t("Other", "event group");
  }
}

/** The name of a colour lens. */
export function lensLabel(lens: FlowLens): string {
  switch (lens) {
    case "origin":
      return t("First column", "flow colour: by the first column's value");
    case "attention":
      return t("Needs a person", "flow colour: share waiting for a person");
    case "change":
      return t("Change in needs a person", "flow colour: how that share moved");
  }
}

/** The starting questions, as a reader asks them. */
export function questionLabel(id: QuestionId): string {
  switch (id) {
    case "changed":
      return t("What changed in this window", "atlas question");
    case "applies":
      return t("Where rules apply", "atlas question");
    case "needs-person":
      return t("What needs a person", "atlas question");
    case "lifetime":
      return t("Rules with a lifetime", "atlas question");
  }
}

/** A day as the reader writes it: "18 Sep". */
export function formatDay(day: string, withYear = false): string {
  try {
    return new Intl.DateTimeFormat(uiLocaleTag(), {
      day: "numeric",
      month: "short",
      ...(withYear ? { year: "numeric" } : {}),
      timeZone: "UTC",
    }).format(new Date(dayStart(day)));
  } catch {
    return day;
  }
}

/** A count in the reader's language. */
export function formatCount(n: number): string {
  try {
    return new Intl.NumberFormat(uiLocaleTag()).format(n);
  } catch {
    return String(n);
  }
}

/** A share as a whole percentage, or a dash when there is nothing to divide. */
export function formatShare(share: number | null): string {
  if (share === null) return "–";
  try {
    return new Intl.NumberFormat(uiLocaleTag(), {
      style: "percent",
      maximumFractionDigits: 0,
    }).format(share);
  } catch {
    return `${Math.round(share * 100)}%`;
  }
}

/** A signed change in percentage points: "+12 pp", "−3 pp". */
export function formatPoints(delta: number | null): string {
  if (delta === null) return "–";
  const pp = Math.round(delta * 100);
  const sign = pp > 0 ? "+" : pp < 0 ? "−" : "";
  return t("{sign}{points} pp", "a change in percentage points", { sign, points: Math.abs(pp) });
}
