import clsx from "clsx";
import Link from "@docusaurus/Link";
import Translate, { translate } from "@docusaurus/Translate";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";

import styles from "./index.module.css";

// Every reader-facing string here goes through Docusaurus's <Translate> /
// translate(). That is not decoration: `write-translations` finds strings by
// STATIC ANALYSIS of these two APIs, so an unwrapped literal never reaches
// code.json and no translation pipeline can see it. This page rendered entirely
// in English in the qps pseudo-locale build until it was wrapped.
//
// JSX text takes <Translate>; strings that live in a data array or a prop take
// translate(), because the JSX walker never sees those.

// A static faux-UI panel illustrating bowrain's correction-learning loop: the
// repeated corrections a team makes to AI output surface as candidate rules, and
// promoting one hardens it into a versioned check enforced on every future
// generation. Purely decorative (aria-hidden) — the real flow lives in
// Voice & corrections.
function HeroPromote() {
  // `from`/`to` are deliberately NOT translated: they are the example terms the
  // rule is about — a vocabulary rule demonstrated, not prose.
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
        <span className={styles.heroCardLabel}>
          <Translate id="home.hero.card.label">brand checks</Translate>
        </span>
      </div>
      <div className={styles.heroCardBody}>
        <div className={styles.heroCardTitle}>
          <Translate id="home.hero.card.title">Suggested rules</Translate>
        </div>
        <p className={styles.heroCardHint}>
          <Translate id="home.hero.card.hint">
            Repeated corrections become candidate rules.
          </Translate>
        </p>
        <ul className={styles.ruleList}>
          {rules.map((r) => (
            <li className={styles.ruleRow} key={r.from}>
              <span className={styles.ruleTerms}>
                <span className={styles.ruleFrom}>{r.from}</span>
                <span className={styles.ruleArrow}>&rarr;</span>
                <span className={styles.ruleTo}>{r.to}</span>
              </span>
              <span className={styles.ruleCount}>
                <Translate id="home.hero.card.corrections" values={{ count: r.count }}>
                  {"{count} corrections"}
                </Translate>
              </span>
              <span className={styles.rulePromote}>
                <Translate id="home.hero.card.promote">Promote</Translate>
              </span>
            </li>
          ))}
        </ul>
        <p className={styles.heroCardFoot}>
          <span className={styles.heroOk}>
            <Translate id="home.hero.card.promoted">Promoted</Translate>
          </span>{" "}
          <Translate id="home.hero.card.promotedTail">
            → a versioned check, enforced on every future generation.
          </Translate>
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
          <img
            src={useBaseUrl("/img/hero-logo.png")}
            alt={translate({ id: "home.hero.logoAlt", message: "Bowrain" })}
            className={styles.heroLogo}
          />
          <Heading as="h1" className={clsx("hero__title", styles.heroTitle)}>
            {siteConfig.title}
          </Heading>
          {/* Split at the <strong>: <Translate> takes a string, not JSX
              children, so a sentence with inline markup becomes two units. */}
          <p className={styles.heroSubtitle}>
            <strong>
              <Translate id="home.hero.lede">
                The context graph your people and agents plug into
              </Translate>
            </strong>{" "}
            <Translate id="home.hero.subtitle">
              — record and steer the coordinates for content, so what you ship is on‑brand and
              on‑profile for the audience it was written for. Voice, vocabulary, approved wording,
              and corrections, versioned and learning from every review. Connected to the systems
              your content already lives in, with collaborative editing, review, and automation
              around them.
            </Translate>
          </p>
          <div className={styles.buttons}>
            <Link className={clsx("button button--lg", styles.tryButton)} to="/quickstart">
              <Translate id="home.cta.getStarted">Get Started</Translate>
            </Link>
            <Link className="button button--secondary button--lg" to="/introduction">
              <Translate id="home.cta.introduction">Introduction</Translate>
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

// A data array, so the strings need translate() rather than <Translate>: the
// JSX walker never sees a string literal sitting in a const. This is the case
// @neokapi/i18n-react-lint flags in the app codebases, and it applies here for
// the same reason.
const BowrainFeatures: ProductItem[] = [
  {
    title: translate({ id: "home.feature.graph.title", message: "One graph, every project" }),
    description: translate({
      id: "home.feature.graph.body",
      message:
        "Profiles, vocabulary, and content memory held on the server and drawn on by every project, person, and agent — versioned and auditable, and learning from every correction. kapi holds the same graph for one project; the difference is reach, not capability.",
    }),
    link: "/getting-started/the-context-graph",
    linkText: translate({ id: "home.feature.graph.link", message: "The context graph" }),
  },
  {
    title: translate({ id: "home.feature.collab.title", message: "Real-time collaboration" }),
    description: translate({
      id: "home.feature.collab.body",
      message:
        "A web editor and a native desktop app connect to the same server: Visual and Table views with content memory and terminology, while edits and presence propagate live to every client.",
    }),
    link: "/server/collaboration",
    linkText: translate({ id: "home.feature.collab.link", message: "Collaboration" }),
  },
  {
    title: translate({ id: "home.feature.connectors.title", message: "Connectors" }),
    description: translate({
      id: "home.feature.connectors.body",
      message:
        "Content platforms, design tools, code repositories, and a developer's checkout are peer routes into one workspace. Most run server-side, with nothing installed and nothing checked out.",
    }),
    link: "/server/connectors",
    linkText: translate({ id: "home.feature.connectors.link", message: "Connectors" }),
  },
  {
    title: translate({ id: "home.feature.current.title", message: "Content that stays current" }),
    description: translate({
      id: "home.feature.current.body",
      message:
        "A connector sync, a push, or a developer's command starts a server run: reuse what memory holds, draft the rest, check everything, and park what needs a person into the review queue.",
    }),
    link: "/the-loop",
    linkText: translate({ id: "home.feature.current.link", message: "Keeping content caught up" }),
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
      description={translate({
        id: "home.meta.description",
        message:
          "Bowrain — the context graph your people and agents plug into: record and steer the coordinates for content, so what you ship is compliant and on-profile for the audience it was written for",
      })}
    >
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
