import { Nav } from './components/Nav'
import { Hero } from './components/Hero'
import { Features } from './components/Features'
import { CliDemo } from './components/CliDemo'
import { DesktopDemo } from './components/DesktopDemo'
import { Pipeline } from './components/Pipeline'
import { Formats } from './components/Formats'
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
      <Features />
      <Divider />
      <CliDemo />
      <Divider />
      <DesktopDemo />
      <Divider />
      <Pipeline />
      <Divider />
      <Formats />
      <Divider />
      <OkapiMapping />
      <Divider />
      <GetStarted />
      <Footer />
    </div>
  )
}

export default App
