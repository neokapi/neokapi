import "./index.css";
import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";

// Mark the root in the native macOS desktop shell so shared @neokapi/ui
// components reserve the traffic-light safe area (e.g. the sidebar workspace
// switcher clears the OS traffic lights). The plain-browser web app never sets
// this class, so it gets no gutter. Mirrors kapi-desktop's traffic-light gutter.
if (typeof navigator !== "undefined" && navigator.platform.startsWith("Mac")) {
  document.documentElement.classList.add("bw-desktop-mac");
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
