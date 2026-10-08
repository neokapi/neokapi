import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import BrowserOnly from "@docusaurus/BrowserOnly";
import Translate, { translate } from "@docusaurus/Translate";
import { Check, Play } from "lucide-react";
import {
  LABS,
  SERIES,
  SAMPLES,
  labsInSeries,
  labById,
  type Lab,
} from "@neokapi/kapi-learn/curriculum";
import { allProgress, lastOpened } from "@neokapi/kapi-learn/progress";
import "../../components/Learn/learn.css";

// Learn kapi: the index of the learning labs. Three series, one sample
// project each, read in order; every card is a lab page at /learn/<id>.
// Static: nothing here touches the engine. The "continue" card and the
// progress bars read this browser's progress on the client.

const EXPLORERS: { to: string; name: string; teaches: string }[] = [
  { to: "/lab", name: "Flow workspace", teaches: "build a flow and watch its trace" },
  {
    to: "/lab/segmentation",
    name: "Segmentation",
    teaches: "compare sentence splitters on your text",
  },
  {
    to: "/lab/convert",
    name: "File conversion",
    teaches: "re-express a format and see what survives",
  },
  {
    to: "/lab/structure",
    name: "Structure and layout",
    teaches: "reading order and geometry from a PDF",
  },
  { to: "/lab/vision", name: "Vision", teaches: "OCR and layout on an image" },
  { to: "/lab/media", name: "Audio and video", teaches: "transcripts on the way to subtitles" },
  { to: "/playground-cli", name: "CLI playground", teaches: "a free terminal with sample files" },
  { to: "/kbf-lab", name: "KBF anatomy", teaches: "the bundle format, part by part" },
];

function LabCard({ lab, done }: { lab: Lab; done: number }): React.ReactElement {
  const commands = lab.chapters.filter((c) => c.command).length;
  const pct = Math.round((done / lab.chapters.length) * 100);
  return (
    <li>
      <Link className={`kl-card kl-card--${lab.sample}`} to={`/learn/${lab.id}`}>
        <div className="kl-card__poster" aria-hidden="true">
          <span className="kl-card__num">{String(lab.position).padStart(2, "0")}</span>
          <span className="kl-card__meta">
            {lab.chapters.length} chapters
            <br />
            {commands} commands · {lab.minutes} min
          </span>
          <span className="kl-card__play">
            <Play size={18} fill="currentColor" />
          </span>
        </div>
        {pct > 0 && (
          <div className="kl-card__progress" aria-label={`${pct}% done`}>
            <span style={{ width: `${pct}%` }} />
          </div>
        )}
        <div className="kl-card__body">
          <h3 className="kl-card__title">{lab.title}</h3>
          <p className="kl-card__tagline">{lab.tagline}</p>
          <ul className="kl-card__concepts" aria-label="Concepts">
            {lab.concepts.slice(0, 4).map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
        </div>
      </Link>
    </li>
  );
}

function ContinueCard(): React.ReactElement | null {
  const last = lastOpened();
  if (!last) return null;
  const lab = labById(last.labId);
  if (!lab) return null;
  const finished = last.progress.done.length >= lab.chapters.length;
  const at = lab.chapters.find((c) => c.id === last.progress.at);
  const href = finished || !at ? `/learn/${lab.id}` : `/learn/${lab.id}?c=${at.id}`;
  return (
    <Link className="kl-continue" to={href}>
      <span className="kl-continue__eyebrow">
        {finished ? "Last finished" : "Continue where you left off"}
      </span>
      <span className="kl-continue__title">{lab.title}</span>
      <span className="kl-continue__sub">
        {finished
          ? "All chapters run. Open it again, or go on to the next lab."
          : at
            ? `Chapter ${lab.chapters.indexOf(at) + 1} of ${lab.chapters.length}: ${at.title}`
            : lab.tagline}
      </span>
    </Link>
  );
}

function SeriesSection({
  index,
  seriesId,
}: {
  index: number;
  seriesId: (typeof SERIES)[number]["id"];
}) {
  const series = SERIES.find((s) => s.id === seriesId)!;
  const labs = labsInSeries(seriesId);
  const sample = SAMPLES[series.sample];
  return (
    <section className="kl-series" aria-labelledby={`series-${series.id}`}>
      <div className="kl-series__head">
        <span className="kl-series__num">Series {index}</span>
        <h2 className="kl-series__title" id={`series-${series.id}`}>
          {series.title}
        </h2>
      </div>
      <p className="kl-series__desc">
        {series.description} <em>{sample.name}:</em> {sample.blurb}
      </p>
      <BrowserOnly fallback={<Cards labs={labs} progress={{}} />}>
        {() => <Cards labs={labs} progress={allProgress()} />}
      </BrowserOnly>
    </section>
  );
}

function Cards({
  labs,
  progress,
}: {
  labs: Lab[];
  progress: ReturnType<typeof allProgress>;
}): React.ReactElement {
  return (
    <ul className="kl-cards">
      {labs.map((lab) => (
        <LabCard key={lab.id} lab={lab} done={progress[lab.id]?.done.length ?? 0} />
      ))}
    </ul>
  );
}

export default function LearnIndexPage(): React.ReactElement {
  const totalMinutes = LABS.reduce((n, l) => n + l.minutes, 0);
  return (
    <Layout
      title={translate({ id: "learn.page.title", message: "Learn kapi" })}
      description={translate({
        id: "learn.page.description",
        message:
          "Learn kapi by running it: eleven short labs in three sample projects, each a scripted session in the real kapi engine running in your browser. Press play, watch the commands run, then type your own.",
      })}
    >
      <main className="kapi-reference kl-index">
        <div className="kl-index__hero">
          <div>
            <p className="kl-index__eyebrow">
              <Translate id="learn.hero.eyebrow">Learning labs</Translate>
            </p>
            <h1 className="kl-index__title">
              <Translate id="learn.hero.title">Learn kapi by running it</Translate>
            </h1>
            <p className="kl-index__lead">
              <Translate id="learn.hero.lead">
                Every lab is a short session in a sample company's repository, run by the real kapi
                engine in your browser. Press play and the chapters type the commands for you, one
                at a time, with a line on what each one shows. Pause and type your own at any point;
                share a link to the chapter you are on.
              </Translate>
            </p>
            <ul className="kl-index__how">
              <li>
                <Check size={16} aria-hidden="true" />
                <span>
                  <Translate id="learn.hero.how.install">
                    Nothing to install and nothing leaves your machine: the engine downloads once,
                    about 20 MB, when you press play.
                  </Translate>
                </span>
              </li>
              <li>
                <Check size={16} aria-hidden="true" />
                <span>
                  <Translate id="learn.hero.how.order">
                    Read the series in order. Each lab is a few minutes; all of them together are
                    about an hour.
                  </Translate>{" "}
                  ({totalMinutes} min)
                </span>
              </li>
              <li>
                <Check size={16} aria-hidden="true" />
                <span>
                  <Translate id="learn.hero.how.samples">
                    The projects are the samples the documentation and the recordings use, so what
                    you see here is what you get on your own machine.
                  </Translate>
                </span>
              </li>
            </ul>
          </div>
          <BrowserOnly>{() => <ContinueCard />}</BrowserOnly>
        </div>

        {SERIES.map((s, i) => (
          <SeriesSection key={s.id} index={i + 1} seriesId={s.id} />
        ))}

        <section className="kl-explorers" aria-labelledby="explorers">
          <h2 id="explorers">
            <Translate id="learn.explorers.title">Engine explorers</Translate>
          </h2>
          <p>
            <Translate id="learn.explorers.lead">
              Beyond the labs, a set of explorers opens one part of the engine at a time on a file
              you bring: segmentation, conversion, document structure, vision, audio and video, the
              bundle format, and a free terminal.
            </Translate>
          </p>
          <ul>
            {EXPLORERS.map((e) => (
              <li key={e.to}>
                <Link to={e.to}>{e.name}</Link> <span>· {e.teaches}</span>
              </li>
            ))}
          </ul>
        </section>
      </main>
    </Layout>
  );
}
