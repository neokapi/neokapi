import clsx from "clsx";
import Link from "@docusaurus/Link";
import Translate from "@docusaurus/Translate";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import styles from "./index.module.css";

type FeatureItem = {
  titleId: string;
  titleDefault: string;
  descriptionId: string;
  descriptionDefault: string;
};

const featureList: FeatureItem[] = [
  {
    titleId: "homepage.features.speed.title",
    titleDefault: "Lightning Fast",
    descriptionId: "homepage.features.speed.description",
    descriptionDefault:
      "Built for performance from the ground up. Sub-millisecond response times even under heavy load.",
  },
  {
    titleId: "homepage.features.security.title",
    titleDefault: "Enterprise Security",
    descriptionId: "homepage.features.security.description",
    descriptionDefault:
      "SOC 2 Type II certified. End-to-end encryption for all data in transit and at rest.",
  },
  {
    titleId: "homepage.features.integration.title",
    titleDefault: "Easy Integration",
    descriptionId: "homepage.features.integration.description",
    descriptionDefault:
      "Connect with your existing tools in minutes. SDKs available for all major languages.",
  },
  {
    titleId: "homepage.features.scalability.title",
    titleDefault: "Infinite Scalability",
    descriptionId: "homepage.features.scalability.description",
    descriptionDefault:
      "Auto-scales from zero to millions of requests. Pay only for what you use.",
  },
  {
    titleId: "homepage.features.monitoring.title",
    titleDefault: "Real-Time Monitoring",
    descriptionId: "homepage.features.monitoring.description",
    descriptionDefault:
      "Track every metric that matters. Custom dashboards and intelligent alerting built in.",
  },
  {
    titleId: "homepage.features.support.title",
    titleDefault: "24/7 Support",
    descriptionId: "homepage.features.support.description",
    descriptionDefault:
      "Our expert team is available around the clock. Average response time under 5 minutes.",
  },
];

function Feature({ titleId, titleDefault, descriptionId, descriptionDefault }: FeatureItem) {
  return (
    <div className={clsx("col col--4")}>
      <div className="padding-horiz--md padding-vert--lg">
        <h3>
          <Translate id={titleId}>{titleDefault}</Translate>
        </h3>
        <p>
          <Translate id={descriptionId}>{descriptionDefault}</Translate>
        </p>
      </div>
    </div>
  );
}

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <header className={clsx("hero hero--primary", styles.heroBanner)}>
      <div className="container">
        <h1 className="hero__title">
          <Translate id="homepage.hero.title">Build Amazing Things with Acme</Translate>
        </h1>
        <p className="hero__subtitle">
          <Translate id="homepage.hero.subtitle">
            Acme provides the tools and infrastructure you need to ship faster and more reliably.
          </Translate>
        </p>
        <div className={styles.buttons}>
          <Link className="button button--secondary button--lg" to="/docs/intro">
            <Translate id="homepage.cta.getStarted">Get Started</Translate>
          </Link>
          <Link
            className="button button--secondary button--outline button--lg margin-left--md"
            to="/docs/features"
          >
            <Translate id="homepage.cta.learnMore">Learn More</Translate>
          </Link>
        </div>
      </div>
    </header>
  );
}

function Testimonial() {
  return (
    <section className={styles.testimonial}>
      <div className="container">
        <blockquote>
          <p>
            <Translate id="homepage.testimonial.quote">
              Acme reduced our deployment time from hours to minutes. It&apos;s transformed how we
              ship software.
            </Translate>
          </p>
          <footer>
            &mdash;{" "}
            <Translate id="homepage.testimonial.author">Jane Smith, CTO at TechCorp</Translate>
          </footer>
        </blockquote>
      </div>
    </section>
  );
}

export default function Home(): JSX.Element {
  const { siteConfig } = useDocusaurusContext();
  return (
    <Layout title={siteConfig.title} description={siteConfig.tagline}>
      <HomepageHeader />
      <main>
        <section className={styles.features}>
          <div className="container">
            <div className="row">
              {featureList.map((props, idx) => (
                <Feature key={idx} {...props} />
              ))}
            </div>
          </div>
        </section>
        <Testimonial />
      </main>
    </Layout>
  );
}
