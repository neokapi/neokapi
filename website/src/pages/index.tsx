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
    title: 'Connector-first',
    description:
      'Bidirectional connectors sync content from CMS, design tools, code repos, and marketing platforms. Files are one connector type, not the whole story.',
  },
  {
    title: 'Versioned Content Store',
    description:
      'Content-addressed blocks with SHA-256 identity. Deduplication, version history, and incremental sync that only processes what changed.',
  },
  {
    title: 'AI-native Tools',
    description:
      'LLM-powered translation, QA, terminology, and review compose in the same pipeline as every other tool. Anthropic, OpenAI, Ollama, plus 5 MT services.',
  },
  {
    title: 'Event-driven Automation',
    description:
      'Triggers, quality gates, and webhooks. Content changes flow through rules that run flows, enforce quality, and notify teams.',
  },
  {
    title: '15+ Formats & Plugins',
    description:
      'HTML, XML, XLIFF, JSON, YAML, PO, Markdown, and more. Crash-isolated gRPC plugins in any language. Java bridge for 40+ Okapi filters.',
  },
  {
    title: 'Progressive Complexity',
    description:
      'Day one: CLI on files. Grow into flows, content store, automation, and team collaboration. Same content model at every scale — single binary, no runtime dependencies.',
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

export default function Home() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="Open, AI-native localization platform in Go">
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
