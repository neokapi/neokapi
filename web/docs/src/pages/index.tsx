import clsx from "clsx";
import Link from "@docusaurus/Link";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";

import styles from "./index.module.css";

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <header className={clsx("hero hero--primary", styles.heroBanner)}>
      <div className="container">
        <img src="/img/hero-logo.png" alt="neokapi" className={styles.heroLogo} />
        <Heading as="h1" className={clsx("hero__title", styles.heroTitle)}>
          {siteConfig.title} &mdash; {siteConfig.tagline}
        </Heading>
        <div className={styles.buttons}>
          <Link className="button button--secondary button--lg" to="/getting-started/introduction">
            Get Started
          </Link>
        </div>
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
    title: "Formats & plugins",
    description:
      "Readers and writers for localization, document, data, subtitle, and office formats, extended by crash-isolated gRPC plugins and a bridge to the Java Okapi filters.",
    link: "/features/formats",
    linkText: "Formats",
  },
  {
    title: "AI-native tools",
    description:
      "LLM-assisted translation, QA, terminology, and review compose in the same pipeline as machine-translation backends and rule-based checks.",
    link: "/features/ai-translation",
    linkText: "AI translation",
  },
  {
    title: "Streaming pipeline",
    description:
      "Each tool runs in its own goroutine, connected by buffered channels with backpressure and context cancellation.",
    link: "/developer/architecture",
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
        <Heading as="h2" className="text--center margin-bottom--lg">
          Neokapi Framework
        </Heading>
        <p className="text--center margin-bottom--lg">
          Open-source localization engine and <Link to="/kapi-cli/overview">kapi CLI</Link> for
          standalone file processing.
        </p>
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
    <Layout title={siteConfig.title} description="Open, AI-native localization platform in Go">
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
