/**
 * Recording-only entry for Kapi Desktop walkthroughs. Mounts {@link DemoApp}
 * (real chrome + real TM/termbase browsers, mock data) so the harness can drive
 * the genuine UI in a browser. `?theme=dark` records the dark palette.
 */
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "../index.css";
import React from "react";
import ReactDOM from "react-dom/client";
import DemoApp from "./DemoApp";

const params = new URLSearchParams(window.location.search);
document.documentElement.classList.toggle("dark", params.get("theme") === "dark");

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <DemoApp />
  </React.StrictMode>,
);
