import { Nav } from "./components/Nav";
import { Hero } from "./components/Hero";
import { Rename } from "./components/Rename";
import { Coordinates } from "./components/Coordinates";
import { Loop } from "./components/Loop";
import { Proof } from "./components/Proof";
import { Product } from "./components/Product";
import { Apps } from "./components/Apps";
import { OpenSource } from "./components/OpenSource";
import { Languages } from "./components/Languages";
import { Plans } from "./components/Plans";
import { CTA } from "./components/CTA";
import { Footer } from "./components/Footer";

const Divider = () => (
  <div className="mx-auto max-w-6xl px-6">
    <hr className="border-border/50" />
  </div>
);

// The page tells one argument in six beats, monolingual throughout until the
// last one:
//
//   Hero         what Bowrain is
//   Rename       why a term list cannot carry it
//   Coordinates  what a point is, and what inherits down to it
//   Loop         where the rules come from
//   Proof        a check you can run, and where the same one runs
//   Languages    a language is one more coordinate — ship states close it
//
// Product, Apps and OpenSource are reference material for a buyer who is
// already convinced, placed after the argument they answer questions about and
// before the price. Languages stays last of the six so the page closes at the
// delivery edge, directly above the plans and the call to action.
function App() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <Nav />
      <Hero />
      <Divider />
      <Rename />
      <Divider />
      <Coordinates />
      <Divider />
      <Loop />
      <Divider />
      <Proof />
      <Divider />
      <Product />
      <Divider />
      <Apps />
      <Divider />
      <OpenSource />
      <Divider />
      <Languages />
      <Divider />
      <Plans />
      <Divider />
      <CTA />
      <Footer />
    </div>
  );
}

export default App;
