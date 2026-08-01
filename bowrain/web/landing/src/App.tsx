import { Nav } from "./components/Nav";
import { Hero } from "./components/Hero";
import { Problem } from "./components/Problem";
import { Rename } from "./components/Rename";
import { Loop } from "./components/Loop";
import { Surfaces } from "./components/Surfaces";
import { Product } from "./components/Product";
import { BrandChallenge } from "./components/BrandChallenge";
import { Apps } from "./components/Apps";
import { OpenSource } from "./components/OpenSource";
import { Plans } from "./components/Plans";
import { CTA } from "./components/CTA";
import { Footer } from "./components/Footer";

const Divider = () => (
  <div className="mx-auto max-w-6xl px-6">
    <hr className="border-border/50" />
  </div>
);

function App() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <Nav />
      <Hero />
      <Divider />
      <Problem />
      <Divider />
      <Rename />
      <Divider />
      <Loop />
      <Divider />
      <Surfaces />
      <Divider />
      <Product />
      <Divider />
      <BrandChallenge />
      <Divider />
      <Apps />
      <Divider />
      <OpenSource />
      <Divider />
      <Plans />
      <Divider />
      <CTA />
      <Footer />
    </div>
  );
}

export default App;
