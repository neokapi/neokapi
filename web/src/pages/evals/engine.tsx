import type { ReactElement } from "react";
import { BandPage } from "./_band";

// Route for the "engine" band. The page itself is _band.tsx; this file exists
// so Docusaurus has something to turn into /evals/engine.
export default function Page(): ReactElement {
  return <BandPage id="engine" />;
}
