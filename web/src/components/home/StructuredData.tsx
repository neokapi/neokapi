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
    "neokapi is an open content engine in Go: it reads documents, data and office files " +
    "into one content model, runs checking and editing tools over them, and writes edits " +
    "back with each file's structure intact. kapi is built on it.";

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
        "kapi keeps a project's writing consistent across people, AI agents and languages: " +
        "it reads the project's files, keeps the rules they are written by in a context graph, " +
        "and checks every edit against it, from the desktop app, the command line or an agent.",
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
