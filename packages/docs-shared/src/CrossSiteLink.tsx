import useDocusaurusContext from "@docusaurus/useDocusaurusContext";

interface CrossSiteLinkProps {
  to: string;
  children: React.ReactNode;
}

/**
 * Link to the kapi/neokapi docs site from the bowrain site.
 * Uses the `kapiWebSite` custom field on docusaurus.config.ts (driven by
 * the `KAPI_WEB_SITE` env var) so the same component works in dev and prod.
 */
export function KapiLink({ to, children }: CrossSiteLinkProps) {
  const { siteConfig } = useDocusaurusContext();
  const base = ((siteConfig.customFields?.kapiWebSite as string) || "/").replace(/\/$/, "");
  const path = to.startsWith("/") ? to : `/${to}`;
  return <a href={`${base}${path}`}>{children}</a>;
}

/**
 * Link to the bowrain docs site from the kapi/neokapi site.
 * Uses the `bowrainWebSite` custom field. Provided for symmetry; rare in
 * practice because the framework docs typically don't mention bowrain.
 */
export function BowrainLink({ to, children }: CrossSiteLinkProps) {
  const { siteConfig } = useDocusaurusContext();
  const base = ((siteConfig.customFields?.bowrainWebSite as string) || "/").replace(/\/$/, "");
  const path = to.startsWith("/") ? to : `/${to}`;
  return <a href={`${base}${path}`}>{children}</a>;
}
