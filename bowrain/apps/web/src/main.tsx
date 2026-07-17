import "@fontsource-variable/dm-sans";
import "@fontsource/dm-mono/300.css";
import "@fontsource/dm-mono/400.css";
import "@fontsource/dm-mono/500.css";
import "./index.css";
import { createRoot } from "react-dom/client";
import { BowrainApp, webPlatform } from "@neokapi/bowrain-app";
import { api } from "./api";
import { initPostHog, identifyUser, resetPostHog, captureEvent, groupIdentify } from "./posthog";

initPostHog();

// Web host: analytics flow through the platform seam so the shared app never
// imports posthog-js directly. `capture` carries the product events (SPA
// $pageview, feature_entered, and the form/action events fired by the shared
// components); `group` scopes the session to the active workspace.
const platform = webPlatform({
  analytics: {
    identify: (user) => identifyUser(user.id, { email: user.email, name: user.name }),
    reset: resetPostHog,
    capture: captureEvent,
    group: groupIdentify,
  },
});

createRoot(document.getElementById("root")!).render(<BowrainApp api={api} platform={platform} />);
