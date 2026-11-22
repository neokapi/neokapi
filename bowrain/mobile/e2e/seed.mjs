// Seeds the local stack with work for the Maestro flows and prints the -e
// arguments they take:
//
//   node e2e/seed.mjs            # after `make -C bowrain stack-up-e2e`
//
// The test user gets a project in their workspace with one file. A second
// account joins the workspace and pseudo-translates the file into Norwegian
// Bokmål, so every string waits for the test user's review: a reviewer may not
// approve a translation they wrote. Each run deletes the projects of earlier
// runs and makes a new one, so the deck starts full.
//
// Both accounts sign in through the device flow, which the local stack
// approves for the email it is given (BOWRAIN_ALLOW_INSECURE_DEV in
// compose.full.yaml). The script refuses any server but a local one.
import { readFile } from "node:fs/promises";

const server = process.env.SERVER_URL || "http://localhost:8080";
const reviewer = process.env.E2E_USERNAME || "e2e-reviewer@example.com";
const translator = process.env.E2E_TRANSLATOR || "e2e-translator@example.com";
const locale = process.env.LOCALE || "nb";
const projectPrefix = "Mobile e2e ";
const api = `${server}/api/v1`;

if (!/^http:\/\/(localhost|127\.0\.0\.1|10\.0\.2\.2)(:\d+)?$/.test(server)) {
  throw new Error(`refusing to seed ${server}: the seed is for a local stack only`);
}

async function call(method, path, { token, json, form, body } = {}) {
  const headers = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  if (json !== undefined) headers["Content-Type"] = "application/json";
  if (form !== undefined) headers["Content-Type"] = "application/x-www-form-urlencoded";
  const resp = await fetch(`${api}${path}`, {
    method,
    headers,
    body: json !== undefined ? JSON.stringify(json) : (form ?? body),
    redirect: "manual",
  });
  if (!resp.ok && resp.status !== 302) {
    throw new Error(`${method} ${path}: ${resp.status} ${await resp.text()}`);
  }
  const type = resp.headers.get("content-type") ?? "";
  return type.includes("application/json") ? resp.json() : null;
}

/** A token for the account with this email, through the device flow. */
async function signIn(email, name) {
  const start = await call("POST", "/auth/device/start", { form: "client_id=mobile-e2e" });
  await call("POST", "/auth/device/verify", {
    form: new URLSearchParams({ user_code: start.user_code, email, name }),
  });
  const poll = await call("POST", "/auth/device/poll", {
    form: new URLSearchParams({
      device_code: start.device_code,
      grant_type: "urn:ietf:params:oauth:grant-type:device_code",
    }),
  });
  return poll.access_token;
}

/** The slug of the account's own workspace, made on first use. */
async function ownWorkspace(token, fallbackSlug) {
  const state = await call("GET", "/auth/me/onboarding", { token });
  if (state.needs_onboarding) {
    await call("POST", "/auth/me/onboarding", {
      token,
      json: { slug: state.suggested_slug || fallbackSlug },
    });
  }
  const me = await call("GET", "/auth/me", { token });
  const list = await call("GET", "/workspaces", { token });
  const own = list.find((w) => w.owner_id === me.id) ?? list[0];
  if (!own) throw new Error("the account has no workspace");
  return own.slug;
}

const reviewerToken = await signIn(reviewer, "E2E Reviewer");
const translatorToken = await signIn(translator, "E2E Translator");
const ws = await ownWorkspace(reviewerToken, "e2e-reviewer");

const invite = await call("POST", `/${ws}/invites`, {
  token: reviewerToken,
  json: { role: "admin", email: translator, max_uses: 1, ttl_days: 1 },
});
await call("POST", `/join/${invite.code}`, { token: translatorToken });

// A free workspace holds three projects, so the projects of earlier runs go.
for (const p of await call("GET", `/${ws}/projects`, { token: reviewerToken })) {
  if (p.name.startsWith(projectPrefix)) {
    await call("DELETE", `/${ws}/${p.id}`, { token: reviewerToken });
  }
}

const project = await call("POST", `/${ws}/projects`, {
  token: reviewerToken,
  json: {
    name: `${projectPrefix}${new Date().toISOString().slice(0, 16).replace("T", " ")}`,
    default_source_language: "en",
    target_languages: [locale],
  },
});

const file = "strings.json";
const content = await readFile(new URL("./seed-strings.json", import.meta.url));
const form = new FormData();
form.append("files", new Blob([content]), file);
await call("POST", `/${ws}/${project.id}/items/main`, { token: reviewerToken, body: form });
const done = await call(
  "POST",
  `/${ws}/${project.id}/actions/main/pseudo-translate?item=${encodeURIComponent(file)}`,
  { token: translatorToken, json: { target_locale: locale } },
);
if (!done.translated_blocks) throw new Error("pseudo-translation produced no translations");

console.log(`-e WORKSPACE=${ws} -e PROJECT=${project.id} -e LOCALE=${locale}`);
