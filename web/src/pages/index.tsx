import clsx from "clsx";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";
import { Sparkles } from "lucide-react";
import { ThemedVideo } from "@neokapi/docs-shared";
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
            Get your content right. Then get it everywhere.
          </Heading>
          <p className={styles.heroSubtitle}>
            kapi parses any format into one content model, lets you &mdash; or your AI agent &mdash;
            edit the text inside it, and{" "}
            <strong>
              writes it back byte&#8209;for&#8209;byte: every tag, placeholder, and structure
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
              to="/kapi/get-started/use-with-skills"
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
    title: "Edit it — you or your AI",
    description:
      "Rewrite the text in place with every tag and placeholder intact, and check it — brand, terminology, placeholders — like tests for AI output, with a pass/fail gate. Loop with your assistant until it passes, then ship.",
    link: "/framework/checks",
    linkText: "Checks",
  },
  {
    title: "Every language — and you can trust it",
    description:
      "The same content, in every language, translated by AI with structure intact. A native speaker confirms tone and brand once, and kapi remembers it — so it sticks and propagates. On-brand everywhere, only re-doing what changed, gated in CI.",
    link: "/kapi/recipes/pre-translate-with-tm",
    linkText: "Go multilingual",
  },
  {
    title: "Measured, not asserted",
    description:
      "We don't claim format support — we measure it. The parity, test-comparison, and format-maturity dashboards show whether it holds, under load and per format.",
    link: "/parity",
    linkText: "See the dashboards",
  },
  {
    title: "Open by lineage",
    description:
      "Open source, Apache-2.0 — the engine rebuilt in Go from the Okapi Framework lineage. Format-agnostic, agent-drivable, headless: a content layer you or your AI can drive.",
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

function SeeItWork() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">See it work</Heading>
          <p className={styles.sectionSubtitle}>
            Ask in plain language, and the content ships in every language &mdash; structure intact.
          </p>
        </div>
        <div className="row margin-bottom--md">
          <div className="col col--8 col--offset-2">
            <ThemedVideo
              sources={{
                light: "/video/kapi/07-global-launch-many-languages-light.webm",
                dark: "/video/kapi/07-global-launch-many-languages-dark.webm",
              }}
              maxWidth="100%"
            />
          </div>
        </div>
        <div className="text--center">
          <Link to="/kapi/walkthroughs">See more walkthroughs &rarr;</Link>
        </div>
      </div>
    </section>
  );
}

function HomepageFeatures() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">From one file to every language</Heading>
          <p className={styles.sectionSubtitle}>
            Parse it, get it right, then make it work everywhere — the same engine, end to end.
          </p>
        </div>
        <div className="row margin-bottom--xl">
          {NeokapiFeatures.map((props, idx) => (
            <ProductCard key={idx} {...props} />
          ))}
        </div>
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">Run it your way</Heading>
          <p className={styles.sectionSubtitle}>
            Drive <strong>kapi</strong> from the command line or a visual desktop app — extract,
            translate, run checks, and manage projects, no code required.
          </p>
        </div>
        <div className="row margin-bottom--lg">
          <div className="col col--6 col--offset-3">
            <div className="text--center padding-horiz--md padding-vert--md">
              <Heading as="h3">kapi — CLI &amp; desktop</Heading>
              <p>
                The standalone toolchain: a command for every step, and a desktop app to do it
                visually.
              </p>
              <Link to="/kapi/overview">Use kapi &rarr;</Link>
            </div>
          </div>
        </div>
        <div className="row margin-bottom--xl">
          <div className="col col--10 col--offset-1">
            <div className={styles.familyRow}>
              <Link to="/framework/go-quickstart" className={styles.reactCallout}>
                <span className={styles.reactCalloutBadge}>For developers</span>
                <span className={styles.reactCalloutText}>
                  <strong>Go framework</strong> — embed the engine directly: format-aware readers
                  and writers, one content model, and a streaming pipeline of composable tools.
                </span>
                <span className={styles.reactCalloutArrow} aria-hidden="true">
                  &rarr;
                </span>
              </Link>
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
      description="An open-source, format-aware content engine in Go. Parse any format, edit and check the content inside it — you or your AI agent — and write it back byte-for-byte. The same engine makes that content work in every language."
    >
      <StructuredData />
      <HomepageHeader />
      <main>
        <SeeItWork />
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
