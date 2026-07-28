import clsx from "clsx";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";
import { Sparkles } from "lucide-react";
import TryNeokapi from "../components/TryNeokapi";
import StructuredData from "../components/home/StructuredData";
import AuthorsNote from "../components/home/AuthorsNote";

import styles from "./index.module.css";

function HomepageHeader() {
  return (
    <header className={clsx("hero", styles.heroBanner)}>
      <div className={clsx("container", styles.heroGrid)}>
        <div className={styles.heroIntro}>
          <img src={useBaseUrl("/img/hero-logo.png")} alt="neokapi" className={styles.heroLogo} />
          <Heading as="h1" className={clsx("hero__title", styles.heroTitle)}>
            Say the same thing the same way &mdash; in every file, and every language.
          </Heading>
          <p className={styles.heroSubtitle}>
            Your project&rsquo;s context, where your tools and your AI can read it. kapi parses any
            format into one content model, resolves what applies to each piece of content, and{" "}
            <strong>
              writes the file back byte&#8209;for&#8209;byte: every tag, placeholder, and structure
              preserved
            </strong>
            .
          </p>
          <div className={styles.buttons}>
            <Link
              className="button button--lg button--primary"
              to="/kapi/get-started/first-project"
            >
              Get started
            </Link>
            <Link
              className={clsx("button button--secondary button--lg", styles.tryButton)}
              to="/kapi/get-started/use-with-claude"
            >
              <Sparkles size={18} aria-hidden="true" />
              Use with Claude
            </Link>
          </div>
        </div>
        <div className={styles.heroAside}>
          <TryNeokapi />
        </div>
      </div>
      <div className="container">
        <AuthorsNote />
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
// CLI + desktop app built on it). neokapi-i18n is a separate React i18n library
// that lives in the same codebase — highlighted on its own below, not framed as
// a third "way to use the engine".
const Tiers: Tier[] = [
  {
    eyebrow: "The engine",
    title: "Go framework",
    description:
      "Format-aware readers and writers, a unified content model, and a streaming pipeline of composable tools — embed it directly in your own Go programs.",
    link: "/framework/go-quickstart",
    linkText: "Go quickstart",
  },
  {
    eyebrow: "Built on it",
    title: "kapi — binary, agent skill, MCP server",
    description:
      "One binary plus the two surfaces your assistant reaches it through. kapi holds your project's context and answers what applies here — to your checks, to you, and to whichever AI you already use. Free forever.",
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
    title: "One model, any format",
    description:
      "kapi reads your real files — JSON, Markdown, HTML, config, .docx — into one unified content model, and writes the originals back unchanged except for the text you touched.",
    link: "/framework/formats",
    linkText: "Formats",
  },
  {
    title: "Your project's context",
    description:
      "A legal notice is not a help article. kapi holds the terms, the voice and the rules your project actually goes by, resolves what applies to a given piece of content, and serves that answer to your checks, to you, and to your AI assistant.",
    link: "/kapi/overview",
    linkText: "How it works",
  },
  {
    title: "Edit it — you or your AI",
    description:
      "Rewrite the text in place with every tag and placeholder intact, and check it against the context that holds there — like tests for AI output, with a pass/fail gate. Loop with your assistant until it passes, then ship.",
    link: "/framework/checks",
    linkText: "Checks",
  },
  {
    title: "Every language — and you can trust it",
    description:
      "One command — kapi up — catches every language up to its ship gates and parks what needs a person. You review, edit, and approve in the desktop Review surface; approvals stick, and only what changed is re-done. The gate check runs in CI.",
    link: "/kapi/get-started/add-languages",
    linkText: "Go multilingual",
  },
  {
    title: "Measured, not asserted",
    description:
      "We don't claim format support — we measure it. The format-maturity and benchmark dashboards show whether it holds, under load and per format.",
    link: "/format-maturity",
    linkText: "See the dashboards",
  },
  {
    title: "Open source, headless",
    description:
      "Open source, Apache-2.0, written in Go. Format-agnostic, agent-drivable, headless: a content layer you or your AI can drive.",
    link: "/framework/architecture",
    linkText: "Architecture",
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
          <Heading as="h2">One context, every file</Heading>
          <p className={styles.sectionSubtitle}>
            Parse it, resolve what applies to it, check it, write it back — the same engine, end to
            end, in one language or twenty.
          </p>
        </div>
        <div className="row margin-bottom--xl">
          {NeokapiFeatures.map((props, idx) => (
            <ProductCard key={idx} {...props} />
          ))}
        </div>
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">Use it your way</Heading>
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
                  <strong>neokapi-i18n</strong> — a zero-config i18n library for React. Its own
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
                  <code>kcat</code>: format-aware grep, sed and cat that read and rewrite the text
                  inside office files, JSON, HTML and more.
                </span>
                <span className={styles.reactCalloutArrow} aria-hidden="true">
                  &rarr;
                </span>
              </Link>
            </div>
          </div>
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
      description="An open-source, format-aware content engine in Go. Parse any format, resolve the context that applies to the content inside it, edit and check it — you or your AI agent — and write it back byte-for-byte. The same engine makes that content work in every language."
    >
      <StructuredData />
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
