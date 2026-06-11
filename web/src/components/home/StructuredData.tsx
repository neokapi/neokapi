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
    "neokapi is an open-source, format-aware content engine in Go. It parses localization, " +
    "document, and data formats into a faithful content model, then translates, leverages " +
    "translation memory, and runs verification checks for terminology, QA, and brand voice — " +
    "for content written by people or AI agents.";

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
        "kapi is the command-line and desktop application built on the neokapi engine: extract, " +
        "translate, run checks, and manage .kapi projects.",
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
