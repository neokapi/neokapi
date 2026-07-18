import "@fontsource-variable/dm-sans";
import "@fontsource/dm-mono/300.css";
import "@fontsource/dm-mono/400.css";
import "@fontsource/dm-mono/500.css";
import "./index.css";
import { createRoot } from "react-dom/client";
import { BowrainApp, webPlatform } from "@neokapi/bowrain-app";
import { ErrorNotice } from "@neokapi/ui";
import { api } from "./api";
import { initPostHog, identifyUser, resetPostHog, captureEvent, groupIdentify } from "./posthog";
import { initSentry, Sentry } from "./sentry";

initSentry();
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

// Top-level ErrorBoundary: catches render crashes (the app had none), shows the
// canonical ErrorNotice, and reports the crash to Sentry when configured. The
// boundary works with or without a DSN — capture is simply a no-op when Sentry
// is disabled.
createRoot(document.getElementById("root")!).render(
  <Sentry.ErrorBoundary
    fallback={(props) => (
      <div className="mx-auto mt-24 max-w-lg px-4">
        <ErrorNotice
          error={props.error}
          title="The app hit an unexpected error"
          onRetry={() => props.resetError()}
          variant="panel"
        />
      </div>
    )}
  >
    <BowrainApp api={api} platform={platform} />
  </Sentry.ErrorBoundary>,
);
