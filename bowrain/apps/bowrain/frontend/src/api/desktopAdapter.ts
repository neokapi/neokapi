import type { ApiAdapter } from "@neokapi/ui";
import { RestApiAdapter, type ApiTransport } from "@neokapi/ui";
import { WailsApiAdapter } from "./WailsApiAdapter";
import { Backend } from "./backend";

/**
 * The desktop's composite ApiAdapter.
 *
 * The base is the shared RestApiAdapter, wired to a Wails ProxyRequest transport
 * so every server surface reaches the connected bowrain server (keychain auth,
 * Go-side, no webview-origin/CORS concern). Over that base, the LOCAL_FIRST
 * methods are served by the desktop's Wails bindings instead — the offline-
 * capable content surfaces (editor blocks, content memory/terms mirrors), native
 * file-path uploads, keychain-local provider config, and the governance/
 * knowledge surfaces the Go backend already proxies with workspace scoping.
 *
 * This converts the ~150 methods the WailsApiAdapter used to stub into real
 * server calls in one move, while keeping the working-copy surfaces local.
 * `getConfig` and `listProjects` deliberately fall through to REST: config so
 * the shared router runs in real server mode (real user + workspaces, not the
 * old single-user hardcodes), and project listing so it is workspace-correct by
 * URL rather than tied to the backend's single selected workspace.
 *
 * `getProject` is REST-first too (see the explicit override below): the server
 * project carries the collections, streams, and item/label model the web renders,
 * so the desktop project overview matches the web instead of the flat local-file
 * view. Items resolve by name (both sides generate item IDs independently), so
 * the editor's local-first `getFileBlocks` still opens them. When the server is
 * unreachable it falls back to the local working copy — normalized to the shared
 * shape — so an offline desktop still renders a cached project rather than an
 * empty view.
 *
 * `createProject` and `deleteProject` also fall through to REST (#1283): the
 * desktop is a working copy, never a source of truth, so it must never mint a
 * server-unknown project (which the now-server-sourced `getProject`/`listProjects`
 * would then fail to show or open). A server mutation while offline fails loud
 * via the connectivity error rather than queueing a local-only project that would
 * 404 later; the Go create/close bindings stay for other callers.
 */
const LOCAL_FIRST: ReadonlySet<string> = new Set<string>([
  "listMembers",
  "addMember",
  "updateMemberRole",
  "removeMember",
  "listInvites",
  "createInvite",
  "deleteInvite",
  "uploadFiles",
  "removeFile",
  "listConnectors",
  "addConnector",
  "removeConnector",
  "getConnectorStatus",
  "getConnectorStatuses",
  "fetchConnector",
  "publishConnector",
  "listConnectorContent",
  "getFileBlocks",
  "getBlockCounts",
  "getBlock",
  "getItem",
  "bulkReviewBlocks",
  "bulkApplyMemory",
  "updateBlockTarget",
  "updateBlockTargetCoded",
  "pseudoTranslateFile",
  "memoryTranslateFile",
  "getWordCount",
  "exportTranslatedFile",
  "lookupMemoryForBlock",
  "lookupTermsForBlock",
  "getMemoryEntries",
  "getMemoryCount",
  "addMemoryEntry",
  "updateMemoryEntry",
  "deleteMemoryEntry",
  "bulkDeleteMemoryEntries",
  "getTerms",
  "getTermCount",
  "addConcept",
  "updateConcept",
  "deleteConcept",
  "bulkDeleteConcepts",
  "importTermsCSV",
  "importTermsJSON",
  "exportTermsJSON",
  "listProviderConfigs",
  "saveProviderConfig",
  "deleteProviderConfig",
  "testProviderConfig",
  "getKnownLocales",
  "listFormats",
  "listTools",
  "getToolSchema",
  "setBlockStatus",
  "renderDocumentPreview",
  "renderBlockHTML",
  "listFlowDefinitions",
  "getFlowDefinition",
  "createFlowDefinition",
  "updateFlowDefinition",
  "deleteFlowDefinition",
  "listVoiceProfiles",
  "getVoiceProfile",
  "getVoiceScores",
  "getVoiceTrends",
  "listVoiceCandidates",
  "promoteVoiceRule",
  "rejectVoiceRule",
  "evaluateVoiceRule",
  "getVoiceDrift",
  "listConcepts",
  "getConceptStatusCounts",
  "getConceptLocaleCoverage",
  "getConcept",
  "createConcept",
  "getConceptStory",
  "listConceptRelations",
  "addConceptRelation",
  "deleteConceptRelation",
  "getConceptBlastRadius",
  "listObservations",
  "addObservation",
  "deleteObservation",
  "listConceptComments",
  "addConceptComment",
  "resolveConceptComment",
  "deleteConceptComment",
  "listMarkets",
  "createMarket",
  "updateMarket",
  "deleteMarket",
  "listChangesets",
  "getChangesetCounts",
  "getChangeset",
  "createChangeset",
  "patchChangeset",
  "appendChangesetOp",
  "removeChangesetOp",
  "submitChangeset",
  "approveChangeset",
  "rejectChangeset",
  "mergeChangeset",
  "abandonChangeset",
  "getChangesetBlastRadius",
  "refreshChangesetBlastRadius",
  "addPilot",
  "removePilot",
]);

/** A raw {status, body} pair as returned by the Go proxy bindings. */
type ProxyResult = { status: number; body: string };

/**
 * Thrown by the proxy transport when a `Backend.ProxyRequest` /
 * `Backend.ProxyMultipart` binding call itself rejects — i.e. the Go side could
 * not reach or authenticate the server (no connection, dead token, or transport
 * failure). A reachable server that returns an HTTP error status comes back as a
 * ProxyResult (status + body), NOT a rejection, so it never becomes this error.
 * `getProject` uses this to distinguish "offline → fall back to the local
 * working copy" from "server said 404 → propagate".
 */
export class ProxyConnectivityError extends Error {
  constructor(cause: unknown) {
    super(cause instanceof Error ? cause.message : String(cause));
    this.name = "ProxyConnectivityError";
    // Preserve the original rejection for error reporting (parseAppError reads
    // `.cause`). Assigned directly rather than via the Error options bag, which
    // this lib target's constructor typing doesn't accept.
    (this as { cause?: unknown }).cause = cause;
  }
}

/**
 * Await a proxy binding call, converting a rejection (never an HTTP status —
 * that returns a ProxyResult) into a typed {@link ProxyConnectivityError}.
 */
async function callProxyBinding(call: Promise<unknown>): Promise<ProxyResult> {
  try {
    return (await call) as ProxyResult;
  } catch (err) {
    throw new ProxyConnectivityError(err);
  }
}

/** 204/205/304 must not carry a body (the Response constructor throws otherwise). */
function proxyResponse(res: ProxyResult): Response {
  const noBody = res.status === 204 || res.status === 205 || res.status === 304;
  return new Response(noBody ? null : res.body, { status: res.status });
}

/** Base64-encode a Blob's bytes (the Wails bridge carries strings, not bytes). */
function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      // readAsDataURL yields "data:<type>;base64,<payload>" — keep the payload.
      const s = reader.result as string;
      const comma = s.indexOf(",");
      resolve(comma >= 0 ? s.slice(comma + 1) : s);
    };
    reader.onerror = () => reject(reader.error ?? new Error("file read failed"));
    reader.readAsDataURL(blob);
  });
}

/** Serialize a FormData and forward it through the Go ProxyMultipart binding. */
async function proxyMultipart(method: string, path: string, form: FormData): Promise<Response> {
  const fields: Record<string, string> = {};
  const files: { field: string; filename: string; contentType: string; dataB64: string }[] = [];
  for (const [key, value] of form.entries()) {
    if (typeof value === "string") {
      fields[key] = value;
    } else {
      files.push({
        field: key,
        filename: value.name || "file",
        contentType: value.type || "application/octet-stream",
        dataB64: await blobToBase64(value),
      });
    }
  }
  const payload = JSON.stringify({ fields, files });
  const res = await callProxyBinding(Backend.ProxyMultipart(method, path, payload));
  return proxyResponse(res);
}

/**
 * fetch-compatible transport that forwards to the connected server through the
 * Go proxy bindings. The composite constructs RestApiAdapter with an empty
 * baseUrl, so `input` is the request path (e.g. "/api/v1/acme/projects"). A
 * string body goes through ProxyRequest; a FormData (multipart upload) goes
 * through ProxyMultipart.
 */
const proxyTransport: ApiTransport = async (input, init) => {
  const method = (init?.method ?? "GET").toUpperCase();
  const rawBody = init?.body;
  if (rawBody instanceof FormData) {
    return proxyMultipart(method, input, rawBody);
  }
  const body = typeof rawBody === "string" ? rawBody : "";
  const res = await callProxyBinding(Backend.ProxyRequest(method, input, body));
  return proxyResponse(res);
};

/**
 * Build the desktop composite adapter. A Proxy routes LOCAL_FIRST methods to the
 * Wails bindings and everything else to the REST base — callers see a plain
 * ApiAdapter.
 */
export function createDesktopAdapter(): ApiAdapter {
  const wails = new WailsApiAdapter();
  const rest = new RestApiAdapter("", null, proxyTransport);

  // A 401 the adapter can't refresh means the keychain token is dead. The web
  // default here redirects to /api/v1/auth/login (a cookie/OIDC browser flow
  // that is meaningless in the webview). Instead, clear the bad token and
  // disconnect: Backend.Logout fires connection-state-changed, and App.tsx's
  // gate demotes to ServerConnect for a fresh sign-in — never the web redirect.
  rest.onSessionExpired = () => {
    void Backend.Logout();
  };

  // getProject: REST-first (server view — collections/streams/items), falling
  // back to the local working copy only when the server is unreachable. A
  // ProxyConnectivityError means the transport couldn't reach/authenticate the
  // server, so the cached working copy is the best available view; any other
  // error (an HTTP status from a reachable server, e.g. 404) is authoritative
  // and propagates.
  const getProjectServerFirst = async (
    ...args: Parameters<ApiAdapter["getProject"]>
  ): ReturnType<ApiAdapter["getProject"]> => {
    try {
      return await rest.getProject(...args);
    } catch (err) {
      if (err instanceof ProxyConnectivityError) {
        const [ws, projectId] = args;
        const local = await wails.getProject(ws, projectId);
        // The local working-copy shape omits the server-only projection
        // (collections/streams/active_stream). Default them explicitly so every
        // shared consumer renders the offline flat view rather than risking a
        // crash on a missing array, regardless of how it reads the project.
        return {
          ...local,
          items: local.items ?? [],
          collections: local.collections ?? [],
          streams: local.streams ?? [],
          active_stream: local.active_stream ?? "main",
        };
      }
      throw err;
    }
  };

  return new Proxy(rest, {
    get(target, prop, receiver) {
      if (prop === "getProject") return getProjectServerFirst;
      if (typeof prop === "string" && LOCAL_FIRST.has(prop)) {
        const fn = (wails as unknown as Record<string, unknown>)[prop];
        if (typeof fn === "function") {
          return (fn as (...args: unknown[]) => unknown).bind(wails);
        }
      }
      return Reflect.get(target, prop, receiver);
    },
  }) as ApiAdapter;
}
