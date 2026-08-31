import clsx from "clsx";
import Link from "@docusaurus/Link";
import Translate, { translate } from "@docusaurus/Translate";
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
          <img
            src={useBaseUrl("/img/hero-logo.png")}
            alt={translate({ id: "home.hero.logoAlt", message: "neokapi" })}
            className={styles.heroLogo}
          />
          <Heading as="h1" className={clsx("hero__title", styles.heroTitle)}>
            <Translate id="home.hero.title">
              Contextually consistent content, in any format.
            </Translate>
          </Heading>
          <p className={styles.heroSubtitle}>
            <Translate id="home.hero.subtitle.lead">
              kapi holds your project’s context (its terms, its voice, the rules it goes by) and
              applies it inside your real files. Any format in,
            </Translate>{" "}
            <strong>
              <Translate id="home.hero.subtitle.strong">the original back byte‑for‑byte</Translate>
            </strong>
            <Translate id="home.hero.subtitle.tail">, every tag and placeholder intact.</Translate>
          </p>
          <div className={styles.buttons}>
            <Link
              className="button button--lg button--primary"
              to="/kapi/get-started/first-project"
            >
              <Translate id="home.cta.getStarted">Get started</Translate>
            </Link>
            <Link
              className={clsx("button button--secondary button--lg", styles.tryButton)}
              to="/kapi/get-started/use-with-claude"
            >
              <Sparkles size={18} aria-hidden="true" />
              <Translate id="home.cta.useWithClaude">Use with Claude</Translate>
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
// translate() rather than <Translate>: a literal in a const never reaches the
// JSX walker, so write-translations would not see it.
const Tiers: Tier[] = [
  {
    eyebrow: translate({
      id: "home.tiers.engine.eyebrow",
      message: "The engine",
    }),
    title: translate({
      id: "home.tiers.engine.title",
      message: "Go framework",
    }),
    description: translate({
      id: "home.tiers.engine.description",
      message:
        "Format-aware readers and writers, a unified content model, and a streaming pipeline of composable tools; embed it directly in your own Go programs.",
    }),
    link: "/framework/go-quickstart",
    linkText: translate({
      id: "home.tiers.engine.linkText",
      message: "Go quickstart",
    }),
  },
  {
    eyebrow: translate({
      id: "home.tiers.kapi.eyebrow",
      message: "Built on it",
    }),
    title: translate({
      id: "home.tiers.kapi.title",
      message: "kapi: binary, agent skill, MCP server",
    }),
    description: translate({
      id: "home.tiers.kapi.description",
      message:
        "One binary plus the two surfaces your AI agent reaches it through. kapi holds your project's context and answers what applies here, to your checks, to you, and to whichever AI you already use. Free forever.",
    }),
    link: "/kapi/overview",
    linkText: translate({
      id: "home.tiers.kapi.linkText",
      message: "Use kapi",
    }),
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
    title: translate({
      id: "home.feat.model.title",
      message: "One model, any format",
    }),
    description: translate({
      id: "home.feat.model.description",
      message:
        "kapi reads your real files (JSON, Markdown, HTML, config, .docx) into one unified content model, and writes the originals back unchanged except for the text you touched.",
    }),
    link: "/framework/formats",
    linkText: translate({
      id: "home.feat.model.linkText",
      message: "Formats",
    }),
  },
  {
    title: translate({
      id: "home.feat.context.title",
      message: "Your project's context",
    }),
    description: translate({
      id: "home.feat.context.description",
      message:
        "A legal notice and a help article call for different words. kapi holds the terms, the voice and the rules your project goes by, and answers what applies to a given piece of content: the same answer for your checks, your AI agent, and you.",
    }),
    link: "/kapi/overview",
    linkText: translate({
      id: "home.feat.context.linkText",
      message: "How it works",
    }),
  },
  {
    title: translate({
      id: "home.feat.edit.title",
      message: "Edit it in place",
    }),
    description: translate({
      id: "home.feat.edit.description",
      message:
        "Rewrite the text with every tag and placeholder intact, then check it against the context that holds there: tests for AI output, with a pass/fail gate. Iterate until it passes, then ship.",
    }),
    link: "/framework/checks",
    linkText: translate({
      id: "home.feat.edit.linkText",
      message: "Checks",
    }),
  },
  {
    title: translate({
      id: "home.feat.langs.title",
      message: "Every language, gated",
    }),
    description: translate({
      id: "home.feat.langs.description",
      message:
        "One command, kapi up, catches every language up to its ship gates and parks what needs a person. You review, edit, and approve in the desktop Review surface; approvals stick, and only what changed is re-done. The gate check runs in CI.",
    }),
    link: "/kapi/get-started/add-languages",
    linkText: translate({
      id: "home.feat.langs.linkText",
      message: "Go multilingual",
    }),
  },
  {
    title: translate({
      id: "home.feat.measured.title",
      message: "Measured",
    }),
    description: translate({
      id: "home.feat.measured.description",
      message:
        "Format support is measured rather than claimed. The format-maturity and benchmark dashboards show whether it holds, under load and per format.",
    }),
    link: "/format-maturity",
    linkText: translate({
      id: "home.feat.measured.linkText",
      message: "See the dashboards",
    }),
  },
  {
    title: translate({
      id: "home.feat.oss.title",
      message: "Open source, headless",
    }),
    description: translate({
      id: "home.feat.oss.description",
      message:
        "Open source, Apache-2.0, written in Go. Format-agnostic and headless: a content layer a person or an agent can drive.",
    }),
    link: "/framework/architecture",
    linkText: translate({
      id: "home.feat.oss.linkText",
      message: "Architecture",
    }),
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
          <Heading as="h2">
            <Translate id="home.section.engine.title">Parse it, check it, write it back</Translate>
          </Heading>
          <p className={styles.sectionSubtitle}>
            <Translate id="home.section.engine.subtitle">
              One engine, end to end, in one language or twenty.
            </Translate>
          </p>
        </div>
        <div className="row margin-bottom--xl">
          {NeokapiFeatures.map((props, idx) => (
            <ProductCard key={idx} {...props} />
          ))}
        </div>
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">
            <Translate id="home.section.waysIn.title">Two ways in</Translate>
          </Heading>
          <p className={styles.sectionSubtitle}>
            <Translate id="home.section.waysIn.subtitle.lead">
              neokapi is a Go framework. Use it directly as a library, or through
            </Translate>{" "}
            <strong>kapi</strong>{" "}
            <Translate id="home.section.waysIn.subtitle.tail">
              , the CLI and desktop app built on it.
            </Translate>
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
                <span className={styles.reactCalloutBadge}>
                  <Translate id="home.family.badge">In the family</Translate>
                </span>
                <span className={styles.reactCalloutText}>
                  <strong>neokapi-i18n</strong>{" "}
                  <Translate id="home.family.i18n">
                    , a zero-config i18n library for React. Its own framework, powered by neokapi
                    under the hood for build-time string extraction and catalog compilation.
                  </Translate>
                </span>
                <span className={styles.reactCalloutArrow} aria-hidden="true">
                  &rarr;
                </span>
              </Link>
              <Link
                to="/toolbox/overview"
                className={`${styles.reactCallout} ${styles.toolboxCallout}`}
              >
                <span className={styles.reactCalloutBadge}>
                  <Translate id="home.family.badge2">In the family</Translate>
                </span>
                <span className={styles.reactCalloutText}>
                  <strong>CLI Tools</strong>: <code>kgrep</code>, <code>ksed</code>,{" "}
                  <code>kcat</code>:{" "}
                  <Translate id="home.family.tools">
                    format-aware grep, sed and cat that read and rewrite the text inside office
                    files, JSON, HTML and more.
                  </Translate>
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
      description={translate({
        id: "home.meta.description",
        message:
          "An open-source, format-aware content engine in Go. It holds a project's content context (its terms, voice and rules) and applies it inside real files: any format in, the original back byte-for-byte, in one language or twenty.",
      })}
    >
      <StructuredData />
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
