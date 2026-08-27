import type { ReactElement } from "react";
import { BandPage } from "./_band";

// Route for the "ai" band. The page itself is _band.tsx; this file exists
// so Docusaurus has something to turn into /evals/ai.
export default function Page(): ReactElement {
  return <BandPage id="ai" />;
}
