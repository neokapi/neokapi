/**
 * PR-preview origin: serves the static preview trees stored in R2 under the
 * `previews/` prefix, with clean-URL resolution so the trailingSlash:false
 * Docusaurus sites, the kapi/bowrain Storybooks, and the bowrain Vite landing
 * all resolve the same way they did on GitHub Pages.
 *
 * Request  https://preview.bowrain.cloud/web/prs/123/neokapi/docs/intro
 *   → R2 key candidates under previews/:
 *       previews/web/prs/123/neokapi/docs/intro            (exact, e.g. an asset)
 *       previews/web/prs/123/neokapi/docs/intro.html       (clean URL → file.html)
 *       previews/web/prs/123/neokapi/docs/intro/index.html (clean URL → dir index)
 *
 * R2 alone cannot do this — a bare custom domain returns the object at the exact
 * key or 404s — which is why previews need this Worker rather than a plain
 * bucket binding.
 */

export interface Env {
  PREVIEWS: R2Bucket;
}

const PREFIX = "previews";

// Authoritative content types by extension. aws s3 sync's guesses are
// inconsistent (notably .wasm and .mjs), and the runtime depends on
// application/wasm for streaming compilation, so the Worker sets them itself.
const CONTENT_TYPES: Record<string, string> = {
  html: "text/html; charset=utf-8",
  js: "text/javascript; charset=utf-8",
  mjs: "text/javascript; charset=utf-8",
  css: "text/css; charset=utf-8",
  json: "application/json; charset=utf-8",
  map: "application/json; charset=utf-8",
  svg: "image/svg+xml",
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  webp: "image/webp",
  gif: "image/gif",
  ico: "image/x-icon",
  webm: "video/webm",
  mp4: "video/mp4",
  woff: "font/woff",
  woff2: "font/woff2",
  ttf: "font/ttf",
  wasm: "application/wasm",
  txt: "text/plain; charset=utf-8",
  xml: "application/xml",
  onnx: "application/octet-stream",
  // Pre-gzipped wasm (kapi-cli.wasm.gz). Served as an OPAQUE blob with NO
  // Content-Encoding: the runtime fetches `${wasmUrl}.gz` and self-inflates via
  // DecompressionStream. Setting content-encoding here would make the browser
  // double-inflate. Same contract as publish-cdn-assets.sh / GitHub Pages.
  gz: "application/octet-stream",
};

function ext(path: string): string {
  const base = path.slice(path.lastIndexOf("/") + 1);
  const dot = base.lastIndexOf(".");
  return dot === -1 ? "" : base.slice(dot + 1).toLowerCase();
}

// Hashed build assets (Docusaurus/Vite/Storybook emit content-hashed names) are
// immutable; HTML and other entry docs must revalidate so a re-pushed preview
// is picked up promptly.
function cacheControl(key: string): string {
  const e = ext(key);
  if (e === "html" || e === "") return "public, max-age=0, must-revalidate";
  if (e === "wasm" || e === "gz" || e === "onnx" || e === "webm" || e === "mp4")
    return "public, max-age=86400";
  return "public, max-age=3600";
}

// Candidate R2 keys for a request path, in priority order. Mirrors how a static
// host resolves clean URLs for a trailingSlash:false build.
function candidates(pathname: string): string[] {
  // Collapse duplicate slashes; never let `..` escape the prefix.
  const clean = pathname.replace(/\/{2,}/g, "/").replace(/\/\.\.(\/|$)/g, "/");
  const base = `${PREFIX}${clean}`;
  if (clean.endsWith("/")) return [`${base}index.html`];
  const last = clean.slice(clean.lastIndexOf("/") + 1);
  if (last.includes(".")) return [base]; // looks like a file → exact only
  return [base, `${base}.html`, `${base}/index.html`];
}

// The deepest preview-root 404.html for a path, so a missing Docusaurus route
// renders that site's themed 404 rather than a bare string. Walks up to the
// `web/prs/<N>/<app>/…` or `storybook/prs/<N>/<app>/` root.
function notFoundKey(pathname: string): string | null {
  const m = pathname.match(/^(\/(?:web|storybook)\/prs\/\d+\/[^/]+(?:\/[^/]+)?)/);
  return m ? `${PREFIX}${m[1]}/404.html` : null;
}

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    if (req.method !== "GET" && req.method !== "HEAD") {
      return new Response("Method Not Allowed", { status: 405, headers: { allow: "GET, HEAD" } });
    }

    const url = new URL(req.url);
    let pathname: string;
    try {
      pathname = decodeURIComponent(url.pathname);
    } catch {
      return new Response("Bad Request", { status: 400 });
    }

    // Friendly index of what a preview path should look like.
    if (pathname === "/" || pathname === "") {
      return new Response(
        "neokapi PR previews. Open the link from the PR comment, e.g. /web/prs/<N>/neokapi/docs/.",
        { status: 404, headers: { "content-type": "text/plain; charset=utf-8" } },
      );
    }

    for (const key of candidates(pathname)) {
      const obj = await env.PREVIEWS.get(key, { onlyIf: req.headers });
      if (obj === null) continue;
      return respond(req, key, obj);
    }

    // Themed 404 if this app has one; else plain.
    const nfKey = notFoundKey(pathname);
    if (nfKey) {
      const nf = await env.PREVIEWS.get(nfKey);
      if (nf) {
        const headers = baseHeaders(nfKey, nf);
        headers.set("cache-control", "public, max-age=0, must-revalidate");
        return new Response(req.method === "HEAD" ? null : nf.body, { status: 404, headers });
      }
    }
    return new Response("Preview not found", {
      status: 404,
      headers: { "content-type": "text/plain; charset=utf-8" },
    });
  },
} satisfies ExportedHandler<Env>;

function baseHeaders(key: string, obj: R2Object): Headers {
  const headers = new Headers();
  obj.writeHttpMetadata(headers);
  headers.set("etag", obj.httpEtag);
  // Extension-derived content type wins over whatever s3 sync stored.
  const ct = CONTENT_TYPES[ext(key)];
  if (ct) headers.set("content-type", ct);
  else if (!headers.has("content-type")) headers.set("content-type", "application/octet-stream");
  return headers;
}

// `get` with onlyIf may return an R2Object (no body, 304-worthy) when the
// conditional matched; honour it so revisits are cheap.
function respond(req: Request, key: string, obj: R2ObjectBody | R2Object): Response {
  const headers = baseHeaders(key, obj);
  headers.set("cache-control", cacheControl(key));
  const body = obj as R2ObjectBody;
  if (!("body" in body) || body.body === undefined) {
    return new Response(null, { status: 304, headers });
  }
  return new Response(req.method === "HEAD" ? null : body.body, { status: 200, headers });
}
