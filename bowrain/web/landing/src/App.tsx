import { Nav } from "./components/Nav";
import { Hero } from "./components/Hero";
import { Problem } from "./components/Problem";
import { OpenSource } from "./components/OpenSource";
import { SourceGovernance } from "./components/SourceGovernance";
import { PseudoChallenge } from "./components/PseudoChallenge";
import { BrandChallenge } from "./components/BrandChallenge";
import { Platform } from "./components/Platform";
import { Desktop } from "./components/Desktop";
import { Plans } from "./components/Plans";
import { CTA } from "./components/CTA";
import { Footer } from "./components/Footer";

const Divider = () => (
  <div className="mx-auto max-w-6xl px-6">
    <hr className="border-neutral-800/50" />
  </div>
);

function App() {
  return (
    <div className="min-h-screen bg-neutral-950 text-neutral-100">
      <Nav />
      <Hero />
      <Divider />
      <Problem />
      <Divider />
      <PseudoChallenge />
      <Divider />
      <SourceGovernance />
      <Divider />
      <BrandChallenge />
      <Divider />
      <OpenSource />
      <Divider />
      <Platform />
      <Divider />
      <Desktop />
      <Divider />
      <Plans />
      <Divider />
      <CTA />
      <Footer />
    </div>
  );
}

export default App;
