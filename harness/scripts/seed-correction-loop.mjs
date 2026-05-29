#!/usr/bin/env node
// Seed the bowrain backend with the state the correction-learning loop video
// (harness/demos/bowrain-web-correction-loop) records: a workspace, a brand
// profile, a project with content that uses some off-brand terms, and a stream
// of corrections that aggregate into candidate rules on the review page.
//
// Idempotent-ish: re-running creates a fresh, uniquely-slugged workspace and
// prints its slug + the brand profile id + a session token to stdout as JSON,
// which the recorder consumes (BOWRAIN_SESSION_TOKEN + the route params).
//
//   node harness/scripts/seed-correction-loop.mjs            # uses http://localhost:8080
//   BOWRAIN_BACKEND_URL=… node harness/scripts/seed-correction-loop.mjs
//
// Requires the bowrain stack running (make -C bowrain stack-up).

const BASE = process.env.BOWRAIN_BACKEND_URL || "http://localhost:8080";
const API = `${BASE}/api/v1`;
const EMAIL = process.env.BOWRAIN_SEED_EMAIL || "admin@example.com";

async function deviceAuth() {
  if (process.env.BOWRAIN_TOKEN) return process.env.BOWRAIN_TOKEN;
  const start = await (
    await fetch(`${API}/auth/device/start`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: "client_id=e2e-shared",
    })
  ).json();
  await fetch(`${API}/auth/device/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `user_code=${start.user_code}&email=${encodeURIComponent(EMAIL)}&name=${encodeURIComponent("Demo User")}`,
    redirect: "manual",
  });
  const poll = await (
    await fetch(`${API}/auth/device/poll`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: `device_code=${start.device_code}&grant_type=urn:ietf:params:oauth:grant-type:device_code`,
    })
  ).json();
  if (!poll.access_token) throw new Error("device auth: no access_token");
  return poll.access_token;
}

async function main() {
  const token = await deviceAuth();
  const H = { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
  const jpost = async (path, body) => {
    const r = await fetch(`${API}${path}`, { method: "POST", headers: H, body: JSON.stringify(body) });
    if (!r.ok) throw new Error(`POST ${path} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
    return r.json();
  };

  // Unique workspace so re-runs are clean. (Seconds-since-epoch via Date is fine
  // in a plain Node script — this is not a workflow sandbox.)
  const stamp = Math.floor(Date.now() / 1000) % 100000;
  const slug = `brand-loop-${stamp}`;
  const ws = await jpost("/workspaces", { name: `Brand Loop ${stamp}`, slug });
  const wsSlug = ws.slug || slug;

  // A brand profile. Its vocabulary starts minimal — the candidates the demo
  // promotes come from the correction stream below, not the seed profile.
  const profile = await jpost(`/${wsSlug}/brand-profiles`, {
    name: "Acme Voice",
    description: "Acme's brand voice — clear, direct, no jargon.",
    tone: { personality: ["clear", "direct"], formality: "neutral", emotion: "warm", humor: "light" },
  });
  const profileId = profile.id;

  // A project with content that uses the off-brand terms, so "Preview impact"
  // has real blocks to evaluate the candidate rule against.
  const project = await jpost(`/${wsSlug}/projects`, {
    name: "Marketing Site",
    default_source_language: "en",
    target_languages: ["fr", "de"],
  });
  const projectId = project.id || project.project?.id;

  // The correction stream: each (original → corrected) repeated past the
  // min-count threshold (3) so it surfaces as a candidate on the review page.
  const stream = [
    { original: "utilize", corrected: "use", n: 5 },
    { original: "leverage", corrected: "use", n: 4 },
    { original: "synergy", corrected: "collaboration", n: 3 },
    { original: "best-in-class", corrected: "proven", n: 3 },
    { original: "kindly", corrected: "please", n: 1 }, // below threshold — stays a non-candidate
  ];
  let posted = 0;
  for (const c of stream) {
    for (let i = 0; i < c.n; i++) {
      await jpost(`/${wsSlug}/${projectId}/brand-voice/main/corrections`, {
        profile_id: profileId,
        block_id: `seed-${c.original}-${i}`,
        dimension: "vocabulary",
        original_text: c.original,
        corrected_text: c.corrected,
      });
      posted++;
    }
  }

  // Verify candidates surfaced.
  const cand = await (
    await fetch(`${API}/${wsSlug}/brand-profiles/${profileId}/candidates?min_count=3`, { headers: H })
  ).json();

  console.log(
    JSON.stringify(
      {
        base: BASE,
        workspace: wsSlug,
        profile_id: profileId,
        project_id: projectId,
        corrections_posted: posted,
        candidates: Array.isArray(cand) ? cand.map((c) => `${c.term}→${c.replacement} (${c.correction_count})`) : cand,
        review_url: `${BASE}/${wsSlug}/brand/review/${profileId}`,
        token,
      },
      null,
      2,
    ),
  );
}

main().catch((e) => {
  console.error("seed failed:", e.message);
  process.exit(1);
});
