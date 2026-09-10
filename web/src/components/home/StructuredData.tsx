import Head from "@docusaurus/Head";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";

// Schema.org JSON-LD for the home page. Makes the site legible to search
// engines (rich results) and to agents/crawlers that read structured metadata:
// what the project is, that it's free and open source, where the repo and docs
// live, and which platforms it runs on. Emitted only on the home page.
export default function StructuredData() {
  const { siteConfig } = useDocusaurusContext();
  const siteUrl = siteConfig.url + siteConfig.baseUrl.replace(/\/$/, "");
  const description =
    "neokapi is the open content engine for people and AI agents. Read, edit and check " +
    "content across formats, and use kapi to apply your project's terms and writing guidance.";

  const graph = [
    {
      "@type": "SoftwareSourceCode",
      name: "neokapi",
      description,
      url: siteUrl,
      codeRepository: "https://github.com/neokapi/neokapi",
      programmingLanguage: "Go",
      license: "https://www.apache.org/licenses/LICENSE-2.0",
    },
    {
      "@type": "SoftwareApplication",
      name: "kapi",
      applicationCategory: "DeveloperApplication",
      operatingSystem: "macOS, Windows, Linux",
      description:
        "kapi is the project tool built on the neokapi engine: read, edit, " +
        "run configured checks, and inspect the guidance that applies to your files.",
      url: siteUrl,
      downloadUrl: "https://github.com/neokapi/neokapi/releases",
      softwareHelp: siteUrl,
      offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
    },
    {
      "@type": "WebSite",
      name: siteConfig.title,
      url: siteUrl,
      description,
    },
  ];

  const jsonLd = { "@context": "https://schema.org", "@graph": graph };

  return (
    <Head>
      <script type="application/ld+json">{JSON.stringify(jsonLd)}</script>
    </Head>
  );
}
