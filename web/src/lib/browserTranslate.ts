// Client-only bridge to the browser's built-in Translator API for the WASM CLI.
//
// The Go-wasm "browser" MT provider (kapi/cmd/kapi-wasm-cli/browser_mt.go) has no
// native translator, so — exactly like the "intl" segmentation engine and the
// "gemma" AI provider — it calls out via syscall/js to a host-provided global:
//
//   globalThis.kapiBrowserTranslate(payloadJSON) =>
//     Promise<{ ok: boolean, translation?: string, error?: string }>
//
// payloadJSON is {text, sourceLang, targetLang}. It uses the browser's built-in,
// on-device Translator API (Chrome 138+, desktop only — the browser manages and
// downloads its own model, nothing comes from us). Where the API is missing the
// bridge returns { ok:false } so a page degrades visibly (the demo provider is
// the fallback). Installing the bridge is synchronous and free — it just defines
// a global; the model only loads when an actual translation runs.

// Minimal structural type for the Translator API (no DOM lib types ship for it).
interface BrowserTranslator {
  translate(text: string): Promise<string>;
}
interface TranslatorStatic {
  availability(opts: {
    sourceLanguage: string;
    targetLanguage: string;
  }): Promise<"available" | "downloadable" | "downloading" | "unavailable">;
  create(opts: { sourceLanguage: string; targetLanguage: string }): Promise<BrowserTranslator>;
}

function translatorAPI(): TranslatorStatic | undefined {
  return (globalThis as unknown as { Translator?: TranslatorStatic }).Translator;
}

/** Whether the platform provides the built-in Translator API at all. */
export function browserTranslatorSupported(): boolean {
  return typeof window !== "undefined" && translatorAPI() !== undefined;
}

interface TranslateResult {
  ok: boolean;
  translation?: string;
  error?: string;
}

// One translator instance per language pair (creating one can download a model).
const pairCache = new Map<string, Promise<BrowserTranslator>>();

function baseLang(code: string): string {
  // The Translator API wants a base language tag ("fr", not "fr-FR").
  const lower = (code || "").toLowerCase();
  const cut = lower.search(/[-_]/);
  return cut >= 0 ? lower.slice(0, cut) : lower;
}

async function getTranslator(
  sourceLanguage: string,
  targetLanguage: string,
): Promise<BrowserTranslator> {
  const key = `${sourceLanguage}->${targetLanguage}`;
  let p = pairCache.get(key);
  if (!p) {
    p = (async () => {
      const T = translatorAPI();
      if (!T) throw new Error("Translator API unavailable");
      const avail = await T.availability({ sourceLanguage, targetLanguage });
      if (avail === "unavailable") {
        throw new Error(`no on-device model for ${sourceLanguage} → ${targetLanguage}`);
      }
      return T.create({ sourceLanguage, targetLanguage });
    })().catch((e) => {
      pairCache.delete(key); // allow a retry after a transient failure
      throw e;
    });
    pairCache.set(key, p);
  }
  return p;
}

/**
 * installBrowserTranslateBridge installs globalThis.kapiBrowserTranslate so the
 * in-wasm "browser" MT provider can translate via the platform Translator API.
 * Idempotent, synchronous, no-op on the server, and free (no model load until a
 * translation actually runs). Returns whether the API is available.
 */
export function installBrowserTranslateBridge(): boolean {
  if (typeof window === "undefined") return false;
  (globalThis as Record<string, unknown>).kapiBrowserTranslate = async (
    payloadJSON: string,
  ): Promise<TranslateResult> => {
    try {
      if (!browserTranslatorSupported()) {
        return { ok: false, error: "Translator API not available (needs desktop Chrome 138+)" };
      }
      const { text, sourceLang, targetLang } = JSON.parse(payloadJSON) as {
        text: string;
        sourceLang?: string;
        targetLang?: string;
      };
      const tgt = baseLang(targetLang ?? "");
      if (!tgt) return { ok: false, error: "no target language" };
      const src = baseLang(sourceLang ?? "") || "en";
      if (src === tgt) return { ok: true, translation: text };
      const t = await getTranslator(src, tgt);
      return { ok: true, translation: await t.translate(text ?? "") };
    } catch (e) {
      return { ok: false, error: e instanceof Error ? e.message : String(e) };
    }
  };
  return browserTranslatorSupported();
}

/** Whether the bridge global is installed. */
export function browserTranslateReady(): boolean {
  return typeof (globalThis as Record<string, unknown>).kapiBrowserTranslate === "function";
}
