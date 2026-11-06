import clsx from "clsx";
import Link from "@docusaurus/Link";
import Translate, { translate } from "@docusaurus/Translate";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";
import MapPreview from "../components/home/MapPreview";
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
              Consistent writing across people, AI agents and languages.
            </Translate>
          </Heading>
          <p className={styles.heroSubtitle}>
            <Translate id="home.hero.subtitle">
              kapi reads your project's files, keeps the rules they are written by in a context
              graph, and checks every edit against it. Use it from the desktop app, the command line
              or your AI agent. It is built on neokapi, the open content engine.
            </Translate>
          </p>
          <div className={styles.buttons}>
            <Link className="button button--lg button--primary" to="/kapi/get-started/installation">
              <Translate id="home.cta.getKapi">Get kapi</Translate>
            </Link>
            <Link className="button button--secondary button--lg" to="/framework/architecture">
              <Translate id="home.cta.engine">The engine, in Go</Translate>
            </Link>
          </div>
          <p className={styles.heroHint}>
            <Link to="/learn">
              <Translate id="home.cta.learn">Try it in the browser</Translate>
            </Link>
          </p>
        </div>
        <div className={styles.heroAside}>
          <MapPreview />
        </div>
      </div>
    </header>
  );
}

type Card = {
  title: string;
  description: string;
  link: string;
  linkText: string;
};

// translate() rather than <Translate>: a literal in a const never reaches the
// JSX walker, so write-translations would not see it.
const KapiCards: Card[] = [
  {
    title: translate({ id: "home.kapi.context.title", message: "The context graph" }),
    description: translate({
      id: "home.kapi.context.description",
      message:
        "The rules your project writes by: its terms, its voice, the decisions people kept, and where each one applies. kapi builds it from your files and keeps it beside them.",
    }),
    link: "/kapi/context",
    linkText: translate({ id: "home.kapi.context.link", message: "Context" }),
  },
  {
    title: translate({ id: "home.kapi.desktop.title", message: "Kapi Desktop" }),
    description: translate({
      id: "home.kapi.desktop.description",
      message:
        "Open a folder. The Map shows your files, the rules in force and what an assistant noticed. Decide what becomes a rule, then open a file and fix a finding in place.",
    }),
    link: "/kapi/desktop/overview",
    linkText: translate({ id: "home.kapi.desktop.link", message: "Kapi Desktop" }),
  },
  {
    title: translate({ id: "home.kapi.agents.title", message: "Command line and agents" }),
    description: translate({
      id: "home.kapi.agents.description",
      message:
        "The same context in a terminal, in CI and in your AI agent: kapi writes the rules into AGENTS.md and CLAUDE.md, serves them over MCP, and fails kapi check on a failing finding.",
    }),
    link: "/kapi/get-started/use-with-claude",
    linkText: translate({ id: "home.kapi.agents.link", message: "Use with your agent" }),
  },
  {
    title: translate({ id: "home.kapi.languages.title", message: "Languages when you need them" }),
    description: translate({
      id: "home.kapi.languages.description",
      message:
        "Add a target language to the project. kapi up translates what changed, reuses what was approved before, and holds back what needs a person.",
    }),
    link: "/kapi/get-started/add-languages",
    linkText: translate({ id: "home.kapi.languages.link", message: "Add languages" }),
  },
];

const EngineCards: Card[] = [
  {
    title: translate({ id: "home.engine.formats.title", message: "Formats" }),
    description: translate({
      id: "home.engine.formats.description",
      message:
        "Readers and writers for documents, data and office files into one content model. An edit goes back into the file with its structure intact.",
    }),
    link: "/framework/formats",
    linkText: translate({ id: "home.engine.formats.link", message: "Formats" }),
  },
  {
    title: translate({ id: "home.engine.tools.title", message: "Tools and flows" }),
    description: translate({
      id: "home.engine.tools.description",
      message:
        "Composable tools for checking, editing and translating content, run through streaming flows. Headless, from a terminal, an agent or your own Go program.",
    }),
    link: "/framework/go-quickstart",
    linkText: translate({ id: "home.engine.tools.link", message: "Go quickstart" }),
  },
  {
    title: translate({ id: "home.engine.measured.title", message: "Measured" }),
    description: translate({
      id: "home.engine.measured.description",
      message:
        "Format support is measured. The format-maturity and benchmark dashboards show how each format holds up.",
    }),
    link: "/format-maturity",
    linkText: translate({ id: "home.engine.measured.link", message: "Dashboards" }),
  },
];

function CardCol({ title, description, link, linkText, cols }: Card & { cols: 3 | 4 }) {
  return (
    <div className={`col col--${cols}`}>
      <div className={styles.card}>
        <Heading as="h3" className={styles.cardTitle}>
          {title}
        </Heading>
        <p className={styles.cardText}>{description}</p>
        <Link to={link}>{linkText} &rarr;</Link>
      </div>
    </div>
  );
}

function KapiSection() {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className={styles.sectionHead}>
          <Heading as="h2">
            <Translate id="home.kapi.title">One context, every way you write</Translate>
          </Heading>
          <p className={styles.sectionSubtitle}>
            <Translate id="home.kapi.subtitle">
              A rule you keep on the desktop is in force on the command line and in your agent's
              session, and a check reads the same rule wherever it runs.
            </Translate>
          </p>
        </div>
        <div className="row">
          {KapiCards.map((card) => (
            <CardCol key={card.link} {...card} cols={3} />
          ))}
        </div>
      </div>
    </section>
  );
}

function EngineSection() {
  return (
    <section className={clsx(styles.section, styles.engine)}>
      <div className="container">
        <div className={styles.sectionHead}>
          <span className={styles.eyebrow}>
            <Translate id="home.engine.eyebrow">Open source, Apache-2.0</Translate>
          </span>
          <Heading as="h2">
            <Translate id="home.engine.title">neokapi, the engine underneath</Translate>
          </Heading>
          <p className={styles.sectionSubtitle}>
            <Translate id="home.engine.subtitle">
              A content engine in Go. kapi is built on it, and your own programs can be too.
            </Translate>
          </p>
        </div>
        <div className="row">
          {EngineCards.map((card) => (
            <CardCol key={card.link} {...card} cols={4} />
          ))}
        </div>
        <div className={styles.engineLinks}>
          <Link className="button button--outline button--primary" to="/framework/architecture">
            <Translate id="home.engine.architecture">Architecture</Translate>
          </Link>
          <Link className="button button--outline button--primary" to="/framework/go-quickstart">
            <Translate id="home.engine.quickstart">Use it from Go</Translate>
          </Link>
        </div>
      </div>
    </section>
  );
}

function FamilyRow() {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className={styles.familyRow}>
          <Link to="/react/introduction" className={styles.familyCallout}>
            <span className={styles.familyBadge}>
              <Translate id="home.family.badge">Also from neokapi</Translate>
            </span>
            <span className={styles.familyText}>
              <strong>neokapi-i18n</strong>{" "}
              <Translate id="home.family.i18n">
                , an i18n library for React with build-time string extraction.
              </Translate>
            </span>
            <span className={styles.familyArrow} aria-hidden="true">
              &rarr;
            </span>
          </Link>
          <Link to="/toolbox/overview" className={clsx(styles.familyCallout, styles.toolsCallout)}>
            <span className={styles.familyBadge}>
              <Translate id="home.family.badge2">Also from neokapi</Translate>
            </span>
            <span className={styles.familyText}>
              <code>kgrep</code>, <code>ksed</code>, <code>kcat</code>:{" "}
              <Translate id="home.family.tools">
                grep, sed and cat for the text inside office files, JSON and HTML.
              </Translate>
            </span>
            <span className={styles.familyArrow} aria-hidden="true">
              &rarr;
            </span>
          </Link>
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
          "kapi keeps a project's writing consistent across people, AI agents and languages: it reads your files, keeps the rules they are written by in a context graph, and checks every edit against it. Built on neokapi, the open content engine in Go.",
      })}
    >
      <StructuredData />
      <HomepageHeader />
      <main>
        <KapiSection />
        <EngineSection />
        <FamilyRow />
        <div className="container">
          <AuthorsNote />
        </div>
      </main>
    </Layout>
  );
}
