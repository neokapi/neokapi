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

  // Seed the workspace content memory and terms so the editor's context panel,
  // the review context rail and the governance walk (memory search "mission",
  // multi-locale concepts) show real content. POST sequentially: concurrent
  // writes to the workspace stores race. A refused write fails the seed, so a
  // retired route or a governed status surfaces here rather than as an empty
  // card in the recording.
  const MEMORY_ENTRIES = [
    { source: "About Acme Inc.", target: "À propos d'Acme Inc.", source_locale: "en", target_locale: "fr" },
    { source: "Our Mission", target: "Notre mission", source_locale: "en", target_locale: "fr" },
    { source: "Our Team", target: "Notre équipe", source_locale: "en", target_locale: "fr" },
    { source: "Our Values", target: "Nos valeurs", source_locale: "en", target_locale: "fr" },
    { source: "Get in Touch", target: "Contactez-nous", source_locale: "en", target_locale: "fr" },
    {
      source: "We believe every developer deserves reliable, fast, and secure infrastructure.",
      target: "Nous pensons que chaque développeur mérite une infrastructure fiable, rapide et sécurisée.",
      source_locale: "en",
      target_locale: "fr",
    },
    { source: "Our Mission", target: "Unsere Mission", source_locale: "en", target_locale: "de" },
    { source: "Our Team", target: "Unser Team", source_locale: "en", target_locale: "de" },
  ];
  for (const e of MEMORY_ENTRIES) {
    await jpost(`/${wsSlug}/translation-memory`, { ...e, project_id: projectId });
  }
  // Workspace terms are concepts (POST /:ws/concepts, server/handlers_concepts.go).
  // The direct route creates a term `approved` or `deprecated`; `preferred` and
  // `forbidden` are governed statuses it refuses with a 409, because they
  // travel through a reviewed change-set.
  const CONCEPTS = [
    {
      domain: "cloud",
      definition: "Managed, multi-tenant compute and storage delivered over the network.",
      terms: [
        { text: "cloud infrastructure", locale: "en", status: "approved" },
        { text: "infrastructure cloud", locale: "fr", status: "approved" },
        { text: "Cloud-Infrastruktur", locale: "de", status: "approved" },
      ],
    },
    {
      domain: "reliability",
      definition: "The proportion of time a service is operational and reachable.",
      terms: [
        { text: "uptime", locale: "en", status: "approved" },
        { text: "disponibilité", locale: "fr", status: "approved" },
        { text: "Verfügbarkeit", locale: "de", status: "approved" },
      ],
    },
    {
      domain: "security",
      definition: "Protecting data so only authorised parties can read it, end to end.",
      terms: [
        { text: "encryption", locale: "en", status: "approved" },
        { text: "chiffrement", locale: "fr", status: "approved" },
        { text: "cryptage", locale: "fr", status: "deprecated" },
        { text: "Verschlüsselung", locale: "de", status: "approved" },
      ],
    },
  ];
  for (const concept of CONCEPTS) {
    await jpost(`/${wsSlug}/concepts`, { ...concept, project_id: projectId });
  }

  // Invite Bob as a member, then accept the invite AS Bob (a second user).
  // This is the genuine membership path — Bob is a real, distinct user who can
  // open the same file and whose presence the collab server relays to Alice.
  const bobToken = await deviceAuth(BOB_EMAIL, BOB_NAME);
  const HB = { Authorization: `Bearer ${bobToken}`, "Content-Type": "application/json" };

  const invite = await jpost(`/${wsSlug}/invites`, {
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

  // ── The review walk's separation of duties ────────────────────────────────
  //
  // The workspace policy decides what happens when someone approves their own
  // writing: `off` ignores it, `warn` (the default, auth/governance.go
  // GetSoDMode) files an audit record and allows it, `block` refuses with
  // "separation of duties: you cannot review or approve your own work"
  // (server/handlers_governance.go). Only `block` puts the rule on screen, so
  // the seed sets it.
  //
  // Two more facts decide who gets refused. `ai-translate` runs in the caller's
  // request context, so every target it writes is authored by Alice
  // (store/history.go records ChangeContext.Actor). And a plain workspace
  // member carries translate but not review (core/auth DefaultPermissionsForRole),
  // so Bob's Approve would be a disabled button rather than a refusal. The seed
  // therefore grants Bob the reviewer role scoped to the target locale, and has
  // Bob write one target of his own. The result is a queue where Alice can
  // approve what Bob wrote, Alice is refused on what she wrote, and Bob can
  // decide it instead.
  const jput = async (path, body, headers = HA) => {
    const r = await fetch(`${API}${path}`, { method: "PUT", headers, body: JSON.stringify(body) });
    if (!r.ok) throw new Error(`PUT ${path} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
    const text = await r.text();
    return text ? JSON.parse(text) : {};
  };
  const jget = async (path, headers = HA) => {
    const r = await fetch(`${API}${path}`, { headers });
    if (!r.ok) throw new Error(`GET ${path} → ${r.status}: ${(await r.text()).slice(0, 300)}`);
    return r.json();
  };

  let sodMode = "";
  let bobBlockId = "";
  let aliceBlockId = "";
  try {
    ({ mode: sodMode } = await jput(`/${wsSlug}/sod`, { mode: "block" }));

    // Bob's own user id, read as Bob (the workspace member list carries no email).
    const bobMe = await jget("/auth/me", HB);
    const bobId = bobMe.id || bobMe.user?.id;

    // The built-in reviewer template: view_content + translate + review.
    const roles = await jget(`/${wsSlug}/roles`);
    const roleList = Array.isArray(roles) ? roles : roles.roles || [];
    const reviewer =
      roleList.find((r) => r.name === "reviewer") ||
      roleList.find((r) => (r.permission_names || []).includes("review"));
    if (!bobId || !reviewer) throw new Error("no bob id or reviewer role template");

    await jpost(`/${wsSlug}/${projectId}/members`, {
      user_id: bobId,
      role_id: reviewer.id,
      languages: [LOCALE],
    });

    // Bob writes one target. His PUT is the newest target_modified row for that
    // block and locale, so LastTargetAuthors answers with Bob for it and with
    // Alice for the rest.
    const blocks = await jget(`/${wsSlug}/${projectId}/blocks/main?item=${encodeURIComponent(FILE_NAME)}`);
    const blockList = Array.isArray(blocks) ? blocks : blocks.blocks || [];
    const translatable = blockList.filter((b) => b.translatable !== false);
    bobBlockId = translatable[0]?.id || "";
    aliceBlockId = translatable[1]?.id || translatable[0]?.id || "";
    if (bobBlockId) {
      await jput(`/${wsSlug}/${projectId}/blocks/main/${bobBlockId}`, {
        item_name: FILE_NAME,
        target_locale: LOCALE,
        text: "Nous concevons des outils que les équipes utilisent chaque jour.",
      }, HB);
    }
  } catch (e) {
    console.error(`  (review governance seed skipped: ${e.message})`);
  }

  console.log(
    JSON.stringify(
      {
        base: BASE,
        workspace: wsSlug,
        project_id: projectId,
        item_id: itemId,
        file_name: FILE_NAME,
        locale: LOCALE,
        // The item sits AFTER /translate/ (routes/index.tsx `translate/$`).
        translate_url: `${BASE}/${wsSlug}/p/${projectId}/s/main/translate/${itemId}`,
        review_url: `${BASE}/${wsSlug}/p/${projectId}/s/main/review`,
        review_inbox_url: `${BASE}/${wsSlug}/review-inbox`,
        members_url: `${BASE}/${wsSlug}/settings/members`,
        sod_mode: sodMode,
        // Queue rows are keyed `${itemId}::${blockId}::${locale}`.
        peer_block_id: bobBlockId,
        self_block_id: aliceBlockId,
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
