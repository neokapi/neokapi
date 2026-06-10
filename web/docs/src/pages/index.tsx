import clsx from "clsx";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";
import { Sparkles } from "lucide-react";
import TryNeokapi from "../components/TryNeokapi";
import Differentiators from "../components/home/Differentiators";
import Pipeline from "../components/home/Pipeline";
import Formats from "../components/home/Formats";
import OkapiMapping from "../components/home/OkapiMapping";
import GetStarted from "../components/home/GetStarted";
import StructuredData from "../components/home/StructuredData";

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

// Two ways to USE the engine: directly as a Go library, or through kapi (the
// CLI + desktop app built on it). kapi-react is a separate React i18n library
// that lives in the same codebase — highlighted on its own below, not framed as
// a third "way to use the engine".
const Tiers: Tier[] = [
  {
    eyebrow: "The engine",
    title: "Go framework",
    description:
      "Format-aware readers and writers, a faithful content model, and a streaming pipeline of composable tools — embed it directly in your own Go programs.",
    link: "/framework/go-quickstart",
    linkText: "Go quickstart",
  },
  {
    eyebrow: "Built on it",
    title: "kapi — CLI & desktop",
    description:
      "Drive the engine from the command line or a visual desktop app: extract, translate, run checks, and manage .kapi projects — no code required.",
    link: "/kapi/overview",
    linkText: "Use kapi",
  },
];

function TierCard({ eyebrow, title, description, link, linkText }: Tier) {
  return (
    <div className="col col--6">
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
          <Heading as="h2">One engine, two ways to use it</Heading>
          <p className={styles.sectionSubtitle}>
            neokapi is a Go framework. Use it directly as a library, or through{" "}
            <strong>kapi</strong> — the CLI and desktop app built on it.
          </p>
        </div>
        <div className="row margin-bottom--lg">
          {Tiers.map((props, idx) => (
            <TierCard key={idx} {...props} />
          ))}
        </div>
        <div className="row margin-bottom--xl">
          <div className="col col--10 col--offset-1">
            <div className={styles.familyRow}>
              <Link to="/react/introduction" className={styles.reactCallout}>
                <span className={styles.reactCalloutBadge}>In the family</span>
                <span className={styles.reactCalloutText}>
                  <strong>kapi-react</strong> — a zero-config i18n library for React. Its own
                  framework, powered by neokapi under the hood for build-time string extraction and
                  catalog compilation.
                </span>
                <span className={styles.reactCalloutArrow} aria-hidden="true">
                  &rarr;
                </span>
              </Link>
              <Link
                to="/toolbox/overview"
                className={`${styles.reactCallout} ${styles.toolboxCallout}`}
              >
                <span className={styles.reactCalloutBadge}>In the family</span>
                <span className={styles.reactCalloutText}>
                  <strong>CLI Tools</strong> — <code>kgrep</code>, <code>ksed</code>,{" "}
                  <code>kcat</code>: format-aware grep, sed and cat that operate on the translatable
                  text inside <code>.docx</code>, JSON, XLIFF and more.
                </span>
                <span className={styles.reactCalloutArrow} aria-hidden="true">
                  &rarr;
                </span>
              </Link>
            </div>
          </div>
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
      description="An open-source, format-aware content engine in Go. Extract, translate, leverage translation memory, and run terminology, QA, and brand-voice checks — for content written by people or AI agents."
    >
      <StructuredData />
      <HomepageHeader />
      <main>
        <HomepageFeatures />
        <Pipeline />
        <Differentiators />
        <Formats />
        <OkapiMapping />
        <GetStarted />
      </main>
    </Layout>
  );
}
