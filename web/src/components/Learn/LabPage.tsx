import React, { useCallback, useMemo } from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useHistory, useLocation } from "@docusaurus/router";
import {
  isExplorerLab,
  labById,
  labsInSeries,
  nextLab,
  SAMPLES,
  seriesById,
  type Lab,
} from "@neokapi/kapi-learn/curriculum";
import { CHAPTER_PARAM, parseLabLink } from "@neokapi/kapi-learn/deeplink";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";
import { ChunkSafeSuspense } from "../ChunkErrorBoundary";
import { lazyWithRetry } from "../../lib/chunkReload";
import { renderStage } from "./stages";
import "./learn.css";

// The page for one learning lab, at /learn/<id> (routes are added by
// web/plugins/learn-routes.ts). The chrome and the chapter list render on the
// server, so the page reads without JavaScript and a shared link previews;
// the player, with xterm and the engine boot path, is one lazy chunk that
// loads in the browser and fetches nothing until Play. A terminal lab gets
// the lab player; an explorer lab gets the explorer player, whose stage is
// one of the engine explorers (see stages.tsx).

const LazyPlayer = lazyWithRetry(async () => {
  const mod = await import("@neokapi/kapi-learn/player");
  return { default: mod.LabPlayer };
});

const LazyExplorerPlayer = lazyWithRetry(async () => {
  const mod = await import("@neokapi/kapi-learn/player");
  return { default: mod.ExplorerPlayer };
});

export interface LabPageProps {
  labId: string;
}

/** The static reading of a lab: what a crawler, a link preview or a reader without scripts gets. */
function StaticLab({ lab }: { lab: Lab }): React.ReactElement {
  const series = seriesById(lab.series);
  return (
    <div className="kl-static">
      <p className="kl-static__eyebrow">
        <Link to="/learn">Learn kapi</Link> › {series.title}
      </p>
      <h1>{lab.title}</h1>
      <p className="kl-static__tagline">{lab.tagline}</p>
      <p>{lab.summary}</p>
      <ol className="kl-static__chapters">
        {lab.chapters.map((ch) => (
          <li key={ch.id}>
            <strong>{ch.title}</strong>
            {ch.command && <code>{ch.command}</code>}
            <span>{ch.narration}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}

function PlayerHost({ lab }: { lab: Lab }): React.ReactElement {
  const assets = useKapiPlaygroundConfig();
  const location = useLocation();
  const history = useHistory();
  const series = seriesById(lab.series);
  const sample = SAMPLES[lab.sample];
  // Read once: the link the page opened with. Later chapter changes rewrite
  // the query without re-reading it, so the player is never re-seeded.
  const link = useMemo(() => parseLabLink(location.search), []); // eslint-disable-line react-hooks/exhaustive-deps
  const siblings = labsInSeries(lab.series);
  const next = nextLab(lab.id);

  const onChapterChange = useCallback(
    (chapterId: string) => {
      const params = new URLSearchParams(window.location.search);
      params.set(CHAPTER_PARAM, chapterId);
      // A shared session is consumed on load; the URL then names the chapter alone.
      params.delete("s");
      history.replace({ pathname: window.location.pathname, search: `?${params.toString()}` });
    },
    [history],
  );

  const shared = {
    lab,
    series,
    sample,
    assets,
    link,
    onChapterChange,
    indexHref: "/learn",
    position: { index: lab.position, of: siblings.length },
    upNext: next
      ? { id: next.id, title: next.title, tagline: next.tagline, href: `/learn/${next.id}` }
      : undefined,
  };

  return isExplorerLab(lab) ? (
    <LazyExplorerPlayer {...shared} renderStage={renderStage} />
  ) : (
    <LazyPlayer {...shared} />
  );
}

export default function LabPage({ labId }: LabPageProps): React.ReactElement {
  const lab = labById(labId);
  if (!lab) {
    return (
      <Layout title="Lab not found">
        <main className="container margin-vert--lg">
          <h1>Lab not found</h1>
          <p>
            No lab is called <code>{labId}</code>. <Link to="/learn">See every lab.</Link>
          </p>
        </main>
      </Layout>
    );
  }
  const series = seriesById(lab.series);
  return (
    <Layout title={`${lab.title} · ${series.title}`} description={`${lab.tagline} ${lab.summary}`}>
      <main className="kapi-reference kl-page">
        <BrowserOnly fallback={<StaticLab lab={lab} />}>
          {() => (
            <ChunkSafeSuspense fallback={<StaticLab lab={lab} />}>
              <PlayerHost lab={lab} />
            </ChunkSafeSuspense>
          )}
        </BrowserOnly>
      </main>
    </Layout>
  );
}
