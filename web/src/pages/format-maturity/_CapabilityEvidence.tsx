import {
  CapabilityCoverage,
  CapabilityInventory,
  ObservationList,
  type CapabilityLabels,
  type CoverageLabels,
} from "@neokapi/docs-shared/capability";
import { t } from "@neokapi/i18n-react/runtime";
import { formats } from "@neokapi/reference-data";
import current from "../../../static/data/content-capabilities.json";
import observations from "../../../static/data/content-capabilities-history.json";
import type { History } from "./_history";

type InventoryProps = { initialStatus?: string; evidenceBaseUrl: string };

const repo = "https://github.com/neokapi/neokapi";
const outputNames = new Map(
  formats.entries
    .filter((entry) => entry.source === "built-in")
    .map((entry) => [entry.id, entry.displayName]),
);

function outputName(id: string) {
  if (id === "json-catalog") return t("JSON catalog", "output format");
  if (id === "yaml-catalog") return t("YAML catalog", "output format");
  return outputNames.get(id) ?? id;
}

function operationName(id: string) {
  const names: Record<string, string> = {
    "project-text": t("Convert text", "output operation"),
    "project-cells": t("Convert spreadsheet cells", "output operation"),
    "project-cues": t("Convert subtitle cues", "output operation"),
    email: t("Convert email", "output operation"),
    structure: t("Preserve document structure", "output operation"),
  };
  return names[id] ?? id;
}

function statusNames(): Record<string, string> {
  return {
    failed: t("Failed", "capability state"),
    unverified: t("Unverified", "capability state"),
    verified: t("Verified", "capability state"),
    declared: t("Declared", "capability state"),
    unsupported: t("Unsupported", "capability state"),
  };
}

function labels(): CapabilityLabels {
  return {
    heading: t("Output formats", "capability inventory"),
    filter: t("Find an output format or operation", "capability filter"),
    status: t("Evidence status", "capability filter"),
    allStatuses: t("All statuses", "capability filter"),
    empty: t("No operations match these filters.", "capability filter"),
    clear: t("Clear filters", "capability filter"),
    count: (shown, total) =>
      t("{shown} of {total} output formats", "inventory count", { shown, total }),
    states: statusNames(),
    subject: outputName,
    operation: operationName,
  };
}

export default function CapabilityEvidence({ initialStatus, evidenceBaseUrl }: InventoryProps) {
  return (
    <CapabilityInventory
      subjects={current.outputs}
      labels={labels()}
      evidenceBaseUrl={evidenceBaseUrl}
      initialStatus={initialStatus}
    />
  );
}

function changeCount(count: number) {
  return count === 1
    ? t("1 change", "observation history")
    : t("{count} changes", "observation history", { count });
}

function coverageLabels(): CoverageLabels {
  const states = statusNames();
  const absent = t("absent", "historical observation");
  return {
    none: t("No observation", "capability history"),
    summary: (counts) =>
      counts.subjects === 1
        ? t("1 subject · {verified} verified · {failed} failed", "capability history", counts)
        : t(
            "{subjects} subjects · {verified} verified · {failed} failed",
            "capability history",
            counts,
          ),
    changes: changeCount,
    change: ({ subject, operation, before, after }) =>
      t("{subject} / {operation} · {before} → {after}", "capability change", {
        subject: outputName(subject),
        operation: operationName(operation),
        before: (before && states[before]) ?? before ?? absent,
        after: (after && states[after]) ?? after ?? absent,
      }),
  };
}

function formatChanges(entry: History, previous?: History) {
  if (!previous) return t("First observation", "capability history");
  const ids = [
    ...new Set([...Object.keys(previous.formats), ...Object.keys(entry.formats)]),
  ].sort();
  const changes = ids.filter((id) => previous.formats[id] !== entry.formats[id]);
  if (!changes.length) return t("No Engine grade changes", "format history");
  const absent = t("absent", "historical observation");
  return (
    <details>
      <summary>{changeCount(changes.length)}</summary>
      <ul>
        {changes.map((id) => (
          <li key={id}>
            {outputName(id)} · {previous.formats[id] ?? absent} → {entry.formats[id] ?? absent}
          </li>
        ))}
      </ul>
    </details>
  );
}

export function ObservationHistory({ dataBaseUrl }: { dataBaseUrl: string }) {
  const history = observations as History[];
  const coverage = coverageLabels();
  return (
    <ObservationList
      heading={t("Observation history", "maturity dashboard")}
      intro={t(
        "Each observation compares with the preceding published date. Output measurements begin with the first operation audit.",
        "capability history",
      )}
      observations={history.map((entry, index) => {
        const previous = history[index - 1];
        return {
          date: entry.date,
          href: entry.revision
            ? repo + "/blob/" + entry.revision + "/web/static/data/format-maturity.json"
            : undefined,
          items: [
            {
              label: t("Engine grades", "observation history"),
              value: formatChanges(entry, previous),
            },
            {
              label: t("Output formats", "capability inventory"),
              value: (
                <CapabilityCoverage
                  profile={entry.outputs}
                  previous={previous?.outputs}
                  labels={coverage}
                />
              ),
            },
          ],
        };
      })}
      datasetsHeading={t("Source data", "observation history")}
      datasets={[
        {
          href: dataBaseUrl + "content-capabilities.json",
          label: t("Current capability evidence (JSON)", "observation history"),
        },
        {
          href: dataBaseUrl + "content-capabilities-history.json",
          label: t("Historical observations (JSON)", "observation history"),
        },
        {
          href: dataBaseUrl + "format-maturity-history.json",
          label: t("Format grade totals by axis and date (JSON)", "observation history"),
        },
      ]}
    />
  );
}
