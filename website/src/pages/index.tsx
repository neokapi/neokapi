import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <Heading as="h1" className="hero__title">
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle">{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/getting-started/introduction">
            Get Started
          </Link>
        </div>
      </div>
    </header>
  );
}

type FeatureItem = {
  title: string;
  description: string;
};

const FeatureList: FeatureItem[] = [
  {
    title: '15 Built-in Formats',
    description:
      'HTML, XML, XLIFF, JSON, YAML, PO, Markdown, and more. Extensible via plugins for any format.',
  },
  {
    title: 'Channel-based Pipeline',
    description:
      'Concurrent streaming pipeline using Go channels and goroutines. Each tool runs in its own goroutine with automatic backpressure.',
  },
  {
    title: 'AI-native Translation',
    description:
      'First-class LLM integration with Anthropic, OpenAI, and Ollama. AI translation, QA, and terminology tools compose in the same pipeline.',
  },
  {
    title: 'Plugin System',
    description:
      'Crash-isolated gRPC plugins in any language. Java bridge provides access to 40+ Okapi filters without rewriting.',
  },
  {
    title: 'Translation Memory',
    description:
      'Built-in Pensieve TM with Levenshtein fuzzy matching, SQLite persistence, and TMX import/export.',
  },
  {
    title: 'Desktop App',
    description:
      'Bowrain: a cross-platform desktop app built with Wails v3, React, and TypeScript for visual translation editing.',
  },
];

function Feature({title, description}: FeatureItem) {
  return (
    <div className={clsx('col col--4')}>
      <div className="text--center padding-horiz--md padding-vert--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

function HomepageFeatures() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}

export default function Home(): JSX.Element {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="AI-native localization framework in Go">
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
