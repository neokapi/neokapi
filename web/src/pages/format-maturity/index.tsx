import Layout from "@theme/Layout";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useBrokenLinks from "@docusaurus/useBrokenLinks";
import { t } from "@neokapi/i18n-react/runtime";
import Dashboard, { dashboardAnchors } from "./_Dashboard";

export default function FormatMaturity() {
  const { siteConfig } = useDocusaurusContext();
  const evidenceBaseUrl =
    "https://github.com/neokapi/neokapi/blob/" + siteConfig.customFields?.sourceRevision + "/";
  const dataBaseUrl = useBaseUrl("/data/");
  const guideUrl = useBaseUrl("/framework/format-maturity");
  const links = useBrokenLinks();
  for (const anchor of dashboardAnchors) links.collectAnchor(anchor);
  return (
    <Layout
      title={t("Format maturity", "dashboard title")}
      description={t(
        "Support tiers, conversion results and connector checks.",
        "maturity dashboard description",
      )}
    >
      <Dashboard evidenceBaseUrl={evidenceBaseUrl} dataBaseUrl={dataBaseUrl} guideUrl={guideUrl} />
    </Layout>
  );
}
