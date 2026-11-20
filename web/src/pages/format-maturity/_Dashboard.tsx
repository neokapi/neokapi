import { useSyncExternalStore } from "react";
import {
  ArrowRight,
  ArrowUpRight,
  BookOpen,
  CalendarDays,
  CircleAlert,
  FileOutput,
  Files,
  ListChecks,
  Plug,
} from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import formats from "../../../static/data/format-maturity.json";
import capabilities from "../../../static/data/content-capabilities.json";
import history from "../../../static/data/content-capabilities-history.json";
import Formats from "./_Formats";
import { TIER_LABEL, TIER_MEANING, TIER_ORDER } from "./_types";
import CapabilityEvidence, { ObservationHistory } from "./_CapabilityEvidence";
import styles from "./index.module.css";

const views = [
  { id: "overview", label: t("Overview", "dashboard view") },
  { id: "format-evidence", label: t("Formats", "dashboard view") },
  { id: "outputs", label: t("Outputs", "dashboard view") },
  { id: "connectors", label: t("Connectors", "dashboard view") },
  { id: "observation-history", label: t("History", "dashboard view") },
] as const;

type View = (typeof views)[number]["id"];
const focusViews: Record<string, { view: "outputs" | "connectors"; status?: string }> = {
  "capability-evidence": { view: "outputs" },
  "failed-outputs": { view: "outputs", status: "failed" },
  "unverified-outputs": { view: "outputs", status: "unverified" },
  "failed-connectors": { view: "connectors", status: "failed" },
  "unverified-connectors": { view: "connectors", status: "unverified" },
};

export const dashboardAnchors = [
  ...views.map((view) => view.id),
  ...Object.keys(focusViews),
  ...TIER_ORDER.map((tier) => "tier/" + tier),
  ...formats.formats.map((format) => "format/" + format.id),
];
const subscribe = (notify: () => void) => {
  window.addEventListener("hashchange", notify);
  return () => window.removeEventListener("hashchange", notify);
};
const locationHash = () => window.location.hash.slice(1);
const serverHash = () => "";

const inventory = [
  {
    id: "format-evidence",
    label: t("Input formats", "inventory count"),
    count: formats.formats.length,
    icon: Files,
  },
  {
    id: "outputs",
    label: t("Output targets", "inventory count"),
    count: capabilities.outputs.length,
    icon: FileOutput,
  },
  {
    id: "connectors",
    label: t("Connectors", "inventory count"),
    count: capabilities.connectors.length,
    icon: Plug,
  },
];

const tiers = [
  ...TIER_ORDER.map((id) => ({ id, label: TIER_LABEL[id], color: styles[id] })),
  { id: "unlisted" as const, label: t("Unlisted", "support tier"), color: styles.unlisted },
]
  .map((tier) => ({
    ...tier,
    count: formats.formats.filter((f) => (f.tier?.declared ?? "unlisted") === tier.id).length,
  }))
  .filter((tier) => tier.id !== "unlisted" || tier.count > 0);

const issueLabels = {
  outputs: {
    failed: t("Failed output checks", "dashboard review filter"),
    unverified: t("Unverified output operations", "dashboard review filter"),
  },
  connectors: {
    failed: t("Failed connector checks", "dashboard review filter"),
    unverified: t("Unverified connector operations", "dashboard review filter"),
  },
};
const issues = (["failed", "unverified"] as const).flatMap((status) =>
  (["outputs", "connectors"] as const)
    .map((kind) => ({
      id: status + "-" + kind,
      label: issueLabels[kind][status],
      count: capabilities[kind]
        .flatMap((row) => row.capabilities)
        .filter((check) => check.status === status).length,
      icon: status === "failed" ? CircleAlert : ListChecks,
      color: status === "failed" ? styles.failed : styles.unverified,
    }))
    .filter((issue) => issue.count > 0),
);

function RecentChanges() {
  const latest = history.at(-1);
  const previous = history.at(-2);
  if (!latest || !previous) return null;
  const before: Record<string, string | undefined> = previous.formats;
  const now: Record<string, string | undefined> = latest.formats;
  const ids = Object.keys(now).sort();
  const added = ids.filter((id) => !before[id]);
  const moved = ids.filter((id) => before[id] && before[id] !== now[id]);
  const removed = Object.keys(before).filter((id) => !now[id]);
  return (
    <section className={styles.recent}>
      <div className={styles.sectionHeading}>
        <h2>{t("Latest changes", "dashboard heading")}</h2>
        <a href="#observation-history">
          {t("View history", "dashboard link")} <ArrowRight size={15} aria-hidden="true" />
        </a>
      </div>
      <p className={styles.comparison}>
        {t("Since {date}", "date of preceding observation", { date: previous.date })}
      </p>
      <div className={styles.changeItems}>
        {added.length > 0 && (
          <span>
            {t("{count, plural, one {# new format} other {# new formats}}", "format changes", {
              count: added.length,
            })}{" "}
            {added.slice(0, 3).map((id) => (
              <a className={styles.formatLink} key={id} href={"#format/" + id}>
                {id}
              </a>
            ))}
            {added.length > 3 && "…"}
          </span>
        )}
        {moved.length > 0 && (
          <span>
            {t(
              "{count, plural, one {# implementation grade changed} other {# implementation grades changed}}",
              "format changes",
              { count: moved.length },
            )}
          </span>
        )}
        {removed.length > 0 && (
          <span>
            {t(
              "{count, plural, one {# format removed} other {# formats removed}}",
              "format changes",
              { count: removed.length },
            )}
          </span>
        )}
        {added.length + moved.length + removed.length === 0 && (
          <span>
            {t("No changes to format coverage or implementation grades.", "latest audit")}
          </span>
        )}
      </div>
    </section>
  );
}

function Overview() {
  return (
    <>
      <div className={styles.cards}>
        {inventory.map(({ id, label, count, icon: Icon }) => (
          <a className={styles.card} key={id} href={"#" + id}>
            <div className={styles.cardLabel}>
              <Icon size={20} aria-hidden="true" />
              {label}
            </div>
            <div className={styles.cardValue}>
              <strong>{count}</strong>
              <ArrowRight size={19} aria-hidden="true" />
            </div>
          </a>
        ))}
      </div>
      <div className={styles.overviewPanels}>
        <section className={styles.panel}>
          <h2>{t("Format support", "dashboard heading")}</h2>
          <div className={styles.tierBar} aria-hidden="true">
            {tiers
              .filter((tier) => tier.count)
              .map((tier) => (
                <span key={tier.id} className={tier.color} style={{ flex: tier.count }} />
              ))}
          </div>
          <dl className={styles.tiers}>
            {tiers.map((tier) => (
              <div key={tier.id}>
                <dt>
                  <i aria-hidden="true" className={tier.color} />
                  {tier.id === "unlisted" ? (
                    tier.label
                  ) : (
                    <a href={"#tier/" + tier.id} title={TIER_MEANING[tier.id]}>
                      {tier.label}
                    </a>
                  )}
                </dt>
                <dd>{tier.count}</dd>
              </div>
            ))}
          </dl>
        </section>
        <section className={styles.panel}>
          <h2>{t("Checks to review", "dashboard heading")}</h2>
          {issues.length ? (
            <ul className={styles.issues}>
              {issues.map(({ id, label, count, icon: Icon, color }) => (
                <li key={id}>
                  <a href={"#" + id}>
                    <Icon className={color} size={19} aria-hidden="true" />
                    <span>{label}</span> <strong>{count}</strong>
                    <ArrowRight size={16} aria-hidden="true" />
                  </a>
                </li>
              ))}
            </ul>
          ) : (
            <p>{t("No failed or unverified operations.", "empty review list")}</p>
          )}
        </section>
      </div>
      <RecentChanges />
    </>
  );
}

export default function Dashboard({
  dataBaseUrl,
  evidenceBaseUrl,
  guideUrl,
}: {
  dataBaseUrl: string;
  evidenceBaseUrl: string;
  guideUrl: string;
}) {
  const hash = useSyncExternalStore(subscribe, locationHash, serverHash);
  const focus = Object.hasOwn(focusViews, hash) ? focusViews[hash] : undefined;
  const format = hash.startsWith("format/")
    ? formats.formats.find((row) => row.id === hash.slice(7))
    : undefined;
  const tier = TIER_ORDER.find((id) => hash === "tier/" + id);
  const view: View =
    format || tier
      ? "format-evidence"
      : (focus?.view ?? views.find((item) => item.id === hash)?.id ?? "overview");
  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <div>
          <h1>{t("Format maturity", "dashboard title")}</h1>
          <p className={styles.date}>
            <CalendarDays size={15} aria-hidden="true" />
            {t("Last measured {date}", "maturity snapshot", { date: formats.generated_at })}
          </p>
        </div>
        <a className={styles.guide} href={guideUrl}>
          <BookOpen size={16} aria-hidden="true" />
          {t("Scoring guide", "dashboard link")}
          <ArrowUpRight size={14} aria-hidden="true" />
        </a>
      </header>
      <nav className={styles.views} aria-label={t("Maturity views", "dashboard navigation")}>
        {views.map((item) => (
          <a
            key={item.id}
            href={"#" + item.id}
            aria-current={item.id === view ? "page" : undefined}
          >
            {item.label}
          </a>
        ))}
      </nav>
      <div className={styles.content} id={view}>
        {view === "overview" && <Overview />}
        {view === "format-evidence" && (
          <Formats key={hash} initialSearch={format?.id} initialTier={tier} />
        )}
        {(view === "outputs" || view === "connectors") && (
          <CapabilityEvidence
            key={hash}
            group={view}
            initialStatus={focus?.status}
            evidenceBaseUrl={evidenceBaseUrl}
          />
        )}
        {view === "observation-history" && <ObservationHistory dataBaseUrl={dataBaseUrl} />}
      </div>
    </main>
  );
}
