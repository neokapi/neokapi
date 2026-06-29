import clsx from "clsx";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";
import { ThemedVideo } from "@neokapi/docs-shared";

import styles from "./index.module.css";

// A static faux-UI panel illustrating bowrain's correction-learning loop: the
// repeated corrections a team makes to AI output surface as candidate rules, and
// promoting one hardens it into a versioned brand check enforced on every future
// generation. Purely decorative (aria-hidden) — the real flow lives in
// Brand voice & corrections.
function HeroPromote() {
  const rules = [
    { from: "leverage", to: "use", count: 4 },
    { from: "best-in-class", to: "proven", count: 3 },
  ];
  return (
    <div className={styles.heroCard} aria-hidden="true">
      <div className={styles.heroCardBar}>
        <span className={styles.heroDot} data-tone="red" />
        <span className={styles.heroDot} data-tone="amber" />
        <span className={styles.heroDot} data-tone="green" />
        <span className={styles.heroCardLabel}>brand checks</span>
      </div>
      <div className={styles.heroCardBody}>
        <div className={styles.heroCardTitle}>Suggested rules</div>
        <p className={styles.heroCardHint}>Repeated corrections become candidate rules.</p>
        <ul className={styles.ruleList}>
          {rules.map((r) => (
            <li className={styles.ruleRow} key={r.from}>
              <span className={styles.ruleTerms}>
                <span className={styles.ruleFrom}>{r.from}</span>
                <span className={styles.ruleArrow}>&rarr;</span>
                <span className={styles.ruleTo}>{r.to}</span>
              </span>
              <span className={styles.ruleCount}>
                {r.count} corrections
              </span>
              <span className={styles.rulePromote}>Promote</span>
            </li>
          ))}
        </ul>
        <p className={styles.heroCardFoot}>
          <span className={styles.heroOk}>Promoted</span> &rarr; a versioned check, enforced on every
          future generation.
        </p>
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
            The team platform for keeping content <strong>on brand and in every language</strong>.
            Bowrain is to <strong>kapi</strong> what GitHub is to git &mdash; a shared, versioned
            home for brand voice, terminology, and translation memory, with collaborative review,
            connectors, and automation, that learns from every correction.
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
          <HeroPromote />
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

function SeeItWork() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="text--center margin-bottom--lg">
          <Heading as="h2">See it work</Heading>
          <p className={styles.sectionSubtitle}>
            How a change ships: a team reviews, approves, and stays on brand &mdash; on one shared
            workspace.
          </p>
        </div>
        <div className="row margin-bottom--md">
          <div className="col col--8 col--offset-2">
            <ThemedVideo
              sources={{
                light: "/video/bowrain-web/bowrain-web-collaboration-light.webm",
                dark: "/video/bowrain-web/bowrain-web-collaboration-dark.webm",
              }}
              maxWidth="100%"
            />
          </div>
        </div>
        <div className="text--center">
          <Link to="/introduction">How Bowrain fits with kapi &rarr;</Link>
        </div>
      </div>
    </section>
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
        <SeeItWork />
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
