import type { ReactElement } from "react";
import Layout from "@theme/Layout";
import {
  bandByID,
  bandTally,
  BandNav,
  layerByID,
  LayerBlock,
  Pill,
  s,
  statusLabel,
  statusMeans,
  statusTone,
  type Eval,
  type Layer,
} from "./_shared";

// One band's page. The three routes under /evals/ are three calls to this with
// a different id, so the shape stays identical and only the content differs.

export function BandPage({ id }: { id: string }): ReactElement {
  const band = bandByID.get(id);
  if (!band) {
    return (
      <Layout title="Unknown band">
        <main className="container margin-vert--lg">
          <h1>Unknown band</h1>
          <p>No band with id {id}.</p>
        </main>
      </Layout>
    );
  }

  const counts = bandTally(band);
  const layers = band.layers.map((lid) => layerByID.get(lid)).filter((l): l is Layer => Boolean(l));

  return (
    <Layout title={band.title} description={band.subject}>
      <main className="container margin-vert--lg">
        <BandNav current={id} />

        <h1>{band.title}</h1>

        <div style={{ ...s.bandGrid, marginBottom: "1.4rem" }}>
          <span style={s.bandK}>Subject</span>
          <span>{band.subject}</span>
          <span style={s.bandK}>Evidence</span>
          <span>{band.evidence}</span>
          <span style={s.bandK}>Gates CI</span>
          <span>{band.gates}</span>
        </div>

        <div style={{ ...s.pills, marginBottom: "1.8rem" }}>
          {(Object.keys(statusMeans) as Eval["status"][])
            .map((k) => [k, counts[k]] as const)
            .filter(([, n]) => n > 0)
            .map(([k, n]) => (
              <Pill key={k} text={`${n} ${statusLabel[k]}`} t={statusTone[k]} />
            ))}
        </div>

        {layers.map((l) => (
          <LayerBlock key={l.id} l={l} />
        ))}
      </main>
    </Layout>
  );
}
