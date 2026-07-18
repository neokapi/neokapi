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

// React 19 root error hooks → Sentry. This is what actually captures route
// errors: TanStack Router wraps each route in its own error boundary
// (CatchBoundary), so a render error on a route is caught there BEFORE it
// reaches the app-level ErrorBoundary below or the global window.onerror
// handler — meaning Sentry would otherwise never see it. onCaughtError fires
// for errors caught by ANY boundary (including the router's); onUncaughtError
// and onRecoverableError cover the rest. reactErrorHandler is a no-op when
// Sentry is not configured.
const root = createRoot(document.getElementById("root")!, {
  onCaughtError: Sentry.reactErrorHandler(),
  onUncaughtError: Sentry.reactErrorHandler(),
  onRecoverableError: Sentry.reactErrorHandler(),
});

// Top-level ErrorBoundary: last-resort catch for anything not caught by an
// inner boundary; shows the canonical ErrorNotice with a reference the user can
// quote. (The root hooks above are what capture the common route-level errors.)
root.render(
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
