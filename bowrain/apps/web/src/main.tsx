import "@fontsource-variable/dm-sans";
import "@fontsource/dm-mono/300.css";
import "@fontsource/dm-mono/400.css";
import "@fontsource/dm-mono/500.css";
import "./index.css";
import { createRoot } from "react-dom/client";
import { BowrainApp, webPlatform } from "@neokapi/bowrain-app";
import { api } from "./api";
import { initPostHog, identifyUser, resetPostHog } from "./posthog";

initPostHog();

// Web host: analytics flow through the platform seam so the shared app never
// imports posthog-js directly.
const platform = webPlatform({
  analytics: {
    identify: (user) => identifyUser(user.id, { email: user.email, name: user.name }),
    reset: resetPostHog,
  },
});

createRoot(document.getElementById("root")!).render(<BowrainApp api={api} platform={platform} />);
