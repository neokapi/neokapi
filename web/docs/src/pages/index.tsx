import clsx from "clsx";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";
import { Sparkles } from "lucide-react";
import TryNeokapi from "../components/TryNeokapi";

import styles from "./index.module.css";

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <header className={clsx("hero", styles.heroBanner)}>
      <div className={clsx("container", styles.heroGrid)}>
        <div className={styles.heroIntro}>
          <img src={useBaseUrl("/img/hero-logo.png")} alt="neokapi" className={styles.heroLogo} />
          <Heading as="h1" className={clsx("hero__title", styles.heroTitle)}>
            {siteConfig.title} &mdash; {siteConfig.tagline}
          </Heading>
          <p className={styles.heroSubtitle}>
            An open-source engine in Go that parses localization, document, and data formats into a
            faithful content model &mdash; then translates it, leverages memory, and runs checks for
            terminology, QA, and brand voice, whether a person or an agent wrote it.
          </p>
          <div className={styles.buttons}>
            <Link
              className={clsx("button button--lg", styles.tryButton)}
              to="/kapi/get-started/use-with-claude"
            >
              <Sparkles size={18} aria-hidden="true" />
              Try Kapi with Claude
            </Link>
            <Link className="button button--secondary button--lg" to="/kapi/overview">
              Get Started
            </Link>
          </div>
        </div>
        <div className={styles.heroAside}>
          <TryNeokapi />
        </div>
      </div>
    </header>
  );
}

type Tier = {
  eyebrow: string;
  title: string;
  description: string;
  link: string;
  linkText: string;
};

// The three tiers of the project: the Go framework, and the kapi + kapi-react
// surfaces built on top of it. Stated once here so the homepage frames the whole
// shape; deeper pages cover each tier.
const Tiers: Tier[] = [
  {
    eyebrow: "Framework",
    title: "A Go library",
    description:
      "The engine itself: format-aware readers and writers, a faithful content model, and a streaming pipeline of composable tools — embed it directly in your own Go programs.",
    link: "/framework/go-quickstart",
    linkText: "Go quickstart",
  },
  {
    eyebrow: "Surface",
    title: "kapi — CLI & desktop",
    description:
      "Drive the framework from the command line or a visual desktop app: extract, translate, run checks, and manage .kapi projects — no code required.",
    link: "/kapi/overview",
    linkText: "Use kapi",
  },
  {
    eyebrow: "Surface",
    title: "kapi-react — in the browser",
    description:
      "The same engine compiled to WebAssembly, wrapped in React components, so you can run format-aware tools and explorers directly in a web page.",
    link: "/lab",
    linkText: "Open the lab",
  },
];

function TierCard({ eyebrow, title, description, link, linkText }: Tier) {
  return (
    <div className="col col--4">
      <div className="text--center padding-horiz--md padding-vert--md">
        <span className={styles.tierEyebrow}>{eyebrow}</span>
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
        <Link to={link}>{linkText} &rarr;</Link>
      </div>
    </div>
  );
}

type ProductItem = {
  title: string;
  description: string;
  link: string;
  linkText: string;
};

const NeokapiFeatures: ProductItem[] = [
  {
    title: "Formats & plugins",
    description:
      "Readers and writers for localization, document, data, subtitle, and office formats, extended by plugins and a bridge to the Java Okapi filters.",
    link: "/framework/formats",
    linkText: "Formats",
  },
  {
    title: "AI-native tools",
    description:
      "LLM-assisted translation, QA, terminology, and review compose in the same pipeline as machine-translation backends and rule-based checks.",
    link: "/framework/ai-translation",
    linkText: "AI translation",
  },
  {
    title: "Streaming pipeline",
    description:
      "Tools run in parallel and stream results as each part is ready, so large files and many languages process fast.",
    link: "/framework/architecture",
    linkText: "Architecture",
  },
  {
    title: "Interchange with any TMS",
    description:
      "Extract bilingual XLIFF 2.x or PO for Trados, memoQ, Phrase, or Crowdin, merge the translation back through a faithful skeleton, and keep translation memory and terminology in the loop.",
    link: "/kapi/bilingual-workflow",
    linkText: "Interchange",
  },
  {
    title: "Project model",
    description:
      "Capture languages, content patterns, and flows once in a committed .kapi recipe; run flows with no repeated flags. Translation memory accumulates, and git-style discovery finds the project from any subdirectory.",
    link: "/kapi/get-started/first-project",
    linkText: "Create a project",
  },
];

function ProductCard({ title, description, link, linkText }: ProductItem) {
  return (
    <div className={clsx("col col--4")}>
      <div className="text--center padding-horiz--md padding-vert--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
        <Link to={link}>{linkText} &rarr;</Link>
      </div>
    </div>
  );
}

function HomepageFeatures() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">One engine, three ways to use it</Heading>
          <p className={styles.sectionSubtitle}>
            neokapi is a Go framework. <strong>kapi</strong> and <strong>kapi-react</strong> are
            surfaces on top of it.
          </p>
        </div>
        <div className="row margin-bottom--xl">
          {Tiers.map((props, idx) => (
            <TierCard key={idx} {...props} />
          ))}
        </div>
        <div className="row margin-bottom--xl">
          {NeokapiFeatures.map((props, idx) => (
            <ProductCard key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}

export default function Home() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="Open, AI-native, format-aware content engine in Go"
    >
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
