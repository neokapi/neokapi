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
 * capable content surfaces (editor blocks, TM/termbase mirrors), native
 * file-path uploads, keychain-local provider config, and the governance/
 * knowledge surfaces the Go backend already proxies with workspace scoping.
 *
 * This converts the ~150 methods the WailsApiAdapter used to stub into real
 * server calls in one move, while keeping the working-copy surfaces local.
 * `getConfig` and `listProjects` deliberately fall through to REST: config so
 * the shared router runs in real server mode (real user + workspaces, not the
 * old single-user hardcodes), and project listing so it is workspace-correct by
 * URL rather than tied to the backend's single selected workspace.
 */
const LOCAL_FIRST: ReadonlySet<string> = new Set<string>([
  "listMembers",
  "addMember",
  "updateMemberRole",
  "removeMember",
  "listInvites",
  "createInvite",
  "deleteInvite",
  "createProject",
  "getProject",
  "deleteProject",
  "uploadFiles",
  "removeFile",
  "listConnectors",
  "addConnector",
  "removeConnector",
  "getConnectorStatus",
  "fetchConnector",
  "publishConnector",
  "listConnectorContent",
  "getFileBlocks",
  "updateBlockTarget",
  "updateBlockTargetCoded",
  "pseudoTranslateFile",
  "tmTranslateFile",
  "getWordCount",
  "exportTranslatedFile",
  "lookupTMForBlock",
  "lookupTermsForBlock",
  "getTMEntries",
  "getTMCount",
  "addTMEntry",
  "updateTMEntry",
  "deleteTMEntry",
  "getTerms",
  "getTermCount",
  "addConcept",
  "updateConcept",
  "deleteConcept",
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
  "setBlockStatus",
  "renderDocumentPreview",
  "renderBlockHTML",
  "listFlowDefinitions",
  "getFlowDefinition",
  "createFlowDefinition",
  "updateFlowDefinition",
  "deleteFlowDefinition",
  "listBrandProfiles",
  "getBrandProfile",
  "getBrandScores",
  "getBrandTrends",
  "listStarterPacks",
  "listBrandCandidates",
  "promoteBrandRule",
  "rejectBrandRule",
  "evaluateBrandRule",
  "getBrandDrift",
  "listConcepts",
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
  "addPilot",
  "removePilot",
]);

/** A raw {status, body} pair as returned by the Go proxy bindings. */
type ProxyResult = { status: number; body: string };

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
  const res = (await Backend.ProxyMultipart(method, path, payload)) as ProxyResult;
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
  const res = (await Backend.ProxyRequest(method, input, body)) as ProxyResult;
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

  return new Proxy(rest, {
    get(target, prop, receiver) {
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
