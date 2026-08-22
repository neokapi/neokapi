import clsx from "clsx";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";

import styles from "./index.module.css";

// A static faux-UI panel illustrating bowrain's correction-learning loop: the
// repeated corrections a team makes to AI output surface as candidate rules, and
// promoting one hardens it into a versioned brand check enforced on every future
// generation. Purely decorative (aria-hidden) — the real flow lives in
// Voice & corrections.
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
            <strong>The context graph your people and agents plug into</strong> &mdash; record and
            steer the coordinates for content, so what you ship is on&#8209;brand and on&#8209;profile
            for the audience it was written for. Voice, vocabulary, approved wording, and
            corrections, versioned and learning from every review. Connected to the systems your
            content already lives in, with collaborative editing, review, and automation around them.
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
    title: "One graph, every project",
    description:
      "Profiles, vocabulary, and content memory held on the server and drawn on by every project, person, and agent — versioned and auditable, and learning from every correction. kapi holds the same graph for one project; the difference is reach, not capability.",
    link: "/getting-started/the-context-graph",
    linkText: "The context graph",
  },
  {
    title: "Real-time collaboration",
    description:
      "A web editor and a native desktop app connect to the same server: Visual and Table views with content memory and terminology, while edits and presence propagate live to every client.",
    link: "/server/collaboration",
    linkText: "Collaboration",
  },
  {
    title: "Connectors",
    description:
      "Content platforms, design tools, code repositories, and a developer's checkout are peer routes into one workspace. Most run server-side, with nothing installed and nothing checked out.",
    link: "/server/connectors",
    linkText: "Connectors",
  },
  {
    title: "Content that stays current",
    description:
      "A connector sync, a push, or a developer's command starts a server run: reuse what memory holds, draft the rest, check everything, and park what needs a person into the review queue.",
    link: "/the-loop",
    linkText: "Keeping content caught up",
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
      description="Bowrain — the context graph your people and agents plug into: record and steer the coordinates for content, so what you ship is compliant and on-profile for the audience it was written for"
    >
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
