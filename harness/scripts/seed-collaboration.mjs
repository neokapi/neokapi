#!/usr/bin/env node
// Seed the bowrain backend with the state the two-user collaboration video
// (harness/demos/bowrain-web-collaboration) records: one shared workspace, a
// project with a real HTML file (so the Translate editor has genuine blocks),
// and a SECOND workspace member (Bob) so the recorder can open a second,
// off-camera authenticated session in the same file — making Bob's presence
// avatar appear live on the recorded (Alice) screen via the real collab
// WebSocket. Nothing about the collaboration is faked: two distinct
// device-flow users join the same Yjs room and the server relays their
// awareness to each other.
//
// Idempotent-ish: re-running creates a fresh, uniquely-slugged workspace and
// prints, as JSON to stdout, everything the recorder consumes:
//   { base, workspace, project_id, item_id, locale,
//     alice: { token, name, email },
//     bob:   { token, name, email } }
//
//   node harness/scripts/seed-collaboration.mjs            # uses http://localhost:8080
//   BOWRAIN_BACKEND_URL=… node harness/scripts/seed-collaboration.mjs
//
// Requires the bowrain stack running (make -C bowrain stack-up-web).

const BASE = process.env.BOWRAIN_BACKEND_URL || "http://localhost:8080";
const API = `${BASE}/api/v1`;

const ALICE_EMAIL = process.env.BOWRAIN_ALICE_EMAIL || "admin@example.com";
const ALICE_NAME = process.env.BOWRAIN_ALICE_NAME || "Alex Rivera";
const BOB_EMAIL = process.env.BOWRAIN_BOB_EMAIL || "maria@acme.example";
const BOB_NAME = process.env.BOWRAIN_BOB_NAME || "Maria Schmidt";

/**
 * Run the device-auth flow for a specific email/name and return a JWT.
 * Unlike the e2e helper, this NEVER honours BOWRAIN_TOKEN — every call mints a
 * distinct user, which is the whole point of a two-user demo.
 */
async function deviceAuth(email, name) {
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
    body: `user_code=${start.user_code}&email=${encodeURIComponent(email)}&name=${encodeURIComponent(name)}`,
    redirect: "manual",
  });
  const poll = await (
    await fetch(`${API}/auth/device/poll`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: `device_code=${start.device_code}&grant_type=urn:ietf:params:oauth:grant-type:device_code`,
    })
  ).json();
  if (!poll.access_token) throw new Error(`device auth (${email}): no access_token`);
  return poll.access_token;
}

const ABOUT_US_HTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <title>About Us - Acme Inc.</title>
  </head>
  <body>
    <header>
      <h1>About Acme Inc.</h1>
      <p>Building the future of <strong>cloud infrastructure</strong> since 2018.</p>
    </header>
    <section id="mission">
      <h2>Our Mission</h2>
      <p>
        We believe every developer deserves reliable, fast, and secure infrastructure. Our platform
        handles over <em>10 million</em> requests per day across 42 countries.
      </p>
      <p>
        From startups to Fortune 500 companies, our customers trust us with their most critical
        workloads. We take that responsibility seriously.
      </p>
    </section>
    <section id="team">
      <h2>Our Team</h2>
      <p>
        We are a distributed team of 120 engineers, designers, and product specialists across
        <a href="/offices">12 offices worldwide</a>.
      </p>
    </section>
    <section id="values">
      <h2>Our Values</h2>
      <ul>
        <li><strong>Transparency</strong> &mdash; We share our roadmap and pricing openly.</li>
        <li><strong>Reliability</strong> &mdash; We maintain 99.99% uptime across all services.</li>
        <li><strong>Security</strong> &mdash; SOC 2 Type II certified with end-to-end encryption.</li>
      </ul>
    </section>
    <section id="contact">
      <h2>Get in Touch</h2>
      <p>
        Have questions? Reach out at
        <a href="mailto:hello@acme-inc.example">hello@acme-inc.example</a>.
      </p>
    </section>
  </body>
</html>
`;

const FILE_NAME = "about-us.html";
const LOCALE = "fr";

async function main() {
  // Alice is the owner/admin; Bob is the teammate she invites.
  const aliceToken = await deviceAuth(ALICE_EMAIL, ALICE_NAME);
  const HA = { Authorization: `Bearer ${aliceToken}`, "Content-Type": "application/json" };

  const jpost = async (path, body, headers = HA) => {
    const r = await fetch(`${API}${path}`, { method: "POST", headers, body: JSON.stringify(body) });
    if (!r.ok) throw new Error(`POST ${path} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
    return r.json();
  };

  // Unique workspace so re-runs are clean.
  const stamp = Math.floor(Date.now() / 1000) % 100000;
  const slug = `collab-${stamp}`;
  const ws = await jpost("/workspaces", { name: `Acme Localization ${stamp}`, slug });
  const wsSlug = ws.slug || slug;

  // A project with a real HTML file so the Translate editor renders genuine
  // blocks (h1/h2/paragraphs) that two people can stand in together.
  const project = await jpost(`/${wsSlug}/projects`, {
    name: "Company Website",
    default_source_language: "en",
    target_languages: ["fr", "de", "ja"],
  });
  const projectId = project.id || project.project?.id;

  // Upload the file via the AD-011 multipart items route (field name: files).
  {
    const form = new FormData();
    form.append("files", new Blob([ABOUT_US_HTML], { type: "text/html" }), FILE_NAME);
    const r = await fetch(`${API}/${wsSlug}/${projectId}/items/main`, {
      method: "POST",
      headers: { Authorization: `Bearer ${aliceToken}` },
      body: form,
    });
    if (!r.ok) throw new Error(`upload ${FILE_NAME} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
  }

  // Resolve the item id the editor route needs (/:ws/p/:pid/s/main/:itemId/translate).
  const proj = await (await fetch(`${API}/${wsSlug}/${projectId}`, { headers: HA })).json();
  const item = (proj.items || []).find((i) => i.name === FILE_NAME) || (proj.items || [])[0];
  const itemId = item?.id;
  if (!itemId) throw new Error(`no item id resolved for ${FILE_NAME} (items: ${JSON.stringify(proj.items)})`);

  // Pre-translate so the file has visible target content for both users to see.
  // Best-effort: the offline demo provider handles this on the local stack.
  try {
    await jpost(`/${wsSlug}/${projectId}/actions/main/ai-translate`, {
      item: FILE_NAME,
      target_locale: LOCALE,
    });
  } catch (e) {
    console.error(`  (ai-translate skipped: ${e.message})`);
  }

  // Invite Bob as a member, then accept the invite AS Bob (a second user).
  // This is the genuine membership path — Bob is a real, distinct user who can
  // open the same file and whose presence the collab server relays to Alice.
  const bobToken = await deviceAuth(BOB_EMAIL, BOB_NAME);
  const HB = { Authorization: `Bearer ${bobToken}`, "Content-Type": "application/json" };

  const invite = await jpost(`/workspaces/${wsSlug}/invites`, {
    role: "member",
    email: BOB_EMAIL,
    max_uses: 1,
    ttl_days: 1,
  });
  const code = invite.code || invite.invite?.code;
  if (!code) throw new Error(`no invite code in response: ${JSON.stringify(invite)}`);
  await jpost(`/join/${code}`, {}, HB);

  // Confirm Bob can now see the workspace (membership took effect).
  const bobWs = await (await fetch(`${API}/workspaces`, { headers: HB })).json();
  const bobHasWs = (Array.isArray(bobWs) ? bobWs : bobWs.workspaces || []).some(
    (w) => w.slug === wsSlug,
  );

  console.log(
    JSON.stringify(
      {
        base: BASE,
        workspace: wsSlug,
        project_id: projectId,
        item_id: itemId,
        file_name: FILE_NAME,
        locale: LOCALE,
        translate_url: `${BASE}/${wsSlug}/p/${projectId}/s/main/${itemId}/translate`,
        members_url: `${BASE}/${wsSlug}/settings/members`,
        alice: { token: aliceToken, name: ALICE_NAME, email: ALICE_EMAIL },
        bob: { token: bobToken, name: BOB_NAME, email: BOB_EMAIL, joined: bobHasWs },
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
