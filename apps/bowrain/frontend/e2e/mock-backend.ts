import type { Page } from "@playwright/test";

/**
 * Injects a mock Wails backend into the page.
 * This simulates window.go.backend.App with in-memory state.
 */
export async function injectMockBackend(page: Page) {
  await page.addInitScript(() => {
    let projectCounter = 0;
    const projects: Record<string, any> = {};
    const projectFiles: Record<string, Record<string, any>> = {};

    const mockBackend = {
      ListFormats: async () => [
        { name: "html", has_reader: true, has_writer: true },
        { name: "json", has_reader: true, has_writer: true },
        { name: "plaintext", has_reader: true, has_writer: true },
        { name: "xml", has_reader: true, has_writer: true },
        { name: "yaml", has_reader: true, has_writer: true },
      ],

      ListTools: async () => [
        { name: "ai-translate", description: "Translate content using AI" },
        { name: "pseudo-translate", description: "Generate pseudo-translations" },
        { name: "word-count", description: "Count words" },
      ],

      ListFlows: async () => [
        { name: "ai-translate", description: "AI translation flow" },
        { name: "pseudo-translate", description: "Pseudo-translation flow" },
      ],

      ListPlugins: async () => [],

      PluginDir: async () => "~/.kapi/plugins",

      CreateProject: async (name: string, sourceLang: string, targetLangs: string[]) => {
        const id = `project-${++projectCounter}`;
        const now = new Date().toISOString();
        const info = {
          id,
          name,
          source_locale: sourceLang,
          target_locales: targetLangs,
          path: "",
          files: [],
          created_at: now,
          modified_at: now,
        };
        projects[id] = info;
        projectFiles[id] = {};
        return info;
      },

      GetProject: async (projectID: string) => {
        const p = projects[projectID];
        if (!p) throw new Error(`Project ${projectID} not found`);
        return p;
      },

      ListProjects: async () => Object.values(projects),

      CloseProject: async (projectID: string) => {
        delete projects[projectID];
        delete projectFiles[projectID];
      },

      AddFiles: async (projectID: string, filePaths: string[]) => {
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

          // Create mock blocks
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

          p.files.push({
            name,
            format,
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
      },

      RemoveFile: async (projectID: string, fileName: string) => {
        const p = projects[projectID];
        if (!p) throw new Error(`Project ${projectID} not found`);
        p.files = p.files.filter((f: any) => f.name !== fileName);
        if (projectFiles[projectID]) delete projectFiles[projectID][fileName];
        p.modified_at = new Date().toISOString();
        return { ...p };
      },

      ListProjectFiles: async (projectID: string) => {
        const p = projects[projectID];
        if (!p) throw new Error(`Project ${projectID} not found`);
        return p.files;
      },

      GetFileBlocks: async (projectID: string, fileName: string) => {
        const files = projectFiles[projectID];
        if (!files || !files[fileName]) return [];
        return files[fileName];
      },

      UpdateBlockTarget: async (req: any) => {
        const files = projectFiles[req.project_id];
        if (!files || !files[req.file_name]) return;
        const block = files[req.file_name].find((b: any) => b.id === req.block_id);
        if (block) {
          block.targets[req.target_locale] = req.text;
        }
      },

      PseudoTranslateFile: async (projectID: string, fileName: string, targetLocale: string) => {
        const files = projectFiles[projectID];
        if (!files || !files[fileName]) throw new Error("File not found");
        const blocks = files[fileName];
        let translated = 0;
        let wordCount = 0;
        for (const b of blocks) {
          if (b.translatable) {
            b.targets[targetLocale] = `[${b.source}]`;
            translated++;
            wordCount += b.source.split(/\s+/).length;
          }
        }
        return { total_blocks: blocks.length, translated_blocks: translated, word_count: wordCount };
      },

      AITranslateFile: async (req: any) => {
        const files = projectFiles[req.project_id];
        if (!files || !files[req.file_name]) throw new Error("File not found");
        const blocks = files[req.file_name];
        let translated = 0;
        let wordCount = 0;
        for (const b of blocks) {
          if (b.translatable) {
            b.targets[req.target_locale] = `[AI] ${b.source}`;
            translated++;
            wordCount += b.source.split(/\s+/).length;
          }
        }
        return { total_blocks: blocks.length, translated_blocks: translated, word_count: wordCount };
      },

      TMTranslateFile: async (projectID: string, fileName: string, targetLocale: string) => {
        const files = projectFiles[projectID];
        if (!files || !files[fileName]) throw new Error("File not found");
        return { total_blocks: files[fileName].length, translated_blocks: 0, word_count: 0 };
      },

      GetWordCount: async (projectID: string, fileName: string) => {
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
      },

      ExportTranslatedFile: async (_projectID: string, fileName: string, targetLocale: string) => {
        const baseName = fileName.replace(/\.[^.]+$/, "");
        const ext = fileName.split(".").pop();
        return `/tmp/${baseName}_${targetLocale}.${ext}`;
      },

      OpenFileInOS: async () => {},

      SaveProject: async () => {},

      SaveProjectAs: async (projectID: string, path: string) => {
        const p = projects[projectID];
        if (p) p.path = path;
      },

      OpenProject: async (path: string) => {
        const id = `project-${++projectCounter}`;
        const info = {
          id,
          name: path.split("/").pop()?.replace(".kaz", "") || "Imported",
          source_locale: "en",
          target_locales: ["fr"],
          path,
          files: [],
          created_at: new Date().toISOString(),
          modified_at: new Date().toISOString(),
        };
        projects[id] = info;
        projectFiles[id] = {};
        return info;
      },
    };

    // Install as Wails backend
    (window as any).go = {
      backend: {
        App: mockBackend,
      },
    };
  });
}
