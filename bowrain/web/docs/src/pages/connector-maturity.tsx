import { translate } from "@docusaurus/Translate";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import {
  CapabilityCoverage,
  CapabilityInventory,
  ObservationList,
  type CapabilityLabels,
  type CoverageLabels,
  type Profile,
} from "@neokapi/docs-shared";
import current from "../../static/data/connector-capabilities.json";
import observations from "../../static/data/connector-capabilities-history.json";
import styles from "./connector-maturity.module.css";

// The connector half of the content maturity report. The data comes from
// `make -C bowrain refresh-connector-maturity`; the inventory and history
// components are shared with the format maturity dashboard on the kapi site.
// Every reader-facing string goes through translate() so write-translations
// finds it.

type Observation = { date: string; connectors?: Profile };

function connectorName(id: string) {
  const names: Record<string, string> = {
    "bowrain-source": translate({ id: "connectorMaturity.connector.project", message: "Bowrain project" }),
    figma: "Figma",
    file: translate({ id: "connectorMaturity.connector.file", message: "Local files" }),
    forge: translate({ id: "connectorMaturity.connector.forge", message: "Git forge" }),
    git: "Git",
    hubspot: "HubSpot",
    wordpress: "WordPress",
  };
  return names[id] ?? id;
}

function operationName(id: string) {
  const names: Record<string, string> = {
    fetch: translate({ id: "connectorMaturity.operation.fetch", message: "Fetch content" }),
    list: translate({ id: "connectorMaturity.operation.list", message: "List content" }),
    publish: translate({ id: "connectorMaturity.operation.publish", message: "Publish content" }),
    registration: translate({ id: "connectorMaturity.operation.registration", message: "Register connector" }),
    pull: translate({ id: "connectorMaturity.operation.pull", message: "Pull content" }),
    push: translate({ id: "connectorMaturity.operation.push", message: "Push content" }),
  };
  return names[id] ?? id;
}

function stateNames(): Record<string, string> {
  return {
    failed: translate({ id: "connectorMaturity.state.failed", message: "Failed" }),
    unverified: translate({ id: "connectorMaturity.state.unverified", message: "Unverified" }),
    verified: translate({ id: "connectorMaturity.state.verified", message: "Verified" }),
    declared: translate({ id: "connectorMaturity.state.declared", message: "Declared" }),
    unsupported: translate({ id: "connectorMaturity.state.unsupported", message: "Unsupported" }),
  };
}

function inventoryLabels(): CapabilityLabels {
  return {
    heading: translate({ id: "connectorMaturity.inventory.heading", message: "Connector operations" }),
    filter: translate({ id: "connectorMaturity.inventory.filter", message: "Find a connector or operation" }),
    status: translate({ id: "connectorMaturity.inventory.status", message: "Evidence status" }),
    allStatuses: translate({ id: "connectorMaturity.inventory.allStatuses", message: "All statuses" }),
    empty: translate({ id: "connectorMaturity.inventory.empty", message: "No operations match these filters." }),
    clear: translate({ id: "connectorMaturity.inventory.clear", message: "Clear filters" }),
    count: (shown, total) =>
      translate(
        { id: "connectorMaturity.inventory.count", message: "{shown} of {total} connectors" },
        { shown, total },
      ),
    states: stateNames(),
    subject: connectorName,
    operation: operationName,
  };
}

function coverageLabels(): CoverageLabels {
  const states = stateNames();
  const absent = translate({ id: "connectorMaturity.history.absent", message: "absent" });
  return {
    none: translate({ id: "connectorMaturity.history.none", message: "No observation" }),
    summary: (counts) =>
      translate(
        {
          id: "connectorMaturity.history.summary",
          message: "{subjects} measured · {verified} verified · {failed} failed",
        },
        counts,
      ),
    changes: (count) =>
      translate({ id: "connectorMaturity.history.changes", message: "{count} changed" }, { count }),
    change: ({ subject, operation, before, after }) =>
      translate(
        { id: "connectorMaturity.history.change", message: "{subject} / {operation} · {before} → {after}" },
        {
          subject: connectorName(subject),
          operation: operationName(operation),
          before: (before && states[before]) ?? before ?? absent,
          after: (after && states[after]) ?? after ?? absent,
        },
      ),
  };
}

export default function ConnectorMaturity() {
  const { siteConfig } = useDocusaurusContext();
  const evidenceBaseUrl =
    "https://github.com/neokapi/neokapi/blob/" + siteConfig.customFields?.sourceRevision + "/";
  const dataBaseUrl = useBaseUrl("/data/");
  const history = observations as Observation[];
  const coverage = coverageLabels();
  const title = translate({ id: "connectorMaturity.title", message: "Connector maturity" });
  return (
    <Layout
      title={title}
      description={translate({
        id: "connectorMaturity.description",
        message: "The operations each connector has passed against a fixture endpoint.",
      })}
    >
      <main className={styles.page}>
        <header className={styles.header}>
          <h1>{title}</h1>
          <p>
            {translate({
              id: "connectorMaturity.intro",
              message:
                "A connector moves content between a workspace and the system that holds it. Each row names an operation and the test that runs it against a fixture endpoint. Check a failed or unverified operation before your team relies on it.",
            })}
          </p>
          <p className={styles.date}>
            {translate(
              { id: "connectorMaturity.measured", message: "Last measured {date}" },
              { date: current.date },
            )}
          </p>
        </header>
        <CapabilityInventory
          subjects={current.connectors}
          labels={inventoryLabels()}
          evidenceBaseUrl={evidenceBaseUrl}
        />
        <ObservationList
          heading={translate({ id: "connectorMaturity.history.heading", message: "Observation history" })}
          intro={translate({
            id: "connectorMaturity.history.intro",
            message: "Each observation compares with the preceding published date.",
          })}
          observations={history.map((entry, index) => ({
            date: entry.date,
            items: [
              {
                label: translate({ id: "connectorMaturity.history.connectors", message: "Connectors" }),
                value: (
                  <CapabilityCoverage
                    profile={entry.connectors}
                    previous={history[index - 1]?.connectors}
                    labels={coverage}
                  />
                ),
              },
            ],
          }))}
          datasetsHeading={translate({ id: "connectorMaturity.history.data", message: "Source data" })}
          datasets={[
            {
              href: dataBaseUrl + "connector-capabilities.json",
              label: translate({
                id: "connectorMaturity.history.current",
                message: "Current connector evidence (JSON)",
              }),
            },
            {
              href: dataBaseUrl + "connector-capabilities-history.json",
              label: translate({
                id: "connectorMaturity.history.all",
                message: "Historical observations (JSON)",
              }),
            },
          ]}
        />
      </main>
    </Layout>
  );
}
