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

// Start with a scoped decision and its review workflow, then explain the
// context model and the bounded local rule example. Keep section IDs stable
// for navigation and attribution.
function App() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <Nav />
      <Hero />
      <Divider />
      <Rename />
      <Divider />
      <Loop />
      <Divider />
      <Coordinates />
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
