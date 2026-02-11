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
 * Injects a mock Wails backend into the page.
 * Intercepts @wailsio/runtime with a mock module that delegates
 * Call.ByID to window.__wailsMock, then populates mock handlers
 * via addInitScript.
 */
export async function injectMockBackend(page: Page) {
  // Intercept the Wails runtime module and serve our mock
  await page.route("**/node_modules/.vite/deps/@wailsio*", (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: MOCK_RUNTIME_MODULE,
    });
  });

  // Also intercept any direct @wailsio/runtime imports (in case Vite resolves differently)
  await page.route("**/@wailsio/runtime*", (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/javascript",
      body: MOCK_RUNTIME_MODULE,
    });
  });

  // Register mock backend handlers keyed by Wails v3 binding call IDs
  await page.addInitScript(() => {
    let projectCounter = 0;
    let providerCounter = 0;
    const projects: Record<string, any> = {};
    const projectFiles: Record<string, Record<string, any>> = {};
    const providerConfigs: Record<string, any> = {};

    // Call IDs from the generated bindings (app.js)
    const IDS = {
      AITranslateFile: 112372233,
      AddConcept: 3793362128,
      AddFiles: 253527467,
      AddFilesDialog: 2566359367,
      AddTMEntry: 4152216329,
      CloseProject: 3497243272,
      CreateProject: 2219834270,
      DeleteConcept: 3900518770,
      DeleteFlowDefinition: 488503031,
      DeleteProviderConfig: 2103858573,
      DeleteTMEntry: 1886996955,
      ExportTermsJSON: 166152904,
      ExportTranslatedFile: 2327530811,
      GetFileBlocks: 711926175,
      GetKnownLocales: 3316047929,
      GetLocaleDisplayName: 804882882,
      GetFlowDefinition: 2095856838,
      GetInitialProject: 4269707510,
      GetProject: 1329939084,
      GetTMCount: 1658982651,
      GetTMEntries: 2865323100,
      GetTermCount: 433951236,
      GetTerms: 2057317714,
      GetWordCount: 2276123042,
      ImportTermsCSV: 4189664393,
      ImportTermsJSON: 498422205,
      ListFlowDefinitions: 3738265581,
      ListFlows: 2730811374,
      ListFormats: 1658666415,
      ListPlugins: 1851753111,
      ListProjectFiles: 3993987349,
      ListProjects: 1552139139,
      ListProviderConfigs: 1091807543,
      ListTools: 2273599896,
      LookupTMForBlock: 2472708440,
      LookupTerms: 1594665302,
      LookupTermsForBlock: 2436021002,
      OpenFileInOS: 2953479918,
      OpenProject: 3349152314,
      OpenProjectDialog: 3415023882,
      PluginDir: 2089167097,
      PseudoTranslateFile: 2296272289,
      RemoveFile: 1600906915,
      RenderBlockHTML: 3630764479,
      RenderDocumentPreview: 3649056848,
      SaveFlowDefinition: 2719448633,
      SaveProject: 179173205,
      SaveProjectAs: 3259052957,
      SaveProjectDialog: 2824494121,
      SaveProviderConfig: 755781535,
      SetApplication: 1182474139,
      TMTranslateFile: 49623414,
      TermEnforceItem: 2381680068,
      TestProviderConfig: 2576607394,
      UpdateBlockTarget: 1528557592,
      UpdateBlockTargetCoded: 1612164251,
      UpdateConcept: 1418320064,
      UpdateTMEntry: 1441483449,
    };

    // Global TM storage (workspace-scoped, not per-project)
    const tmStore: Record<string, any> = {};
    let tmEntryCounter = 0;

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
      { name: "ai-translate", description: "Translate content using AI" },
      { name: "pseudo-translate", description: "Generate pseudo-translations" },
      { name: "word-count", description: "Count words" },
    ];

    mock[IDS.ListFlows] = () => [
      { name: "ai-translate", description: "AI translation flow" },
      { name: "pseudo-translate", description: "Pseudo-translation flow" },
    ];

    // --- Flow definition storage ---
    const builtInFlowDefs: Record<string, any> = {
      "ai-translate": {
        id: "ai-translate",
        name: "AI Translate",
        description: "Translate content using AI/LLM",
        source: "built-in",
        nodes: [
          { id: "reader", type: "reader", name: "auto", label: "Input", position: { x: 0, y: 100 } },
          { id: "ai-translate", type: "tool", name: "ai-translate", label: "AI Translate", position: { x: 250, y: 100 } },
          { id: "writer", type: "writer", name: "auto", label: "Output", position: { x: 500, y: 100 } },
        ],
        edges: [
          { id: "e-reader-translate", source: "reader", target: "ai-translate" },
          { id: "e-translate-writer", source: "ai-translate", target: "writer" },
        ],
      },
      "pseudo-translate": {
        id: "pseudo-translate",
        name: "Pseudo Translate",
        description: "Generate pseudo-translations for testing",
        source: "built-in",
        nodes: [
          { id: "reader", type: "reader", name: "auto", label: "Input", position: { x: 0, y: 100 } },
          { id: "pseudo-translate", type: "tool", name: "pseudo-translate", label: "Pseudo Translate", position: { x: 250, y: 100 } },
          { id: "writer", type: "writer", name: "auto", label: "Output", position: { x: 500, y: 100 } },
        ],
        edges: [
          { id: "e-reader-pseudo", source: "reader", target: "pseudo-translate" },
          { id: "e-pseudo-writer", source: "pseudo-translate", target: "writer" },
        ],
      },
    };
    const userFlowDefs: Record<string, any> = {};

    mock[IDS.ListFlowDefinitions] = () => [
      ...Object.values(builtInFlowDefs),
      ...Object.values(userFlowDefs),
    ];

    mock[IDS.GetFlowDefinition] = (id: string) => {
      if (builtInFlowDefs[id]) return builtInFlowDefs[id];
      if (userFlowDefs[id]) return userFlowDefs[id];
      throw new Error(`Flow ${id} not found`);
    };

    mock[IDS.SaveFlowDefinition] = (def: any) => {
      if (def.source === "built-in") throw new Error("Cannot modify built-in flows");
      def.source = "user";
      def.modified_at = new Date().toISOString();
      if (!def.created_at) def.created_at = def.modified_at;
      userFlowDefs[def.id] = { ...def };
      return { ...def };
    };

    mock[IDS.DeleteFlowDefinition] = (id: string) => {
      if (builtInFlowDefs[id]) throw new Error(`Cannot delete built-in flow ${id}`);
      if (!userFlowDefs[id]) throw new Error(`Flow ${id} not found`);
      delete userFlowDefs[id];
    };

    mock[IDS.ListPlugins] = () => [];
    mock[IDS.PluginDir] = () => "~/.kapi/plugins";
    mock[IDS.SetApplication] = () => {};
    mock[IDS.GetInitialProject] = () => "";

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
        source_locale: sourceLang,
        target_locales: targetLangs,
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

    mock[IDS.AddFiles] = (projectID: string, filePaths: string[]) => {
      const p = projects[projectID];
      if (!p) throw new Error(`Project ${projectID} not found`);

      for (const fp of filePaths) {
        const name = fp.split("/").pop() || fp;
        const ext = name.split(".").pop() || "";
        const formatMap: Record<string, string> = {
          html: "html", htm: "html", json: "json", txt: "plaintext",
          xml: "xml", yaml: "yaml", yml: "yaml", po: "po",
          properties: "properties", md: "markdown",
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
          word_count: blocks.reduce(
            (sum: number, b: any) => sum + b.source.split(/\s+/).length,
            0,
          ),
        });
      }

      p.modified_at = new Date().toISOString();
      return { ...p };
    };

    mock[IDS.RemoveFile] = (projectID: string, fileName: string) => {
      const p = projects[projectID];
      if (!p) throw new Error(`Project ${projectID} not found`);
      p.items = p.items.filter((f: any) => f.name !== fileName);
      if (projectFiles[projectID]) delete projectFiles[projectID][fileName];
      p.modified_at = new Date().toISOString();
      return { ...p };
    };

    mock[IDS.ListProjectFiles] = (projectID: string) => {
      const p = projects[projectID];
      if (!p) throw new Error(`Project ${projectID} not found`);
      return p.items;
    };

    mock[IDS.GetFileBlocks] = (projectID: string, fileName: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[fileName]) return [];
      return files[fileName].map((b: any) => ({
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
        block.targets[req.target_locale] = req.text;
      }
    };

    mock[IDS.UpdateBlockTargetCoded] = (req: any) => {
      const itemName = req.item_name || req.file_name;
      const files = projectFiles[req.project_id];
      if (!files || !files[itemName]) return;
      const block = files[itemName].find((b: any) => b.id === req.block_id);
      if (block) {
        const plain = req.coded_text.replace(/[\uE001-\uE003]/g, "");
        block.targets[req.target_locale] = plain;
        if (!block.targets_coded) block.targets_coded = {};
        block.targets_coded[req.target_locale] = req.coded_text;
      }
    };

    mock[IDS.PseudoTranslateFile] = (projectID: string, fileName: string, targetLocale: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[fileName]) throw new Error("File not found");
      const blocks = files[fileName];
      let translated = 0;
      let wordCount = 0;
      for (const b of blocks) {
        if (b.translatable) {
          b.targets[targetLocale] = `[${b.source}]`;
          if (!b.properties) b.properties = {};
          b.properties["translation-origin"] = "pseudo";
          b.properties["translation-status"] = "draft";
          translated++;
          wordCount += b.source.split(/\s+/).length;
        }
      }
      return { total_blocks: blocks.length, translated_blocks: translated, word_count: wordCount };
    };

    mock[IDS.AITranslateFile] = (req: any) => {
      const itemName = req.item_name || req.file_name;
      const files = projectFiles[req.project_id];
      if (!files || !files[itemName]) throw new Error("File not found");
      const blocks = files[itemName];
      let translated = 0;
      let wordCount = 0;
      for (const b of blocks) {
        if (b.translatable) {
          b.targets[req.target_locale] = `[AI] ${b.source}`;
          if (!b.properties) b.properties = {};
          b.properties["translation-origin"] = "machine";
          b.properties["translation-status"] = "draft";
          translated++;
          wordCount += b.source.split(/\s+/).length;
        }
      }
      return { total_blocks: blocks.length, translated_blocks: translated, word_count: wordCount };
    };

    mock[IDS.TMTranslateFile] = (projectID: string, fileName: string, targetLocale: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[fileName]) throw new Error("File not found");
      const blocks = files[fileName];
      const entries = Object.values(tmStore);
      let translated = 0;
      let wordCount = 0;
      for (const b of blocks) {
        if (!b.translatable || b.targets[targetLocale]) continue;
        // Find exact or fuzzy match from TM
        const exact = entries.find((e: any) =>
          e.source.toLowerCase() === b.source.toLowerCase() &&
          e.target_locale === targetLocale
        );
        if (exact) {
          b.targets[targetLocale] = (exact as any).target;
          if (!b.properties) b.properties = {};
          b.properties["translation-origin"] = "tm";
          b.properties["translation-status"] = "draft";
          translated++;
          wordCount += b.source.split(/\s+/).length;
        }
      }
      return { total_blocks: blocks.length, translated_blocks: translated, word_count: wordCount };
    };

    mock[IDS.GetWordCount] = (projectID: string, fileName: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[fileName]) return { source_words: 0, source_chars: 0, target_words: {}, target_chars: {} };
      const blocks = files[fileName];
      let sourceWords = 0;
      let sourceChars = 0;
      const targetWords: Record<string, number> = {};
      const targetChars: Record<string, number> = {};
      for (const b of blocks) {
        if (b.translatable) {
          sourceWords += b.source.split(/\s+/).length;
          sourceChars += b.source.length;
          for (const [locale, text] of Object.entries(b.targets)) {
            const t = text as string;
            targetWords[locale] = (targetWords[locale] || 0) + t.split(/\s+/).length;
            targetChars[locale] = (targetChars[locale] || 0) + t.length;
          }
        }
      }
      return { source_words: sourceWords, source_chars: sourceChars, target_words: targetWords, target_chars: targetChars };
    };

    mock[IDS.ExportTranslatedFile] = (_projectID: string, fileName: string, targetLocale: string) => {
      const baseName = fileName.replace(/\.[^.]+$/, "");
      const ext = fileName.split(".").pop();
      return `/tmp/${baseName}_${targetLocale}.${ext}`;
    };

    mock[IDS.OpenFileInOS] = () => {};
    mock[IDS.SaveProject] = () => {};

    mock[IDS.SaveProjectAs] = (projectID: string, path: string) => {
      const p = projects[projectID];
      if (p) p.path = path;
    };

    mock[IDS.RenderDocumentPreview] = (_projectID: string, itemName: string, _targetLocale: string) => {
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

    mock[IDS.RenderBlockHTML] = (projectID: string, itemName: string, blockID: string, targetLocale: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) return "";
      const block = files[itemName].find((b: any) => b.id === blockID);
      if (!block) return "";
      if (targetLocale && block.targets[targetLocale]) {
        return block.targets[targetLocale];
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

    // --- TM mock handlers ---

    mock[IDS.GetTMEntries] = (_projectID: string, query: string, sourceLocale: string, targetLocale: string, offset: number, limit: number) => {
      let entries = Object.values(tmStore);

      if (query) {
        const q = query.toLowerCase();
        entries = entries.filter((e: any) =>
          e.source.toLowerCase().includes(q) || e.target.toLowerCase().includes(q)
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

    mock[IDS.GetTMCount] = () => {
      return Object.keys(tmStore).length;
    };

    mock[IDS.AddTMEntry] = (_projectID: string, source: string, target: string, sourceLocale: string, targetLocale: string) => {
      const id = `tm-entry-${++tmEntryCounter}`;
      const entry = {
        id,
        source,
        target,
        source_locale: sourceLocale,
        target_locale: targetLocale,
        updated_at: new Date().toISOString(),
      };
      tmStore[id] = entry;
      return entry;
    };

    mock[IDS.UpdateTMEntry] = (req: any) => {
      if (!tmStore[req.entry_id]) throw new Error("TM entry not found");
      tmStore[req.entry_id] = {
        ...tmStore[req.entry_id],
        source: req.source,
        target: req.target,
        source_locale: req.source_locale,
        target_locale: req.target_locale,
        updated_at: new Date().toISOString(),
      };
    };

    mock[IDS.DeleteTMEntry] = (_projectID: string, entryID: string) => {
      if (!tmStore[entryID]) throw new Error("TM entry not found");
      delete tmStore[entryID];
    };

    // --- Context panel: per-block TM and term lookup ---

    mock[IDS.LookupTMForBlock] = (projectID: string, itemName: string, blockID: string, targetLocale: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) return [];
      const block = files[itemName].find((b: any) => b.id === blockID);
      if (!block) return [];
      const entries = Object.values(tmStore);
      const matches: any[] = [];
      for (const e of entries) {
        const entry = e as any;
        if (targetLocale && entry.target_locale !== targetLocale) continue;
        const srcLower = block.source.toLowerCase();
        const entryLower = entry.source.toLowerCase();
        if (srcLower === entryLower) {
          matches.push({ source: entry.source, target: entry.target, score: 1.0, match_type: "exact" });
        } else if (srcLower.includes(entryLower) || entryLower.includes(srcLower)) {
          const longer = Math.max(srcLower.length, entryLower.length);
          const shorter = Math.min(srcLower.length, entryLower.length);
          const score = shorter / longer;
          if (score > 0.5) {
            matches.push({ source: entry.source, target: entry.target, score, match_type: "fuzzy" });
          }
        }
      }
      matches.sort((a: any, b: any) => b.score - a.score);
      return matches;
    };

    mock[IDS.LookupTermsForBlock] = (projectID: string, itemName: string, blockID: string, targetLocale: string) => {
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
              .filter((tt: any) => tt.locale !== t.locale && (!targetLocale || tt.locale === targetLocale))
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

    mock[IDS.GetTerms] = (_projectID: string, query: string, sourceLocale: string, targetLocale: string, offset: number, limit: number) => {
      let concepts = Object.values(termsStore);

      if (query) {
        const q = query.toLowerCase();
        concepts = concepts.filter((c: any) =>
          c.terms.some((t: any) => t.text.toLowerCase().includes(q)) ||
          (c.domain && c.domain.toLowerCase().includes(q)) ||
          (c.definition && c.definition.toLowerCase().includes(q))
        );
      }
      if (sourceLocale) {
        concepts = concepts.filter((c: any) =>
          c.terms.some((t: any) => t.locale === sourceLocale)
        );
      }
      if (targetLocale) {
        concepts = concepts.filter((c: any) =>
          c.terms.some((t: any) => t.locale === targetLocale)
        );
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

    mock[IDS.LookupTerms] = (_projectID: string, text: string, _sourceLocale: string, targetLocale: string) => {
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
              .filter((tt: any) => tt.locale !== t.locale && (!targetLocale || tt.locale === targetLocale))
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

    mock[IDS.ImportTermsCSV] = (_projectID: string, content: string, sourceLocale: string, targetLocale: string, domain: string, hasHeader: boolean) => {
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
      return JSON.stringify({
        name,
        concepts: Object.values(termsStore),
      }, null, 2);
    };

    mock[IDS.TermEnforceItem] = (projectID: string, itemName: string, targetLocale: string) => {
      const files = projectFiles[projectID];
      if (!files || !files[itemName]) return [];
      const concepts = Object.values(termsStore);
      const results: any[] = [];
      for (const b of files[itemName]) {
        if (!b.translatable || !b.targets[targetLocale]) continue;
        const srcLower = b.source.toLowerCase();
        const tgtLower = b.targets[targetLocale].toLowerCase();
        for (const c of concepts) {
          const concept = c as any;
          const srcTerms = concept.terms.filter((t: any) =>
            srcLower.includes(t.text.toLowerCase())
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
                target_text: b.targets[targetLocale],
                source_locale: "en",
                target_locale: targetLocale,
              });
            }
          }
        }
      }
      return results;
    };

    mock[IDS.OpenProject] = (path: string) => {
      const id = `project-${++projectCounter}`;
      const info = {
        id,
        name: path.split("/").pop()?.replace(".kaz", "") || "Imported",
        source_locale: "en",
        target_locales: ["fr"],
        path,
        items: [],
        created_at: new Date().toISOString(),
        modified_at: new Date().toISOString(),
      };
      projects[id] = info;
      projectFiles[id] = {};
      return info;
    };

    mock[IDS.OpenProjectDialog] = () => {
      const id = `project-${++projectCounter}`;
      const info = {
        id,
        name: "test-project",
        source_locale: "en",
        target_locales: ["fr"],
        path: "/mock/test-project.kaz",
        items: [],
        created_at: new Date().toISOString(),
        modified_at: new Date().toISOString(),
      };
      projects[id] = info;
      projectFiles[id] = {};
      return info;
    };

    mock[IDS.SaveProjectDialog] = (projectID: string) => {
      const p = projects[projectID];
      if (p) p.path = `/mock/${p.name}.kaz`;
    };

    mock[IDS.AddFilesDialog] = (projectID: string) => {
      return mock[IDS.AddFiles](projectID, ["/mock/dialog-file.txt"]);
    };

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
  });
}
