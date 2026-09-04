import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import type { Page } from "@playwright/test";

/**
 * Mock @wailsio/runtime module served as an ES module.
 * Delegates Call.ByID to window.__wailsMock handlers and provides
 * Create helpers used by the generated model classes.
 */
const MOCK_RUNTIME_MODULE = `
export const Call = {
  ByID(id, ...args) {
    const handler = window.__wailsMock?.[id];
    if (handler) {
      try {
        const result = handler(...args);
        if (result && typeof result.then === 'function') {
          return result;
        }
        return Promise.resolve(result);
      } catch (e) {
        return Promise.reject(e);
      }
    }
    console.warn('[mock] Unmocked Wails call ID:', id, args);
    return Promise.resolve(null);
  }
};

export const Create = {
  Any(val) { return val; },
  Nullable(fn) {
    return function(val) {
      if (val === null || val === undefined) return null;
      return typeof fn === 'function' ? fn(val) : val;
    };
  },
  Array(fn) {
    return function(arr) {
      if (!Array.isArray(arr)) return [];
      return typeof fn === 'function' ? arr.map(fn) : arr;
    };
  },
  Map(keyFn, valueFn) {
    return function(map) {
      if (!map || typeof map !== 'object') return {};
      const result = {};
      for (const [k, v] of Object.entries(map)) {
        result[typeof keyFn === 'function' ? keyFn(k) : k] = typeof valueFn === 'function' ? valueFn(v) : v;
      }
      return result;
    };
  }
};

export class CancellablePromise extends Promise {
  cancel() {}
}

export const Browser = {
  OpenURL(url) {
    console.log('[mock] Browser.OpenURL:', url);
  }
};

export const Events = {
  On(eventName, callback) {
    // Store listeners so tests can emit events if needed
    if (!window.__wailsEventListeners) window.__wailsEventListeners = {};
    if (!window.__wailsEventListeners[eventName]) window.__wailsEventListeners[eventName] = [];
    window.__wailsEventListeners[eventName].push(callback);
    // Return cancel function
    return function() {
      const arr = window.__wailsEventListeners?.[eventName];
      if (arr) {
        const idx = arr.indexOf(callback);
        if (idx >= 0) arr.splice(idx, 1);
      }
    };
  },
  Once(eventName, callback) {
    const cancel = Events.On(eventName, function(...args) {
      cancel();
      callback(...args);
    });
    return cancel;
  },
  Emit(event) {
    // no-op in mock
  },
  Off(eventName) {
    if (window.__wailsEventListeners) {
      delete window.__wailsEventListeners[eventName];
    }
  },
  OffAll() {
    window.__wailsEventListeners = {};
  }
};
`;

/**
 * The generated Wails bindings, as `name -> Call.ByID` number.
 *
 * These used to be a hand-copied table with a comment telling you which grep to
 * re-run. Nobody re-ran it. By the time this was written eight of the ninety-odd
 * ids had drifted — every content-memory binding among them — and six named
 * bindings no longer existed at all. A stale id is silent: `Call.ByID` finds no
 * handler, warns into a log nobody reads, and resolves `null`, so the mock goes
 * on answering "nothing" to calls it believes it is serving.
 *
 * The bindings file is committed, so it can simply be read. It is the same file
 * the app imports, which makes it the only copy.
 */
function wailsCallIds(): Record<string, number> {
  const bindings = fileURLToPath(
    new URL(
      "../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js",
      import.meta.url,
    ),
  );
  const source = readFileSync(bindings, "utf8");
  const ids: Record<string, number> = {};
  for (const [, name, id] of source.matchAll(
    /export function (\w+)\([^)]*\)\s*\{\s*return \$Call\.ByID\((\d+)/g,
  )) {
    ids[name] = Number(id);
  }
  if (Object.keys(ids).length === 0) {
    throw new Error(`[mock] no Call.ByID bindings found in ${bindings} — did their shape change?`);
  }
  return ids;
}

/**
 * Injects a mock Wails backend into the page.
 * Intercepts @wailsio/runtime with a mock module that delegates
 * Call.ByID to window.__wailsMock, then populates mock handlers
 * via addInitScript.
 */
export async function injectMockBackend(page: Page) {
  // Intercept the Wails runtime module and serve our mock
  await page.route("**/node_modules/.vite/deps/@wailsio*", (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: MOCK_RUNTIME_MODULE,
    });
  });

  // Also intercept any direct @wailsio/runtime imports (in case Vite resolves differently)
  await page.route("**/@wailsio/runtime*", (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: MOCK_RUNTIME_MODULE,
    });
  });

  // Register mock backend handlers keyed by Wails v3 binding call IDs
  await page.addInitScript((callIds: Record<string, number>) => {
    let projectCounter = 0;
    let providerCounter = 0;
    const projects: Record<string, any> = {};
    const projectFiles: Record<string, Record<string, any>> = {};
    const providerConfigs: Record<string, any> = {};

    // Per-locale target entries are either a bare string (legacy payloads) or
    // a {text, status} object carrying the per-locale Target.Status (mirrors
    // the server's BlockTargetInfo). All mock reads go through this helper so
    // both shapes work — the same rule as the frontend's getTargetText.
    const targetText = (block: any, locale: string): string => {
      const entry = block?.targets?.[locale];
      if (entry == null) return "";
      return typeof entry === "string" ? entry : (entry.text ?? "");
    };
    // Replace a target's text while preserving its per-locale status, the way
    // the server's SetTargetText/SetTargetRuns preserve Target.Status.
    const setTargetPreservingStatus = (block: any, locale: string, text: string) => {
      const entry = block.targets[locale];
      const status = entry != null && typeof entry === "object" ? entry.status : undefined;
      block.targets[locale] = status ? { text, status } : text;
    };

    // Every id comes from the committed bindings (see wailsCallIds). The proxy
    // is what stops a rename from going quiet: reading a name the bindings no
    // longer export throws here, at setup, naming it — rather than registering a
    // handler under `undefined` that no call will ever reach.
    const IDS: Record<string, number> = new Proxy(callIds, {
      get(target, name: string) {
        const id = target[name];
        if (id === undefined) {
          throw new Error(
            `[mock] the Wails bindings export no "${String(name)}" — it was renamed or removed. ` +
              `Update the handler in e2e/mock-backend.ts.`,
          );
        }
        return id;
      },
    });

    // Global content memory storage (workspace-scoped, not per-project)
    const memoryStore: Record<string, any> = {};
    let memoryEntryCounter = 0;

    // Global terminology storage (workspace-scoped, not per-project)
    const termsStore: Record<string, any> = {};
    let conceptCounter = 0;

    const mock: Record<number, (...args: any[]) => any> = {};

    mock[IDS.ListFormats] = () => [
      { name: "html", has_reader: true, has_writer: true },
      { name: "json", has_reader: true, has_writer: true },
      { name: "plaintext", has_reader: true, has_writer: true },
      { name: "xml", has_reader: true, has_writer: true },
      { name: "yaml", has_reader: true, has_writer: true },
    ];

    mock[IDS.ListTools] = () => [
      { name: "translate", description: "Translate content using AI" },
      { name: "pseudo-translate", description: "Generate pseudo-translations" },
      { name: "word-count", description: "Count words" },
    ];

    // --- Flow definition storage ---
    // The shapes the server returns: a flow is tool nodes only (it owns no
    // I/O), chained by its edges and laid out along one row.
    const builtInFlowDefs: Record<string, any> = {
      translate: {
        id: "translate",
        name: "AI Translate",
        description: "Translate content using AI/LLM",
        source: "built-in",
        nodes: [
          {
            id: "translate",
            type: "tool",
            name: "translate",
            label: "AI Translate",
            position: { x: 0, y: 100 },
          },
          {
            id: "word-count",
            type: "tool",
            name: "word-count",
            label: "Word Count",
            position: { x: 250, y: 100 },
          },
        ],
        edges: [{ id: "e-translate-word-count", source: "translate", target: "word-count" }],
      },
      "pseudo-translate": {
        id: "pseudo-translate",
        name: "Pseudo Translate",
        description: "Generate pseudo-translations for testing",
        source: "built-in",
        nodes: [
          {
            id: "pseudo-translate",
            type: "tool",
            name: "pseudo-translate",
            label: "Pseudo Translate",
            position: { x: 0, y: 100 },
          },
        ],
        edges: [],
      },
    };
    const userFlowDefs: Record<string, any> = {};
    let flowDefCounter = 0;

    mock[IDS.ListFlowDefinitions] = () => [
      ...Object.values(builtInFlowDefs),
      ...Object.values(userFlowDefs),
    ];

    // The flow bindings are project-scoped: every call carries the project id
    // first, as the generated bindings do.
    mock[IDS.GetFlowDefinition] = (_projectId: string, id: string) => {
      if (builtInFlowDefs[id]) return builtInFlowDefs[id];
      if (userFlowDefs[id]) return userFlowDefs[id];
      throw new Error(`Flow ${id} not found`);
    };

    // Like the server: a flow without an id is created under a fresh one, and
    // every stored flow is a project flow.
    mock[IDS.SaveFlowDefinition] = (_projectId: string, def: any) => {
      if (def.source === "built-in") throw new Error("Cannot modify built-in flows");
      if (!def.id) def.id = `flow-${++flowDefCounter}`;
      if (builtInFlowDefs[def.id]) throw new Error("Cannot modify built-in flows");
      def.source = "project";
      def.modified_at = new Date().toISOString();
      if (!def.created_at) def.created_at = def.modified_at;
      userFlowDefs[def.id] = { ...def };
      return { ...def };
    };

    mock[IDS.DeleteFlowDefinition] = (_projectId: string, id: string) => {
      if (builtInFlowDefs[id]) throw new Error(`Cannot delete built-in flow ${id}`);
      if (!userFlowDefs[id]) throw new Error(`Flow ${id} not found`);
      delete userFlowDefs[id];
    };

    mock[IDS.ListPlugins] = () => [];
    mock[IDS.PluginDir] = () => "~/.kapi/plugins";
    mock[IDS.SetApplication] = () => {};

    // The plugin-catalogue bindings (install/search/update) were removed from
    // the backend; their stubs went with them. The IDS proxy is what found
    // them — reading a name the bindings no longer export throws at setup.
    mock[IDS.ConfigureConnector] = () => null;
    mock[IDS.CreateStoreVersion] = () => null;
    mock[IDS.DetectFormat] = (filePath: string) => {
      const ext = filePath.split(".").pop() || "";
      const formatMap: Record<string, string> = {
        html: "html",
        htm: "html",
        json: "json",
        txt: "plaintext",
        xml: "xml",
        yaml: "yaml",
        yml: "yaml",
        po: "po",
        properties: "properties",
        md: "markdown",
      };
      return formatMap[ext] || "plaintext";
    };
    mock[IDS.GetCurrentWorkspace] = () => ({
      id: "personal",
      name: "Personal",
      type: "local",
    });
    mock[IDS.GetConnectorStatus] = () => null;
    mock[IDS.GetVersion] = () => ({
      version: "0.0.0-mock",
      commit: "mock",
      date: new Date().toISOString(),
    });
    mock[IDS.InitContentStore] = () => {};
    mock[IDS.ListConnectorTypes] = () => [];
    mock[IDS.ListConnectors] = () => [];
    mock[IDS.ListContentItems] = () => [];
    mock[IDS.ListStoreProjects] = () => [];
    mock[IDS.ListStoreVersions] = () => [];
    mock[IDS.ListWorkspaces] = () => [{ id: "personal", name: "Personal", type: "local" }];
    mock[IDS.LoadPlugins] = () => {};
    mock[IDS.FetchContent] = () => [];
    mock[IDS.PublishContent] = () => {};
    mock[IDS.RemoveConnector] = () => {};
    mock[IDS.StoreProject] = () => null;

    // Locale handlers
    const knownLocales = [
      { code: "af", display_name: "Afrikaans" },
      { code: "ar", display_name: "Arabic" },
      { code: "bg", display_name: "Bulgarian" },
      { code: "bn", display_name: "Bengali" },
      { code: "pt-BR", display_name: "Brazilian Portuguese" },
      { code: "ca", display_name: "Catalan" },
      { code: "cs", display_name: "Czech" },
      { code: "da", display_name: "Danish" },
      { code: "nl", display_name: "Dutch" },
      { code: "en", display_name: "English" },
      { code: "et", display_name: "Estonian" },
      { code: "fi", display_name: "Finnish" },
      { code: "fr", display_name: "French" },
      { code: "de", display_name: "German" },
      { code: "el", display_name: "Greek" },
      { code: "gu", display_name: "Gujarati" },
      { code: "he", display_name: "Hebrew" },
      { code: "hi", display_name: "Hindi" },
      { code: "hr", display_name: "Croatian" },
      { code: "hu", display_name: "Hungarian" },
      { code: "id", display_name: "Indonesian" },
      { code: "it", display_name: "Italian" },
      { code: "ja", display_name: "Japanese" },
      { code: "kn", display_name: "Kannada" },
      { code: "ko", display_name: "Korean" },
      { code: "lt", display_name: "Lithuanian" },
      { code: "lv", display_name: "Latvian" },
      { code: "ml", display_name: "Malayalam" },
      { code: "mr", display_name: "Marathi" },
      { code: "ms", display_name: "Malay" },
      { code: "nb", display_name: "Norwegian Bokm\u00e5l" },
      { code: "fa", display_name: "Persian" },
      { code: "pl", display_name: "Polish" },
      { code: "pt", display_name: "Portuguese" },
      { code: "ro", display_name: "Romanian" },
      { code: "ru", display_name: "Russian" },
      { code: "sr", display_name: "Serbian" },
      { code: "zh-Hans", display_name: "Simplified Chinese" },
      { code: "sk", display_name: "Slovak" },
      { code: "sl", display_name: "Slovenian" },
      { code: "es", display_name: "Spanish" },
      { code: "sw", display_name: "Swahili" },
      { code: "sv", display_name: "Swedish" },
      { code: "ta", display_name: "Tamil" },
      { code: "te", display_name: "Telugu" },
      { code: "th", display_name: "Thai" },
      { code: "zh-Hant", display_name: "Traditional Chinese" },
      { code: "tr", display_name: "Turkish" },
      { code: "uk", display_name: "Ukrainian" },
      { code: "ur", display_name: "Urdu" },
      { code: "vi", display_name: "Vietnamese" },
      { code: "zh", display_name: "Chinese" },
    ];

    mock[IDS.GetKnownLocales] = () => knownLocales;
    mock[IDS.GetLocaleDisplayName] = (code: string) => {
      const found = knownLocales.find((l: any) => l.code === code);
      return found ? found.display_name : code;
    };

    mock[IDS.CreateProject] = (name: string, sourceLang: string, targetLangs: string[]) => {
      const id = `project-${++projectCounter}`;
      const now = new Date().toISOString();
      const info = {
        id,
        name,
        default_source_language: sourceLang,
        target_languages: targetLangs || [],
        path: "",
        items: [],
        created_at: now,
        modified_at: now,
      };
      projects[id] = info;
      projectFiles[id] = {};
      return info;
    };

    mock[IDS.GetProject] = (projectID: string) => {
      const p = projects[projectID];
      if (!p) throw new Error(`Project ${projectID} not found`);
      return p;
    };

    mock[IDS.ListProjects] = () => Object.values(projects);

    mock[IDS.CloseProject] = (projectID: string) => {
      delete projects[projectID];
      delete projectFiles[projectID];
    };

    mock[IDS.AddItems] = (projectID: string, filePaths: string[]) => {
      const p = projects[projectID];
      if (!p) throw new Error(`Project ${projectID} not found`);

      for (const fp of filePaths) {
        const name = fp.split("/").pop() || fp;
        const ext = name.split(".").pop() || "";
        const formatMap: Record<string, string> = {
          html: "html",
          htm: "html",
          json: "json",
          txt: "plaintext",
          xml: "xml",
          yaml: "yaml",
          yml: "yaml",
          po: "po",
          properties: "properties",
          md: "markdown",
        };
        const format = formatMap[ext] || "plaintext";

        const blocks = [
          {
            id: `${name}-block-1`,
            source: `Hello from ${name}`,
            targets: {},
            translatable: true,
            has_spans: false,
            properties: {},
          },
          {
            id: `${name}-block-2`,
            source: `Welcome to our application`,
            targets: {},
            translatable: true,
            has_spans: false,
            properties: {},
          },
          {
            id: `${name}-block-3`,
            source: `Click here to continue`,
            targets: {},
            translatable: true,
            has_spans: false,
            properties: {},
          },
        ];

        projectFiles[projectID] = projectFiles[projectID] || {};
        projectFiles[projectID][name] = blocks;

        p.items.push({
          name,
          format,
          type: "file",
          size: 1024,
          block_count: blocks.length,
          word_count: blocks.reduce((sum: number, b: any) => sum + b.source.split(/\s+/).length, 0),
        });
      }

      p.modified_at = new Date().toISOString();
      return { ...p };
    };

    mock[IDS.RemoveItem] = (projectID: string, itemName: string) => {
      const p = projects[projectID];
      if (!p) throw new Error(`Project ${projectID} not found`);
      p.items = p.items.filter((f: any) => f.name !== itemName);
      if (projectFiles[projectID]) delete projectFiles[projectID][itemName];
      p.modified_at = new Date().toISOString();
      return { ...p };
    };

    mock[IDS.ListProjectFiles] = (projectID: string) => {
      const p = projects[projectID];
      if (!p) throw new Error(`Project ${projectID} not found`);
      return p.items;
    };

    mock[IDS.GetItemBlocks] = (projectID: string, itemName: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) return [];
      return files[itemName].map((b: any) => ({
        ...b,
        targets: { ...b.targets },
        targets_coded: b.targets_coded ? { ...b.targets_coded } : undefined,
        source_spans: b.source_spans ? [...b.source_spans] : undefined,
      }));
    };

    mock[IDS.UpdateBlockTarget] = (req: any) => {
      const itemName = req.item_name || req.file_name;
      const files = projectFiles[req.project_id];
      if (!files || !files[itemName]) return;
      const block = files[itemName].find((b: any) => b.id === req.block_id);
      if (block) {
        setTargetPreservingStatus(block, req.target_locale, req.text);
      }
    };

    // The desktop adapter converts the editor's coded text + spans to an RFC
    // 0001 Run sequence (WailsApiAdapter.updateBlockTargetCoded \u2192
    // Backend.UpdateBlockTargetRuns), so the request carries `runs`, not
    // coded_text, so the handler binds IDS.UpdateBlockTargetRuns — the key the
    // adapter actually calls.
    mock[IDS.UpdateBlockTargetRuns] = (req: any) => {
      const itemName = req.item_name || req.file_name;
      const files = projectFiles[req.project_id];
      if (!files || !files[itemName]) return;
      const block = files[itemName].find((b: any) => b.id === req.block_id);
      if (!block) return;
      let plain = "";
      let coded = "";
      for (const run of req.runs ?? []) {
        if (run.text) {
          plain += run.text.text;
          coded += run.text.text;
        } else if (run.pcOpen) {
          coded += "\uE001";
        } else if (run.pcClose) {
          coded += "\uE002";
        } else if (run.ph) {
          coded += "\uE003";
        }
      }
      setTargetPreservingStatus(block, req.target_locale, plain);
      if (!block.targets_coded) block.targets_coded = {};
      block.targets_coded[req.target_locale] = coded;
    };

    mock[IDS.PseudoTranslateItem] = (projectID: string, itemName: string, targetLocale: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) throw new Error("Item not found");
      const blocks = files[itemName];
      let translated = 0;
      let wordCount = 0;
      for (const b of blocks) {
        if (b.translatable) {
          b.targets[targetLocale] = `[${b.source}]`;
          if (!b.properties) b.properties = {};
          // The retired block-global "translation-status" property is never
          // written; getBlockStatus derives "draft" from the origin.
          b.properties["translation-origin"] = "pseudo";
          translated++;
          wordCount += b.source.split(/\s+/).length;
        }
      }
      return { total_blocks: blocks.length, translated_blocks: translated, word_count: wordCount };
    };

    mock[IDS.MemoryTranslateItem] = (projectID: string, itemName: string, targetLocale: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) throw new Error("Item not found");
      const blocks = files[itemName];
      const entries = Object.values(memoryStore);
      let translated = 0;
      let wordCount = 0;
      for (const b of blocks) {
        if (!b.translatable || targetText(b, targetLocale)) continue;
        // Find exact or fuzzy match from content memory
        const exact = entries.find(
          (e: any) =>
            e.source.toLowerCase() === b.source.toLowerCase() && e.target_locale === targetLocale,
        );
        if (exact) {
          b.targets[targetLocale] = (exact as any).target;
          if (!b.properties) b.properties = {};
          b.properties["translation-origin"] = "tm";
          translated++;
          wordCount += b.source.split(/\s+/).length;
        }
      }
      return { total_blocks: blocks.length, translated_blocks: translated, word_count: wordCount };
    };

    mock[IDS.GetWordCount] = (projectID: string, itemName: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName])
        return { source_words: 0, source_chars: 0, target_words: {}, target_chars: {} };
      const blocks = files[itemName];
      let sourceWords = 0;
      let sourceChars = 0;
      const targetWords: Record<string, number> = {};
      const targetChars: Record<string, number> = {};
      for (const b of blocks) {
        if (b.translatable) {
          sourceWords += b.source.split(/\s+/).length;
          sourceChars += b.source.length;
          for (const locale of Object.keys(b.targets)) {
            const t = targetText(b, locale);
            targetWords[locale] = (targetWords[locale] || 0) + t.split(/\s+/).length;
            targetChars[locale] = (targetChars[locale] || 0) + t.length;
          }
        }
      }
      return {
        source_words: sourceWords,
        source_chars: sourceChars,
        target_words: targetWords,
        target_chars: targetChars,
      };
    };

    mock[IDS.ExportTranslatedItem] = (
      _projectID: string,
      itemName: string,
      targetLocale: string,
    ) => {
      const baseName = itemName.replace(/\.[^.]+$/, "");
      const ext = itemName.split(".").pop();
      return `/tmp/${baseName}_${targetLocale}.${ext}`;
    };

    mock[IDS.OpenFileInOS] = () => {};

    mock[IDS.RenderDocumentPreview] = (
      _projectID: string,
      itemName: string,
      _targetLocale: string,
    ) => {
      return `<!DOCTYPE html><html><head><style>
        kat-block { cursor: pointer; border-radius: 2px; display: inline; }
        kat-block:hover { background-color: rgba(59,130,246,0.15); }
        kat-block.kat-selected { background-color: rgba(59,130,246,0.25); outline: 2px solid #3b82f6; }
      </style></head><body>
        <p><kat-block id="${itemName}-block-1">Hello from ${itemName}</kat-block></p>
        <p><kat-block id="${itemName}-block-2">Welcome to our application</kat-block></p>
        <p><kat-block id="${itemName}-block-3">Click here to continue</kat-block></p>
      <script>
        document.querySelectorAll('kat-block').forEach(el => {
          el.addEventListener('click', () => {
            window.parent.postMessage({ type: 'kat-block-click', blockId: el.id }, '*');
          });
        });
        window.addEventListener('message', (e) => {
          if (e.data?.type === 'kat-select-block') {
            document.querySelector('.kat-selected')?.classList.remove('kat-selected');
            const el = document.getElementById(e.data.blockId);
            if (el) { el.classList.add('kat-selected'); }
          }
          if (e.data?.type === 'kat-update-block') {
            const el = document.getElementById(e.data.blockId);
            if (el) el.innerHTML = e.data.html;
          }
        });
        window.parent.postMessage({ type: 'kat-iframe-ready' }, '*');
      </script></body></html>`;
    };

    mock[IDS.RenderBlockHTML] = (
      projectID: string,
      itemName: string,
      blockID: string,
      targetLocale: string,
    ) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) return "";
      const block = files[itemName].find((b: any) => b.id === blockID);
      if (!block) return "";
      const rendered = targetLocale ? targetText(block, targetLocale) : "";
      if (rendered) {
        return rendered;
      }
      return block.source;
    };

    mock[IDS.ListProviderConfigs] = () => Object.values(providerConfigs);

    mock[IDS.SaveProviderConfig] = (cfg: any) => {
      const id = cfg.id || `provider-${++providerCounter}`;
      const saved = {
        id,
        name: cfg.name,
        provider_type: cfg.provider_type,
        model: cfg.model || "",
        base_url: cfg.base_url || "",
      };
      providerConfigs[id] = saved;
      return saved;
    };

    mock[IDS.DeleteProviderConfig] = (id: string) => {
      delete providerConfigs[id];
    };

    mock[IDS.TestProviderConfig] = () => {};

    // --- content memory mock handlers ---

    mock[IDS.GetMemoryEntries] = (
      _projectID: string,
      query: string,
      sourceLocale: string,
      targetLocale: string,
      offset: number,
      limit: number,
    ) => {
      let entries = Object.values(memoryStore);

      if (query) {
        const q = query.toLowerCase();
        entries = entries.filter(
          (e: any) => e.source.toLowerCase().includes(q) || e.target.toLowerCase().includes(q),
        );
      }
      if (sourceLocale) {
        entries = entries.filter((e: any) => e.source_locale === sourceLocale);
      }
      if (targetLocale) {
        entries = entries.filter((e: any) => e.target_locale === targetLocale);
      }

      const total = entries.length;
      const page = entries.slice(offset, offset + limit);
      return { entries: page, total_count: total };
    };

    mock[IDS.GetMemoryCount] = () => {
      return Object.keys(memoryStore).length;
    };

    mock[IDS.AddMemoryEntry] = (
      _projectID: string,
      source: string,
      target: string,
      sourceLocale: string,
      targetLocale: string,
    ) => {
      const id = `tm-entry-${++memoryEntryCounter}`;
      const entry = {
        id,
        source,
        target,
        source_locale: sourceLocale,
        target_locale: targetLocale,
        updated_at: new Date().toISOString(),
      };
      memoryStore[id] = entry;
      return entry;
    };

    mock[IDS.UpdateMemoryEntry] = (req: any) => {
      if (!memoryStore[req.entry_id]) throw new Error("content-memory entry not found");
      memoryStore[req.entry_id] = {
        ...memoryStore[req.entry_id],
        source: req.source,
        target: req.target,
        source_locale: req.source_locale,
        target_locale: req.target_locale,
        updated_at: new Date().toISOString(),
      };
    };

    mock[IDS.DeleteMemoryEntry] = (_projectID: string, entryID: string) => {
      if (!memoryStore[entryID]) throw new Error("content-memory entry not found");
      delete memoryStore[entryID];
    };

    // --- Context panel: per-block content memory and term lookup ---

    mock[IDS.LookupMemoryForBlock] = (
      projectID: string,
      itemName: string,
      blockID: string,
      targetLocale: string,
    ) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) return [];
      const block = files[itemName].find((b: any) => b.id === blockID);
      if (!block) return [];
      const entries = Object.values(memoryStore);
      const matches: any[] = [];
      for (const e of entries) {
        const entry = e as any;
        if (targetLocale && entry.target_locale !== targetLocale) continue;
        const srcLower = block.source.toLowerCase();
        const entryLower = entry.source.toLowerCase();
        if (srcLower === entryLower) {
          matches.push({
            source: entry.source,
            target: entry.target,
            score: 1.0,
            match_type: "exact",
          });
        } else if (srcLower.includes(entryLower) || entryLower.includes(srcLower)) {
          const longer = Math.max(srcLower.length, entryLower.length);
          const shorter = Math.min(srcLower.length, entryLower.length);
          const score = shorter / longer;
          if (score > 0.5) {
            matches.push({
              source: entry.source,
              target: entry.target,
              score,
              match_type: "fuzzy",
            });
          }
        }
      }
      matches.sort((a: any, b: any) => b.score - a.score);
      return matches;
    };

    mock[IDS.LookupTermsForBlock] = (
      projectID: string,
      itemName: string,
      blockID: string,
      targetLocale: string,
    ) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) return [];
      const block = files[itemName].find((b: any) => b.id === blockID);
      if (!block) return [];
      const concepts = Object.values(termsStore);
      const matches: any[] = [];
      const srcLower = block.source.toLowerCase();
      for (const c of concepts) {
        const concept = c as any;
        for (const t of concept.terms) {
          if (!t.text) continue;
          const termLower = t.text.toLowerCase();
          const idx = srcLower.indexOf(termLower);
          if (idx >= 0) {
            const targetTerms = concept.terms
              .filter(
                (tt: any) =>
                  tt.locale !== t.locale && (!targetLocale || tt.locale === targetLocale),
              )
              .map((tt: any) => tt.text);
            matches.push({
              source_term: t.text,
              target_terms: targetTerms,
              domain: concept.domain || "",
              status: t.status || "approved",
              start: idx,
              end: idx + t.text.length,
            });
            break; // One match per concept
          }
        }
      }
      return matches;
    };

    // --- Terminology mock handlers ---

    mock[IDS.GetTerms] = (
      _projectID: string,
      query: string,
      sourceLocale: string,
      targetLocale: string,
      offset: number,
      limit: number,
    ) => {
      let concepts = Object.values(termsStore);

      if (query) {
        const q = query.toLowerCase();
        concepts = concepts.filter(
          (c: any) =>
            c.terms.some((t: any) => t.text.toLowerCase().includes(q)) ||
            (c.domain && c.domain.toLowerCase().includes(q)) ||
            (c.definition && c.definition.toLowerCase().includes(q)),
        );
      }
      if (sourceLocale) {
        concepts = concepts.filter((c: any) => c.terms.some((t: any) => t.locale === sourceLocale));
      }
      if (targetLocale) {
        concepts = concepts.filter((c: any) => c.terms.some((t: any) => t.locale === targetLocale));
      }

      const total = concepts.length;
      const page = concepts.slice(offset, offset + limit);
      return { concepts: page, total_count: total };
    };

    mock[IDS.GetTermCount] = () => {
      return Object.keys(termsStore).length;
    };

    mock[IDS.AddConcept] = (req: any) => {
      const id = `concept-${++conceptCounter}`;
      const now = new Date().toISOString();
      const concept = {
        id,
        domain: req.domain || "",
        definition: req.definition || "",
        terms: (req.terms || []).map((t: any) => ({
          text: t.text || "",
          locale: t.locale || "",
          status: t.status || "approved",
          part_of_speech: t.part_of_speech || "",
          gender: t.gender || "",
          note: t.note || "",
        })),
        properties: {},
        created_at: now,
        updated_at: now,
      };
      termsStore[id] = concept;
      return concept;
    };

    mock[IDS.UpdateConcept] = (req: any) => {
      if (!termsStore[req.concept_id]) throw new Error("Concept not found");
      termsStore[req.concept_id] = {
        ...termsStore[req.concept_id],
        domain: req.domain || "",
        definition: req.definition || "",
        terms: (req.terms || []).map((t: any) => ({
          text: t.text || "",
          locale: t.locale || "",
          status: t.status || "approved",
          part_of_speech: t.part_of_speech || "",
          gender: t.gender || "",
          note: t.note || "",
        })),
        updated_at: new Date().toISOString(),
      };
    };

    mock[IDS.DeleteConcept] = (_projectID: string, conceptID: string) => {
      if (!termsStore[conceptID]) throw new Error("Concept not found");
      delete termsStore[conceptID];
    };

    mock[IDS.LookupTerms] = (
      _projectID: string,
      text: string,
      _sourceLocale: string,
      targetLocale: string,
    ) => {
      const concepts = Object.values(termsStore);
      const textLower = text.toLowerCase();
      const matches: any[] = [];
      for (const c of concepts) {
        const concept = c as any;
        for (const t of concept.terms) {
          if (!t.text) continue;
          const termLower = t.text.toLowerCase();
          const idx = textLower.indexOf(termLower);
          if (idx >= 0) {
            const targetTerms = concept.terms
              .filter(
                (tt: any) =>
                  tt.locale !== t.locale && (!targetLocale || tt.locale === targetLocale),
              )
              .map((tt: any) => ({ text: tt.text, locale: tt.locale, status: tt.status }));
            matches.push({
              source_term: t.text,
              concept_id: concept.id,
              domain: concept.domain,
              score: 1.0,
              match_type: "exact",
              status: t.status,
              target_terms: targetTerms,
              position: { start: idx, end: idx + t.text.length },
            });
            break;
          }
        }
      }
      return { matches };
    };

    mock[IDS.ImportTermsCSV] = (
      _projectID: string,
      content: string,
      sourceLocale: string,
      targetLocale: string,
      domain: string,
      hasHeader: boolean,
    ) => {
      const lines = content.split("\n").filter((l: string) => l.trim());
      const startIdx = hasHeader ? 1 : 0;
      let count = 0;
      for (let i = startIdx; i < lines.length; i++) {
        const parts = lines[i].split(",").map((s: string) => s.trim());
        if (parts.length >= 2 && parts[0] && parts[1]) {
          const id = `concept-${++conceptCounter}`;
          const now = new Date().toISOString();
          termsStore[id] = {
            id,
            domain: domain || "",
            definition: "",
            terms: [
              { text: parts[0], locale: sourceLocale, status: "preferred" },
              { text: parts[1], locale: targetLocale, status: "preferred" },
            ],
            created_at: now,
            updated_at: now,
          };
          count++;
        }
      }
      return count;
    };

    mock[IDS.ImportTermsJSON] = (_projectID: string, content: string) => {
      const data = JSON.parse(content);
      const concepts = data.concepts || data;
      let count = 0;
      for (const c of concepts) {
        const id = c.id || `concept-${++conceptCounter}`;
        const now = new Date().toISOString();
        termsStore[id] = {
          id,
          domain: c.domain || "",
          definition: c.definition || "",
          terms: c.terms || [],
          created_at: c.created_at || now,
          updated_at: now,
        };
        count++;
      }
      return count;
    };

    mock[IDS.ExportTermsJSON] = (_projectID: string, name: string) => {
      return JSON.stringify(
        {
          name,
          concepts: Object.values(termsStore),
        },
        null,
        2,
      );
    };

    mock[IDS.TermEnforceItem] = (projectID: string, itemName: string, targetLocale: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) return [];
      const concepts = Object.values(termsStore);
      const results: any[] = [];
      for (const b of files[itemName]) {
        if (!b.translatable || !targetText(b, targetLocale)) continue;
        const srcLower = b.source.toLowerCase();
        const tgtLower = targetText(b, targetLocale).toLowerCase();
        for (const c of concepts) {
          const concept = c as any;
          const srcTerms = concept.terms.filter((t: any) =>
            srcLower.includes(t.text.toLowerCase()),
          );
          for (const st of srcTerms) {
            const tgtTerms = concept.terms.filter((t: any) => t.locale !== st.locale);
            const found = tgtTerms.some((tt: any) => tgtLower.includes(tt.text.toLowerCase()));
            if (!found && tgtTerms.length > 0) {
              results.push({
                block_id: b.id,
                source_term: st.text,
                concept_id: concept.id,
                expected: tgtTerms.map((tt: any) => tt.text),
                source_text: b.source,
                target_text: targetText(b, targetLocale),
                source_locale: "en",
                target_locale: targetLocale,
              });
            }
          }
        }
      }
      return results;
    };

    // --- Connection mock handlers ---

    // Connection state: "disconnected" by default.
    // If __skipConnection flag is set (via addInitScript), GetConnectionState
    // returns a state that bypasses the ServerConnect screen:
    //   - "local" (default): offline state with workspace → non-server mode
    //   - "server": connected state with workspace → server mode
    let connectionState = "disconnected";
    let serverURL = "";
    let userName = "";
    let workspace = "";

    mock[IDS.GetConnectionState] = () => {
      // Lazy check: if __skipConnection was set (possibly by a later addInitScript),
      // upgrade the connection state on first call.
      const skipMode = (window as any).__skipConnection;
      if (skipMode && connectionState === "disconnected") {
        if (skipMode === "server") {
          connectionState = "connected";
          serverURL = "http://mock-server";
          userName = "Test User";
          workspace = "personal";
        } else {
          // Default: offline mode with cached workspace.
          // App.tsx treats "offline" + workspace as isServerMode=false if
          // no server URL, but actually "offline" + workspace → isServerMode=true.
          // The only way to get isServerMode=false is "disconnected" which
          // shows ServerConnect. So we use "connected" but the App.tsx logic
          // for workspace-less connected will set isServerMode=true.
          // For test compatibility, we use "connected" with workspace.
          connectionState = "connected";
          serverURL = "http://mock-server";
          userName = "Test User";
          workspace = "personal";
        }
      }
      return {
        state: connectionState,
        server_url: serverURL,
        user_name: userName,
        workspace: workspace,
      };
    };

    mock[IDS.GetDefaultServerURL] = () => "http://localhost:8080";

    mock[IDS.TryAutoConnect] = () => {
      // No stored auth in mock — stays disconnected.
    };

    mock[IDS.ConnectToServer] = (url: string) => {
      if (connectionState === "connected") {
        // Already authenticated (e.g. after PollLogin) — return success.
        serverURL = url;
        return { state: "connected", server_url: url, user_name: userName, workspace };
      }
      throw new Error("not authenticated: please log in first");
    };

    mock[IDS.StartLogin] = (url: string) => {
      // PKCE flow: opens browser, no return value needed.
      serverURL = url;
    };

    mock[IDS.WaitForLogin] = () => {
      // Simulate immediate PKCE auth success.
      connectionState = "connected";
      userName = "Test User";
      return true;
    };

    mock[IDS.CancelLogin] = () => {};

    mock[IDS.GetServerWorkspaces] = () => [
      {
        id: "ws-1",
        slug: "acme-corp",
        name: "Acme Corp",
        description: "Main workspace",
        role: "editor",
      },
      {
        id: "ws-2",
        slug: "personal",
        name: "Personal",
        description: "Personal workspace",
        role: "owner",
      },
    ];

    mock[IDS.SelectWorkspace] = (slug: string) => {
      workspace = slug;
    };

    mock[IDS.Disconnect] = () => {
      connectionState = "disconnected";
      serverURL = "";
      userName = "";
      workspace = "";
    };

    mock[IDS.Logout] = () => {
      connectionState = "disconnected";
      serverURL = "";
      userName = "";
      workspace = "";
    };

    mock[IDS.GetPendingChangesCount] = () => 0;

    mock[IDS.ReviewBlock] = (
      _projectID: string,
      itemName: string,
      blockID: string,
      targetLocale: string,
      reviewed: boolean,
      status?: string,
    ) => {
      const files = projectFiles[_projectID];
      if (!files || !files[itemName]) return;
      const block = files[itemName].find((b: any) => b.id === blockID);
      if (!block) return;
      // Mirror the server's HandleReviewBlock: review state is the per-locale
      // {text, status} target entry (the Target.Status ladder), never the
      // legacy block-global properties["translation-status"]; approving an
      // empty translation is rejected (the server 422s it).
      const text = targetText(block, targetLocale);
      if (reviewed && !text.trim()) {
        throw new Error(
          `block "${blockID}" has no ${targetLocale} translation to review: translate it first`,
        );
      }
      if (!reviewed && block.targets[targetLocale] == null) {
        // Un-reviewing a no-target locale clears a stuck legacy flag; no-op otherwise.
        if (block.properties) delete block.properties["translation-status"];
        return;
      }
      block.targets[targetLocale] = {
        text,
        // The optional status picks the rung within each direction: a sign-off
        // (reviewed=true + "signed-off") lands above reviewed, a rejection
        // (reviewed=false + "draft") demotes to draft, and either default
        // lands on reviewed or translated.
        status: reviewed
          ? status === "signed-off"
            ? "signed-off"
            : "reviewed"
          : status === "draft"
            ? "draft"
            : "translated",
      };
    };

    // ── REST transport ──────────────────────────────────────────────────
    //
    // The desktop app is not a Wails app with a REST fallback; it is a REST
    // client with a Wails fast path. desktopAdapter keeps a LOCAL_FIRST set —
    // the methods the working copy answers — and routes *everything else*
    // through Backend.ProxyRequest to the server. So mocking the bindings alone
    // mocks only half the app, and the half it leaves out contains the launch
    // probe: getConfig and getCurrentUser are both REST.
    //
    // That is how this suite came to be entirely red. #1775 moved the adapter
    // onto the proxy transport; nothing here learned about it; every spec died
    // in setupLocalApp on an offline-launch screen, and the job is nightly, so
    // the failure had no reader. The lesson is not "remember to update the
    // mock" — it is that an unmocked route must be impossible to merge past,
    // which is why this job is now path-gated onto the surfaces it covers (see
    // .github/workflows/e2e.yml) and why an unknown route below is a loud 501
    // naming itself rather than a quiet 404 that reads as "no data".
    //
    // Routes read and write the SAME state the binding handlers use. There is
    // no second copy of a project here: a project created through the Wails
    // CreateProject is visible to GET /api/v1/{ws}/projects because both are
    // looking at `projects`. A mock whose two transports disagree is worse than
    // no mock at all.
    const restJSON = (value: unknown) => ({ status: 200, body: JSON.stringify(value ?? null) });

    /** Locale rollups for a project, in the shape the dashboard reads. */
    const localeStatsFor = (project: any) =>
      (project?.target_languages ?? []).map((locale: string) => ({
        locale,
        translated_blocks: 0,
        total_blocks: 0,
        translated_words: 0,
        total_words: 0,
        percentage: 0,
      }));

    /**
     * `[method, path pattern, handler]`. The pattern's capture groups arrive as
     * `params`; `{ws}` is the workspace slug, which standalone mode reports as
     * "local". Add a route here when the adapter learns a new call — the 501
     * below will tell you the exact path and method to add.
     */
    const REST_ROUTES: Array<
      [string, RegExp, (params: string[], body: any, query: URLSearchParams) => unknown]
    > = [
      // Launch probe: reachability, then whether the stored session is valid.
      [
        "GET",
        /^\/api\/v1\/info$/,
        () => ({
          mode: "standalone",
          version: "e2e",
          commit: "e2e",
          build_date: "",
          features: { context_scan: false },
          provider_types: [],
        }),
      ],
      [
        "GET",
        /^\/api\/v1\/auth\/me$/,
        () => ({
          id: "user-1",
          email: "dev@example.com",
          name: "Dev User",
          avatar_url: "",
        }),
      ],
      [
        "GET",
        /^\/api\/v1\/workspaces$/,
        () => [{ id: "ws-local", slug: "local", name: "Local", description: "", role: "owner" }],
      ],

      // Projects — the same `projects` map the bindings mutate.
      ["GET", /^\/api\/v1\/[^/]+\/projects$/, () => Object.values(projects)],
      [
        "POST",
        /^\/api\/v1\/[^/]+\/projects$/,
        (_p, body) =>
          mock[IDS.CreateProject](
            body?.name,
            body?.source_language ?? body?.default_source_language ?? "en",
            body?.target_languages ?? [],
          ),
      ],
      ["GET", /^\/api\/v1\/[^/]+\/([^/]+)$/, ([id]) => projects[id] ?? null],
      [
        "DELETE",
        /^\/api\/v1\/[^/]+\/([^/]+)$/,
        ([id]) => {
          mock[IDS.CloseProject](id);
          return null;
        },
      ],

      // The translation dashboard the project overview opens on.
      [
        "GET",
        /^\/api\/v1\/[^/]+\/([^/]+)\/dashboard\/[^/]+$/,
        ([id]) => {
          const project = projects[id];
          const files = projectFiles[id] ?? {};
          const item_stats = Object.entries(files).map(([item_name, blocks]: [string, any]) => ({
            item_id: item_name,
            item_name,
            format: "json",
            collection_id: "",
            block_count: blocks.length,
            word_count: 0,
            locales: localeStatsFor(project),
          }));
          return {
            locale_stats: localeStatsFor(project),
            item_stats,
            item_total: item_stats.length,
            collection_stats: [],
            total_blocks: item_stats.reduce((n, i) => n + i.block_count, 0),
            translatable_blocks: 0,
            total_source_words: 0,
          };
        },
      ],

      // Workspace chrome the shell polls on every page.
      ["GET", /^\/api\/v1\/[^/]+\/activities$/, () => ({ activities: [], new_count: 0 })],
      ["POST", /^\/api\/v1\/[^/]+\/activities\/seen$/, () => null],
      ["GET", /^\/api\/v1\/[^/]+\/tasks\/counts$/, () => ({ total: 0, by_status: {} })],
      ["GET", /^\/api\/v1\/[^/]+\/tasks$/, () => ({ tasks: [], total: 0 })],
      ["GET", /^\/api\/v1\/[^/]+\/loop-rollup$/, () => ({})],
      ["GET", /^\/api\/v1\/[^/]+\/voice\/rollup$/, () => ({ projects: [] })],
      ["GET", /^\/api\/v1\/[^/]+\/changesets\/counts$/, () => ({ total: 0, by_status: {} })],
      ["GET", /^\/api\/v1\/[^/]+\/changesets$/, () => []],
      ["GET", /^\/api\/v1\/[^/]+\/connectors$/, () => []],
      // The project's Automations tab opens on its run history.
      ["GET", /^\/api\/v1\/[^/]+\/[^/]+\/automations\/runs$/, () => ({ runs: [] })],
    ];

    mock[IDS.ProxyRequest] = (method: string, path: string, body: string) => {
      const [route, rawQuery] = path.split("?");
      const query = new URLSearchParams(rawQuery ?? "");
      for (const [routeMethod, pattern, handler] of REST_ROUTES) {
        if (routeMethod !== method) continue;
        const match = pattern.exec(route);
        if (!match) continue;
        return restJSON(handler(match.slice(1), body ? JSON.parse(body) : undefined, query));
      }
      // Loud, and it names its own fix. A 404 here would read to the app as
      // "the server has no such thing", which is a plausible answer — and a
      // plausible wrong answer is what lets a suite go green against a mock
      // that no longer resembles the server.
      const message = `[mock] no REST route for ${method} ${route} — add one to REST_ROUTES in e2e/mock-backend.ts`;
      console.error(message);
      (window as any).__mockUnhandledRoutes = [
        ...((window as any).__mockUnhandledRoutes ?? []),
        `${method} ${route}`,
      ];
      return { status: 501, body: JSON.stringify({ error: message }) };
    };

    // Bindings the shell polls on every page. Each returned a silent null
    // before — Call.ByID's fallback for an unmocked id — and a null where a
    // component expects a list is a crash waiting for the right page.
    mock[IDS.GetFailedChangesCount] = () => 0;
    mock[IDS.ListChangesets] = () => [];
    mock[IDS.ListVoiceProfiles] = () => [];

    mock[IDS.StartWatching] = () => {};
    mock[IDS.StopWatching] = () => {};
    mock[IDS.UpdatePresence] = () => {};

    // Install on window for Call.ByID to find
    (window as any).__wailsMock = mock;

    // Also install a name-keyed map for test convenience
    const byName: Record<string, (...args: any[]) => any> = {};
    for (const [name, id] of Object.entries(IDS)) {
      if (mock[id as unknown as number]) {
        byName[name] = mock[id as unknown as number];
      }
    }
    (window as any).__wailsMockByName = byName;

    // Expose IDs so tests can monkey-patch __wailsMock by name lookup
    (window as any).__wailsIDs = IDS;

    // Expose projectFiles so recordings can add custom blocks and use PseudoTranslateItem
    (window as any).__projectFiles = projectFiles;
  }, wailsCallIds());
}

/**
 * Bypasses the ServerConnect screen by setting a flag that makes the
 * mock backend start in "connected" mode with a workspace.
 *
 * IMPORTANT: Must be called BEFORE page.goto("/"), then await the
 * returned promise to wait for the app to become ready.
 *
 * Usage:
 *   await injectMockBackend(page);
 *   const ready = skipConnectionScreen(page);
 *   await page.goto("/");
 *   await ready;
 */
export async function skipConnectionScreen(page: Page) {
  // Set a flag that the addInitScript mock handler reads.
  await page.addInitScript(() => {
    (window as any).__skipConnection = true;
  });
  // Wait for the app shell to appear: either the empty-projects onboarding or,
  // when the workspace already has projects, the nav rail. Both are testids —
  // a readiness gate must never key on copy, which is how this suite spent a
  // fortnight red after the dashboard hero was reworded (#1363).
  await page
    .getByTestId("empty-projects")
    .or(page.getByTestId("nav-translate"))
    .first()
    .waitFor({ state: "visible", timeout: 10000 });
}

/**
 * Convenience: injects mock backend, navigates to the app, and skips
 * the connection screen so existing tests reach the main app directly.
 */
export async function setupLocalApp(page: Page) {
  await injectMockBackend(page);
  // Set up the skip flag before navigating.
  await page.addInitScript(() => {
    (window as any).__skipConnection = true;
  });
  await page.goto("/");
  // Wait for the app shell to appear: either the empty-projects onboarding or,
  // when the workspace already has projects, the nav rail. Both are testids —
  // a readiness gate must never key on copy, which is how this suite spent a
  // fortnight red after the dashboard hero was reworded (#1363).
  await page
    .getByTestId("empty-projects")
    .or(page.getByTestId("nav-translate"))
    .first()
    .waitFor({ state: "visible", timeout: 10000 });
  // Launch is the path that rotted last time — getConfig and getCurrentUser are
  // both REST — so it is worth asserting outright rather than leaving to
  // whatever the spec happens to look at next.
  await assertNoUnhandledRoutes(page);
}

/**
 * Fails with the offending paths if the app asked the mock for a REST route it
 * does not serve.
 *
 * Call it after a flow that exercises new ground. The mock answers an unknown
 * route with 501, never 404: a 404 is a legitimate server answer that a
 * component may render as a harmless empty state, so it would let a spec pass
 * while the mock quietly stopped resembling the server.
 */
export async function assertNoUnhandledRoutes(page: Page) {
  const unhandled = await page.evaluate(
    () => (window as unknown as { __mockUnhandledRoutes?: string[] }).__mockUnhandledRoutes ?? [],
  );
  if (unhandled.length > 0) {
    throw new Error(
      `the app called REST routes the mock does not serve:\n  ${[...new Set(unhandled)].join("\n  ")}\n` +
        `Add them to REST_ROUTES in e2e/mock-backend.ts.`,
    );
  }
}

/**
 * Opens the project's source view — its collections, the files inside them and
 * its streams. Opening a project lands on its overview, so every test that
 * works with files goes through here first.
 */
/**
 * Opens a project from the workspace's project grid, by name.
 *
 * Scoped to the cards on purpose. `getByText(name).first()` used to do this,
 * and it worked only for as long as the name appeared exactly once: the shell
 * now also puts it in the breadcrumb trail and on the panel holding the
 * project's sections, so a bare text match can resolve to a heading that is not
 * a link to anywhere.
 */
export async function openProjectByName(page: Page, name: string) {
  await page.locator('[data-testid^="project-card-"]').filter({ hasText: name }).first().click();
}

export async function openProjectSource(page: Page) {
  // Project sections live in the secondary panel beside the rail, not in the
  // rail itself — the rail stays the workspace so Context/Insights/Settings
  // remain reachable from inside a project.
  await page.getByTestId("subnav-source").click();
  await page.getByTestId("file-drop-zone").waitFor({ state: "visible", timeout: 10000 });
}
