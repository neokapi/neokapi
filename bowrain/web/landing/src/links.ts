// Shared outbound links. The app lives at app.bowrain.cloud (DECISIONS L6
// splittable URLs); contact is on the bowrain.cloud domain we control —
// never the unverified .com mailbox (epic 011).
export const APP_URL = "https://app.bowrain.cloud";
export const SIGNUP_URL = `${APP_URL}/`;
export const CONTACT_EMAIL = "hello@bowrain.cloud";
export const GITHUB_URL = "https://github.com/neokapi";
export const KAPI_SITE_URL = "https://neokapi.github.io/web/neokapi/";

// Relative to the deploy base: /web/bowrain/docs/ on GitHub Pages today,
// /docs/ once bowrain.cloud serves the site.
export const docsUrl = () => `${import.meta.env.BASE_URL}docs/`;
