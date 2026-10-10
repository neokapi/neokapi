import React from "react";
import type { Lab, SampleInfo } from "../curriculum/types.ts";
import type { UpNext } from "./types.ts";

// The reading under the player: what the lab is about, the sample it runs in,
// the concepts, where to read on, and the lab that follows. Quiet by design;
// the stage and the chapter card are the page.

export interface AboutLabProps {
  lab: Lab;
  sample: SampleInfo;
  upNext?: UpNext;
  /** The keys the player answers to, in a short phrase. */
  keys?: string;
}

export default function AboutLab({ lab, sample, upNext, keys }: AboutLabProps): React.ReactElement {
  return (
    <section className="kl-about" aria-label="About this lab">
      <div className="kl-about__main">
        <h2 className="kl-about__title">About this lab</h2>
        <p>{lab.summary}</p>
        <p className="kl-about__sample">
          It runs in the {sample.name} sample. {sample.blurb}
        </p>
        {keys && <p className="kl-about__keys">{keys}</p>}
      </div>
      <div className="kl-about__side">
        <h3 className="kl-about__sub">Concepts</h3>
        <ul className="kl-concepts">
          {lab.concepts.map((c) => (
            <li key={c}>{c}</li>
          ))}
        </ul>
        <h3 className="kl-about__sub">Read on</h3>
        <ul className="kl-docs">
          {lab.docs.map((d) => (
            <li key={d.href}>
              <a href={d.href}>{d.label}</a>
            </li>
          ))}
        </ul>
        {upNext && (
          <>
            <h3 className="kl-about__sub">Next lab</h3>
            <a className="kl-about__next" href={upNext.href}>
              <span className="kl-about__next-title">{upNext.title}</span>
              <span className="kl-about__next-tagline">{upNext.tagline}</span>
            </a>
          </>
        )}
      </div>
    </section>
  );
}
