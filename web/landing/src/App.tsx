import { Nav } from './components/Nav'
import { Hero } from './components/Hero'
import { Differentiators } from './components/Differentiators'
import { BrandLoop } from './components/BrandLoop'
import { Features } from './components/Features'
import { CliDemo } from './components/CliDemo'
import { SeeItInAction } from './components/SeeItInAction'
import { Formats } from './components/Formats'
import { DesktopDemo } from './components/DesktopDemo'
import { Pipeline } from './components/Pipeline'
import { OkapiMapping } from './components/OkapiMapping'
import { GetStarted } from './components/GetStarted'
import { Footer } from './components/Footer'

const Divider = () => (
  <div className="mx-auto max-w-6xl px-6">
    <hr className="border-surface-700/40" />
  </div>
)

function App() {
  return (
    <div className="min-h-screen bg-surface-950 text-neutral-100">
      <Nav />
      <Hero />
      <Divider />
      <Differentiators />
      <Divider />
      <BrandLoop />
      <Divider />
      <Features />
      <Divider />
      <CliDemo />
      <Divider />
      <SeeItInAction />
      <Divider />
      <Formats />
      <Divider />
      <DesktopDemo />
      <Divider />
      <Pipeline />
      <Divider />
      <OkapiMapping />
      <Divider />
      <GetStarted />
      <Footer />
    </div>
  )
}

export default App
