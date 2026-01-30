import { useState, useEffect, useCallback } from "react";
import type {
  FormatInfo,
  ToolInfo,
  FlowInfo,
  PluginInfo,
  ProjectInfo,
  BlockInfo,
  UpdateBlockRequest,
  UpdateBlockTargetCodedRequest,
  AITranslateFileRequest,
  TranslationStats,
  WordCountResult,
  ProviderConfig,
  ProviderConfigWithKey,
} from "../types/api";

// Wails v3 generates bindings at runtime. In dev mode we fall back to fetch.
// The Go backend methods are available as window.go.backend.App.*
interface WailsBackend {
  ListFormats(): Promise<FormatInfo[]>;
  ListTools(): Promise<ToolInfo[]>;
  ListFlows(): Promise<FlowInfo[]>;
  ListPlugins(): Promise<PluginInfo[]>;
  PluginDir(): Promise<string>;
  CreateProject(name: string, sourceLang: string, targetLangs: string[]): Promise<ProjectInfo>;
  GetProject(projectID: string): Promise<ProjectInfo>;
  ListProjects(): Promise<ProjectInfo[]>;
  CloseProject(projectID: string): Promise<void>;
  AddFiles(projectID: string, filePaths: string[]): Promise<ProjectInfo>;
  RemoveFile(projectID: string, fileName: string): Promise<ProjectInfo>;
  ListProjectFiles(projectID: string): Promise<ProjectInfo["items"]>;
  GetFileBlocks(projectID: string, fileName: string): Promise<BlockInfo[]>;
  RenderDocumentPreview(projectID: string, itemName: string, targetLocale: string): Promise<string>;
  RenderBlockHTML(projectID: string, itemName: string, blockID: string, targetLocale: string): Promise<string>;
  UpdateBlockTarget(req: UpdateBlockRequest): Promise<void>;
  UpdateBlockTargetCoded(req: UpdateBlockTargetCodedRequest): Promise<void>;
  PseudoTranslateFile(projectID: string, fileName: string, targetLocale: string): Promise<TranslationStats>;
  AITranslateFile(req: AITranslateFileRequest): Promise<TranslationStats>;
  TMTranslateFile(projectID: string, fileName: string, targetLocale: string): Promise<TranslationStats>;
  GetWordCount(projectID: string, fileName: string): Promise<WordCountResult>;
  ExportTranslatedFile(projectID: string, fileName: string, targetLocale: string): Promise<string>;
  OpenFileInOS(filePath: string): Promise<void>;
  SaveProject(projectID: string): Promise<void>;
  SaveProjectAs(projectID: string, path: string): Promise<void>;
  OpenProject(path: string): Promise<ProjectInfo>;
  OpenProjectDialog(): Promise<ProjectInfo | null>;
  SaveProjectDialog(projectID: string): Promise<void>;
  AddFilesDialog(projectID: string): Promise<ProjectInfo | null>;
  ListProviderConfigs(): Promise<ProviderConfig[]>;
  SaveProviderConfig(cfg: ProviderConfigWithKey): Promise<ProviderConfig>;
  DeleteProviderConfig(id: string): Promise<void>;
  TestProviderConfig(cfg: ProviderConfigWithKey): Promise<void>;
}

function getBackend(): WailsBackend | null {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const w = window as any;
  return w?.go?.backend?.App ?? null;
}

// Fallback fetch for non-Wails dev mode
const API_BASE = "/api/v1";
async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${res.statusText}`);
  return res.json() as Promise<T>;
}

export function useFormats() {
  const [formats, setFormats] = useState<FormatInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const be = getBackend();
    const p = be
      ? be.ListFormats()
      : fetchJSON<FormatInfo[]>(`${API_BASE}/formats`);

    p.then(setFormats)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return { formats, loading, error };
}

export function useTools() {
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const be = getBackend();
    const p = be
      ? be.ListTools()
      : fetchJSON<ToolInfo[]>(`${API_BASE}/tools`);

    p.then(setTools)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return { tools, loading, error };
}

export function useFlows() {
  const [flows, setFlows] = useState<FlowInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const be = getBackend();
    const p = be
      ? be.ListFlows()
      : fetchJSON<FlowInfo[]>(`${API_BASE}/flows`);

    p.then(setFlows)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return { flows, loading, error };
}

export function usePlugins() {
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [pluginDir, setPluginDir] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const be = getBackend();
    const pPlugins = be
      ? be.ListPlugins()
      : fetchJSON<PluginInfo[]>(`${API_BASE}/plugins`);
    const pDir = be
      ? be.PluginDir()
      : Promise.resolve("");

    Promise.all([pPlugins, pDir])
      .then(([pl, dir]) => {
        setPlugins(pl);
        setPluginDir(dir);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  return { plugins, pluginDir, loading, error };
}

export function useHealth() {
  const [connected, setConnected] = useState(false);

  const refresh = useCallback(() => {
    const be = getBackend();
    if (be) {
      // Wails is available - we're connected
      setConnected(true);
    } else {
      // Fallback: check REST API
      fetch(`${API_BASE}/health`)
        .then((r) => setConnected(r.ok))
        .catch(() => setConnected(false));
    }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 30000);
    return () => clearInterval(interval);
  }, [refresh]);

  return { connected, refresh };
}

// Project API hooks

export function useProjectApi() {
  const be = getBackend();

  const createProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[]): Promise<ProjectInfo> => {
      if (be) return be.CreateProject(name, sourceLang, targetLangs);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const listProjects = useCallback(async (): Promise<ProjectInfo[]> => {
    if (be) return be.ListProjects();
    return [];
  }, [be]);

  const openProject = useCallback(
    async (path: string): Promise<ProjectInfo> => {
      if (be) return be.OpenProject(path);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const openProjectDialog = useCallback(
    async (): Promise<ProjectInfo | null> => {
      if (be) return be.OpenProjectDialog();
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const saveProjectDialog = useCallback(
    async (projectID: string): Promise<void> => {
      if (be) return be.SaveProjectDialog(projectID);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const closeProject = useCallback(
    async (projectID: string): Promise<void> => {
      if (be) return be.CloseProject(projectID);
    },
    [be],
  );

  const addFiles = useCallback(
    async (projectID: string, filePaths: string[]): Promise<ProjectInfo> => {
      if (be) return be.AddFiles(projectID, filePaths);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const removeFile = useCallback(
    async (projectID: string, fileName: string): Promise<ProjectInfo> => {
      if (be) return be.RemoveFile(projectID, fileName);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const saveProject = useCallback(
    async (projectID: string): Promise<void> => {
      if (be) return be.SaveProject(projectID);
    },
    [be],
  );

  const saveProjectAs = useCallback(
    async (projectID: string, path: string): Promise<void> => {
      if (be) return be.SaveProjectAs(projectID, path);
    },
    [be],
  );

  const addFilesDialog = useCallback(
    async (projectID: string): Promise<ProjectInfo | null> => {
      if (be) return be.AddFilesDialog(projectID);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  return {
    createProject,
    listProjects,
    openProject,
    openProjectDialog,
    saveProjectDialog,
    closeProject,
    addFiles,
    removeFile,
    saveProject,
    saveProjectAs,
    addFilesDialog,
  };
}

export function useEditorApi() {
  const be = getBackend();

  const getFileBlocks = useCallback(
    async (projectID: string, fileName: string): Promise<BlockInfo[]> => {
      if (be) return be.GetFileBlocks(projectID, fileName);
      return [];
    },
    [be],
  );

  const updateBlockTarget = useCallback(
    async (req: UpdateBlockRequest): Promise<void> => {
      if (be) return be.UpdateBlockTarget(req);
    },
    [be],
  );

  const updateBlockTargetCoded = useCallback(
    async (req: UpdateBlockTargetCodedRequest): Promise<void> => {
      if (be) return be.UpdateBlockTargetCoded(req);
    },
    [be],
  );

  const pseudoTranslateFile = useCallback(
    async (projectID: string, fileName: string, targetLocale: string): Promise<TranslationStats> => {
      if (be) return be.PseudoTranslateFile(projectID, fileName, targetLocale);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const aiTranslateFile = useCallback(
    async (req: AITranslateFileRequest): Promise<TranslationStats> => {
      if (be) return be.AITranslateFile(req);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const tmTranslateFile = useCallback(
    async (projectID: string, fileName: string, targetLocale: string): Promise<TranslationStats> => {
      if (be) return be.TMTranslateFile(projectID, fileName, targetLocale);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const getWordCount = useCallback(
    async (projectID: string, fileName: string): Promise<WordCountResult> => {
      if (be) return be.GetWordCount(projectID, fileName);
      return { source_words: 0, source_chars: 0, target_words: {}, target_chars: {} };
    },
    [be],
  );

  const exportTranslatedFile = useCallback(
    async (projectID: string, fileName: string, targetLocale: string): Promise<string> => {
      if (be) return be.ExportTranslatedFile(projectID, fileName, targetLocale);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const openFileInOS = useCallback(
    async (filePath: string): Promise<void> => {
      if (be) return be.OpenFileInOS(filePath);
    },
    [be],
  );

  const renderDocumentPreview = useCallback(
    async (projectID: string, itemName: string, targetLocale: string): Promise<string> => {
      if (be) return be.RenderDocumentPreview(projectID, itemName, targetLocale);
      return "";
    },
    [be],
  );

  const renderBlockHTML = useCallback(
    async (projectID: string, itemName: string, blockID: string, targetLocale: string): Promise<string> => {
      if (be) return be.RenderBlockHTML(projectID, itemName, blockID, targetLocale);
      return "";
    },
    [be],
  );

  return {
    getFileBlocks,
    updateBlockTarget,
    updateBlockTargetCoded,
    pseudoTranslateFile,
    aiTranslateFile,
    tmTranslateFile,
    getWordCount,
    exportTranslatedFile,
    openFileInOS,
    renderDocumentPreview,
    renderBlockHTML,
  };
}

// Provider config hooks

export function useProviderConfigs() {
  const [configs, setConfigs] = useState<ProviderConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(() => {
    setLoading(true);
    const be = getBackend();
    if (be) {
      be.ListProviderConfigs()
        .then((c) => setConfigs(c || []))
        .catch((e) => setError(e.message))
        .finally(() => setLoading(false));
    } else {
      setConfigs([]);
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { configs, loading, error, refresh };
}

export function useProviderApi() {
  const be = getBackend();

  const saveProviderConfig = useCallback(
    async (cfg: ProviderConfigWithKey): Promise<ProviderConfig> => {
      if (be) return be.SaveProviderConfig(cfg);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const deleteProviderConfig = useCallback(
    async (id: string): Promise<void> => {
      if (be) return be.DeleteProviderConfig(id);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  const testProviderConfig = useCallback(
    async (cfg: ProviderConfigWithKey): Promise<void> => {
      if (be) return be.TestProviderConfig(cfg);
      throw new Error("Wails backend not available");
    },
    [be],
  );

  return { saveProviderConfig, deleteProviderConfig, testProviderConfig };
}
