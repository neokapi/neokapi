import clsx from "clsx";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import Heading from "@theme/Heading";
import { Play, Sparkles, Terminal } from "lucide-react";
// Import from the SSR-clean ./store subpath, NOT the index barrel: the barrel
// re-exports KapiModal/KapiEmbed, which pull in xterm + the wasm boot path.
// openKapi is just the lightweight event bus, so the hero CTA fetches zero wasm
// until the shared modal opens.
import { openKapi } from "@neokapi/kapi-playground/store";

import styles from "./index.module.css";

// The inviting first command for the hero CTA. Pseudo-translation is the most
// instantly legible "wow" — readable accented output, no API key, no input file
// to find (the fixture is seeded). autoRun so the result appears the moment the
// modal opens.
const HERO_CMD = "kapi pseudo-translate messages.json";

function tryItLive() {
  openKapi({ cmd: HERO_CMD, seed: ["messages.json"], autoRun: true });
}

// A faux terminal preview that doubles as the primary CTA. The whole card is a
// button: clicking anywhere (or the green Run pill) opens the real in-browser
// kapi playground and runs HERO_CMD. SSR-clean — openKapi is the lightweight
// event bus; no wasm is fetched until the shared modal opens.
function HeroTerminal() {
  return (
    <button
      type="button"
      className={styles.heroTerminal}
      onClick={tryItLive}
      aria-label="Run kapi pseudo-translate in an in-browser terminal"
    >
      <span className={styles.heroTerminalBar} aria-hidden="true">
        <span className={styles.heroDot} data-tone="red" />
        <span className={styles.heroDot} data-tone="amber" />
        <span className={styles.heroDot} data-tone="green" />
        <span className={styles.heroTerminalLabel}>
          <Terminal size={13} aria-hidden="true" /> in-browser · no server
        </span>
      </span>
      <span className={styles.heroTerminalBody} aria-hidden="true">
        <span className={styles.heroLine}>
          <span className={styles.heroPrompt}>$</span> {HERO_CMD}
        </span>
        <span className={clsx(styles.heroLine, styles.heroOutput)}>
          messages.json &rarr; <span className={styles.heroOk}>fr</span>
        </span>
        <span className={clsx(styles.heroLine, styles.heroOutput)}>
          greeting: <span className={styles.heroPseudo}>[!! Ĥëëllöö, Ŵöörld! !!]</span>
        </span>
        <span className={clsx(styles.heroLine, styles.heroCursorLine)}>
          <span className={styles.heroPrompt}>$</span>
          <span className={styles.heroCursor} />
        </span>
      </span>
      <span className={styles.heroRunPill}>
        <Play size={15} aria-hidden="true" fill="currentColor" />
        Run it now
      </span>
    </button>
  );
}

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <header className={clsx("hero", styles.heroBanner)}>
      <div className={clsx("container", styles.heroGrid)}>
        <div className={styles.heroIntro}>
          <img src={useBaseUrl("/img/hero-logo.png")} alt="neokapi" className={styles.heroLogo} />
          <Heading as="h1" className={clsx("hero__title", styles.heroTitle)}>
            {siteConfig.title} &mdash; {siteConfig.tagline}
          </Heading>
          <p className={styles.heroSubtitle}>
            An open-source engine in Go that parses localization, document, and data formats into a
            faithful content model &mdash; then translates it, leverages memory, and runs checks for
            terminology, QA, and brand voice, whether a person or an agent wrote it.
          </p>
          <div className={styles.buttons}>
            <Link
              className={clsx("button button--lg", styles.tryButton)}
              to="/kapi/get-started/use-with-claude"
            >
              <Sparkles size={18} aria-hidden="true" />
              Try Kapi with Claude
            </Link>
            <Link
              className="button button--secondary button--lg"
              to="/kapi/overview"
            >
              Get Started
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

const NeokapiFeatures: ProductItem[] = [
  {
    title: "Formats & plugins",
    description:
      "Readers and writers for localization, document, data, subtitle, and office formats, extended by plugins and a bridge to the Java Okapi filters.",
    link: "/framework/formats",
    linkText: "Formats",
  },
  {
    title: "AI-native tools",
    description:
      "LLM-assisted translation, QA, terminology, and review compose in the same pipeline as machine-translation backends and rule-based checks.",
    link: "/framework/ai-translation",
    linkText: "AI translation",
  },
  {
    title: "Streaming pipeline",
    description:
      "Tools run in parallel and stream results as each part is ready, so large files and many languages process fast.",
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

function HomepageFeatures() {
  return (
    <section className={styles.features}>
      <div className="container">
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
    <Layout title={siteConfig.title} description="Open, AI-native, format-aware content engine in Go">
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
