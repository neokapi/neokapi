import clsx from "clsx";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";

import styles from "./index.module.css";

// A static faux-terminal that illustrates the one-command sync a kapi project
// gets once its recipe declares a `server:` block. It is purely decorative —
// not interactive — so it carries no click handler.
function HeroTerminal() {
  return (
    <div className={styles.heroTerminal} aria-hidden="true">
      <div className={styles.heroTerminalBar}>
        <span className={styles.heroDot} data-tone="red" />
        <span className={styles.heroDot} data-tone="amber" />
        <span className={styles.heroDot} data-tone="green" />
        <span className={styles.heroTerminalLabel}>kapi sync</span>
      </div>
      <div className={styles.heroTerminalBody}>
        <span className={styles.heroLine}>
          <span className={styles.heroPrompt}>$</span> kapi sync
        </span>
        <span className={clsx(styles.heroLine, styles.heroOutput)}>
          <span className={styles.heroKey}>push</span> &nbsp;12 blocks changed &rarr; bowrain.cloud
        </span>
        <span className={clsx(styles.heroLine, styles.heroOutput)}>
          <span className={styles.heroKey}>wait</span> &nbsp;translate &middot; QA &middot; brand checks
        </span>
        <span className={clsx(styles.heroLine, styles.heroOutput)}>
          <span className={styles.heroKey}>pull</span> &nbsp;12 blocks translated
        </span>
        <span className={clsx(styles.heroLine, styles.heroOutput)}>
          <span className={styles.heroOk}>in sync</span> with the workspace
        </span>
        <span className={clsx(styles.heroLine, styles.heroCursorLine)}>
          <span className={styles.heroPrompt}>$</span>
          <span className={styles.heroCursor} />
        </span>
      </div>
    </div>
  );
}

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <header className={clsx("hero", styles.heroBanner)}>
      <div className={clsx("container", styles.heroGrid)}>
        <div className={styles.heroIntro}>
          <img src={useBaseUrl("/img/hero-logo.png")} alt="Bowrain" className={styles.heroLogo} />
          <Heading as="h1" className={clsx("hero__title", styles.heroTitle)}>
            {siteConfig.title}
          </Heading>
          <p className={styles.heroSubtitle}>
            The server-side platform companion to kapi: shared, versioned governance of brand voice,
            terminology, and translation memory; collaborative editing; connectors to the systems your
            content already lives in; and automation &mdash; the persistent, multi-user layer a team
            needs. It is to kapi what GitHub is to git.
          </p>
          <div className={styles.buttons}>
            <Link className={clsx("button button--lg", styles.tryButton)} to="/quickstart">
              Get Started
            </Link>
            <Link className="button button--secondary button--lg" to="/introduction">
              Introduction
            </Link>
          </div>
        </div>
        <div className={styles.heroAside}>
          <HeroTerminal />
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

const BowrainFeatures: ProductItem[] = [
  {
    title: "Shared, versioned governance",
    description:
      "One brand-voice profile, terminology base, and translation memory, held on the server and shared across writers, translators, and AI tools — versioned and auditable, and learning from every correction.",
    link: "/server/brand-voice",
    linkText: "Brand voice",
  },
  {
    title: "Real-time collaboration",
    description:
      "A web editor and a native desktop app connect to the same server: Visual and Table views with translation memory and terminology, while edits and presence propagate live to every client.",
    link: "/server/collaboration",
    linkText: "Collaboration",
  },
  {
    title: "Connectors",
    description:
      "Sync against the systems content already lives in — a CMS, a design tool, a git host, or a developer's local files via kapi. Most pull source in server-side, with no local checkout.",
    link: "/server/connectors",
    linkText: "Connectors",
  },
  {
    title: "Project sync with kapi",
    description:
      "A kapi project whose recipe declares a server: block pushes and pulls in a single step. In CI, kapi sync replaces a multi-job pipeline — one invocation from code change to translated files.",
    link: "/cli/overview",
    linkText: "Connect (CLI)",
  },
];

function ProductCard({ title, description, link, linkText }: ProductItem) {
  return (
    <div className={clsx("col col--6")}>
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
        <div className="row margin-bottom--xl">
          {BowrainFeatures.map((props, idx) => (
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
      description="Bowrain — the team platform for governed brand voice, terminology, and translation, built on the kapi toolchain"
    >
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
