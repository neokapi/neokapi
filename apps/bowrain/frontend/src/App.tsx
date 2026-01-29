import { useState } from "react";
import { Sidebar, type View } from "./components/Sidebar";
import { Header } from "./components/Header";
import { FormatList } from "./components/FormatList";
import { ToolList } from "./components/ToolList";
import { FlowList } from "./components/FlowList";
import { ConvertPanel } from "./components/ConvertPanel";
import { TranslatePanel } from "./components/TranslatePanel";
import { useHealth } from "./hooks/useApi";

function App() {
  const [activeView, setActiveView] = useState<View>("formats");
  const { health } = useHealth();

  const renderView = () => {
    switch (activeView) {
      case "formats":
        return <FormatList />;
      case "tools":
        return <ToolList />;
      case "flows":
        return <FlowList />;
      case "convert":
        return <ConvertPanel />;
      case "translate":
        return <TranslatePanel />;
    }
  };

  return (
    <div
      style={{
        display: "flex",
        height: "100vh",
        overflow: "hidden",
      }}
    >
      <Sidebar activeView={activeView} onViewChange={setActiveView} />
      <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        <Header health={health} />
        <main
          style={{
            flex: 1,
            padding: 24,
            overflowY: "auto",
          }}
        >
          {renderView()}
        </main>
      </div>
    </div>
  );
}

export default App;
