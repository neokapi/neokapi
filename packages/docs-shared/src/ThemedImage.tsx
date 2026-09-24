import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { readCdnConfig, cdnEnabled, cdnHref } from "./cdn";
import "./ThemedImage.css";

interface ThemedImageProps {
  alt: string;
  sources: {
    light: string;
    dark: string;
  };
  className?: string;
}

// A theme-aware image, using the same CSS switching as ThemedVideo. Both variants
// are rendered and selected through Docusaurus's data-theme attribute; identical
// variants render one image. Avoid useColorMode here because workspace consumers
// can resolve a separate @docusaurus/theme-common instance without its provider.
//
// Paths use the configured CDN origin, or useBaseUrl when no CDN is configured.
// This supports non-root site deployments and keeps large screenshots outside
// Pages and preview bundles. CDN images are published by make publish-cdn-images
// and publish-cdn-bowrain-images. useBaseUrl must run unconditionally to preserve
// hook order, even when the CDN path takes precedence.
export default function ThemedImage({ alt, sources, className }: ThemedImageProps) {
  const { siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const onCdn = cdnEnabled(cdn);
  const lightLocal = useBaseUrl(sources.light);
  const darkLocal = useBaseUrl(sources.dark);
  const light = onCdn ? cdnHref(cdn, sources.light) : lightLocal;
  const dark = onCdn ? cdnHref(cdn, sources.dark) : darkLocal;
  const cls = className ? ` ${className}` : "";

  if (light === dark) {
    return <img className={`themed-img${cls}`} src={light} alt={alt} loading="lazy" />;
  }

  return (
    <>
      <img className={`themed-img themed-img--light${cls}`} src={light} alt={alt} loading="lazy" />
      <img className={`themed-img themed-img--dark${cls}`} src={dark} alt={alt} loading="lazy" />
    </>
  );
}
