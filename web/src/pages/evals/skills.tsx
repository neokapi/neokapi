import type { ReactElement } from "react";
import { BandPage } from "./_band";

// Route for the "skills" band. The page itself is _band.tsx; this file exists
// so Docusaurus has something to turn into /evals/skills.
export default function Page(): ReactElement {
  return <BandPage id="skills" />;
}
