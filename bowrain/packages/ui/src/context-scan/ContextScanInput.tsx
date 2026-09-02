import { Button, Card, Input, Label, Textarea } from "@neokapi/ui-primitives";
import { useCallback, useRef, useState } from "react";
import type { ContextScanRequest, SkippedFile } from "../types/api";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { TEST_IDS } from "../test-ids";
import {
  AlertTriangle,
  FileText,
  FolderOpen,
  Link as LinkIcon,
  Loader2,
  Plus,
  Upload,
  X,
} from "../components/icons";

/** The server fetches at most this many pages per scan. */
export const MAX_SCAN_URLS = 5;
/** The upload endpoint accepts at most this many files per scan. */
export const MAX_SCAN_FILES = 10;

export interface ContextScanInputProps {
  /** Called once the scan job is enqueued. */
  onStarted: (jobId: string, request: ContextScanRequest) => void;
  /** Prefill, e.g. when regenerating a scan with the same request. */
  initialRequest?: ContextScanRequest;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * The brand-scan intake surface: paste text, add page links, drop files, or
 * point at a repository. Everything is optional, but at least one source is
 * required to start. Files are uploaded to the blob store first; the scan
 * itself runs as a background job the caller polls.
 */
export function ContextScanInput({ onStarted, initialRequest }: ContextScanInputProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [pasteText, setPasteText] = useState(initialRequest?.paste_text ?? "");
  const [urls, setUrls] = useState<string[]>(initialRequest?.urls ?? []);
  const [urlDraft, setUrlDraft] = useState("");
  const [urlError, setUrlError] = useState<string | null>(null);
  const [files, setFiles] = useState<File[]>([]);
  const [repoUrl, setRepoUrl] = useState(initialRequest?.repo_url ?? "");
  const [profileName, setProfileName] = useState(initialRequest?.profile_name ?? "");
  const [domain, setDomain] = useState(initialRequest?.domain ?? "");
  const [skipped, setSkipped] = useState<SkippedFile[]>([]);
  // Blob keys of already-uploaded files, held while the start is paused on a
  // non-empty skipped list (see handleStart). Non-null means "the next start
  // click is the explicit 'Start anyway' confirmation — reuse these keys".
  const [pendingKeys, setPendingKeys] = useState<string[] | null>(null);
  const [phase, setPhase] = useState<"idle" | "uploading" | "starting">("idle");
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const hasSource =
    pasteText.trim().length > 0 || urls.length > 0 || files.length > 0 || repoUrl.trim().length > 0;

  const addUrl = useCallback(() => {
    const value = urlDraft.trim();
    if (!value) return;
    if (!value.startsWith("https://")) {
      setUrlError("Only https:// links can be fetched.");
      return;
    }
    if (urls.length >= MAX_SCAN_URLS) {
      setUrlError(`A scan reads at most ${MAX_SCAN_URLS} pages.`);
      return;
    }
    if (urls.includes(value)) {
      setUrlError("That link is already in the list.");
      return;
    }
    setUrls((prev) => [...prev, value]);
    setUrlDraft("");
    setUrlError(null);
  }, [urlDraft, urls]);

  const addFiles = useCallback((incoming: FileList | File[]) => {
    // Materialize before the state update: both FileList sources (input.files,
    // dataTransfer.files) are live objects that the browser empties when the
    // input is reset, and React may run the updater only after the change
    // handler has already cleared the input — reading the FileList inside the
    // updater would then silently drop the picked files.
    const picked = Array.from(incoming);
    setFiles((prev) => {
      const merged = [...prev];
      for (const f of picked) {
        if (merged.length >= MAX_SCAN_FILES) break;
        if (!merged.some((m) => m.name === f.name && m.size === f.size)) merged.push(f);
      }
      return merged;
    });
    // Changing the file set invalidates any paused upload: the held keys no
    // longer describe what would be scanned.
    setPendingKeys(null);
    setSkipped([]);
  }, []);

  const handleStart = useCallback(async () => {
    if (!hasSource || phase !== "idle") return;
    setError(null);
    try {
      let uploadKeys: string[] = [];
      if (files.length > 0) {
        if (pendingKeys) {
          // Explicit "Start anyway" after the pause below: the accepted files
          // are already uploaded, so reuse their keys without re-uploading.
          uploadKeys = pendingKeys;
        } else {
          setSkipped([]);
          setPhase("uploading");
          const result = await api.uploadContextScanSources(ws, files);
          uploadKeys = result.uploads.map((u) => u.key);
          const skippedFiles = result.skipped ?? [];
          if (skippedFiles.length > 0) {
            // Pause instead of starting: starting now would navigate to the
            // job page and unmount the skipped-files panel before it can be
            // read, silently dropping those files from the scan. The button
            // flips to "Start anyway" for an explicit second click.
            setSkipped(skippedFiles);
            setPhase("idle");
            if (
              uploadKeys.length === 0 &&
              !pasteText.trim() &&
              urls.length === 0 &&
              !repoUrl.trim()
            ) {
              setError("None of the files could be read. Add another source to start the scan.");
              return;
            }
            setPendingKeys(uploadKeys);
            return;
          }
        }
      }
      const request: ContextScanRequest = {};
      if (pasteText.trim()) request.paste_text = pasteText;
      if (urls.length > 0) request.urls = urls;
      if (repoUrl.trim()) request.repo_url = repoUrl.trim();
      if (uploadKeys.length > 0) request.upload_keys = uploadKeys;
      if (profileName.trim()) request.profile_name = profileName.trim();
      if (domain.trim()) request.domain = domain.trim();

      if (!request.paste_text && !request.urls && !request.repo_url && !request.upload_keys) {
        setError("None of the files could be read. Add another source to start the scan.");
        setPhase("idle");
        return;
      }

      setPhase("starting");
      const { job_id } = await api.startContextScan(ws, request);
      onStarted(job_id, request);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setPhase("idle");
    }
  }, [
    api,
    ws,
    files,
    pasteText,
    urls,
    repoUrl,
    profileName,
    domain,
    hasSource,
    phase,
    pendingKeys,
    onStarted,
  ]);

  /** Non-null pending keys + a visible skipped list = the paused confirm state. */
  const awaitingSkipConfirm = pendingKeys !== null && skipped.length > 0;

  return (
    <div className="space-y-4">
      {/* Paste text */}
      <Card className="p-5 space-y-3">
        <div className="flex items-center gap-2">
          <FileText className="w-4 h-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Paste text</h2>
        </div>
        <Textarea
          data-testid={TEST_IDS.contextScan.input}
          value={pasteText}
          onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setPasteText(e.target.value)}
          placeholder="Paste anything that sounds like you: web copy, an About page, product descriptions, a style guide…"
          rows={5}
          className="text-sm"
        />
      </Card>

      {/* Web pages */}
      <Card className="p-5 space-y-3">
        <div className="flex items-center gap-2">
          <LinkIcon className="w-4 h-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Web pages</h2>
          <span className="text-xs text-muted-foreground">up to {MAX_SCAN_URLS}</span>
        </div>
        <div className="flex gap-2">
          <Input
            data-testid={TEST_IDS.contextScan.urlInput}
            value={urlDraft}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
              setUrlDraft(e.target.value);
              setUrlError(null);
            }}
            onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
              if (e.key === "Enter") {
                e.preventDefault();
                addUrl();
              }
            }}
            placeholder="https://example.com/about"
            className="text-sm"
          />
          <Button
            type="button"
            variant="outline"
            onClick={addUrl}
            disabled={!urlDraft.trim() || urls.length >= MAX_SCAN_URLS}
          >
            <Plus className="w-3.5 h-3.5 mr-1" /> Add
          </Button>
        </div>
        {urlError && <p className="text-xs text-destructive">{urlError}</p>}
        {urls.length > 0 && (
          <ul className="space-y-1.5">
            {urls.map((u) => (
              <li
                key={u}
                className="flex items-center justify-between gap-2 rounded-md border border-border/50 bg-muted/30 px-3 py-1.5 text-xs"
              >
                <span className="truncate font-mono">{u}</span>
                <button
                  type="button"
                  aria-label={`Remove ${u}`}
                  onClick={() => setUrls((prev) => prev.filter((x) => x !== u))}
                  className="p-0.5 rounded text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer shrink-0"
                >
                  <X className="w-3.5 h-3.5" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {/* File upload */}
      <Card className="p-5 space-y-3">
        <div className="flex items-center gap-2">
          <Upload className="w-4 h-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Files</h2>
          <span className="text-xs text-muted-foreground">
            brand guides, docs, marketing copy; up to {MAX_SCAN_FILES} files, 10 MB each
          </span>
        </div>
        <div
          data-testid={TEST_IDS.contextScan.fileDrop}
          role="button"
          tabIndex={0}
          onClick={() => fileInputRef.current?.click()}
          onKeyDown={(e: React.KeyboardEvent) => {
            if (e.key === "Enter" || e.key === " ") fileInputRef.current?.click();
          }}
          onDragOver={(e: React.DragEvent) => e.preventDefault()}
          onDrop={(e: React.DragEvent) => {
            e.preventDefault();
            if (e.dataTransfer.files.length > 0) addFiles(e.dataTransfer.files);
          }}
          className="flex flex-col items-center justify-center gap-1.5 rounded-md border border-dashed border-border px-4 py-8 text-center cursor-pointer transition-colors hover:border-primary/50 hover:bg-muted/30"
        >
          <Upload className="w-5 h-5 text-muted-foreground" />
          <p className="text-sm">Drop files here or click to browse</p>
          <p className="text-xs text-muted-foreground">
            docx, odt, epub, md, html, csv, txt and similar text formats
          </p>
        </div>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
            if (e.target.files) addFiles(e.target.files);
            e.target.value = "";
          }}
        />
        {files.length > 0 && (
          <ul className="space-y-1.5">
            {files.map((f) => (
              <li
                key={`${f.name}-${f.size}`}
                className="flex items-center justify-between gap-2 rounded-md border border-border/50 bg-muted/30 px-3 py-1.5 text-xs"
              >
                <span className="truncate">{f.name}</span>
                <span className="flex items-center gap-2 shrink-0">
                  <span className="text-muted-foreground">{formatSize(f.size)}</span>
                  <button
                    type="button"
                    aria-label={`Remove ${f.name}`}
                    onClick={() => {
                      setFiles((prev) => prev.filter((x) => x !== f));
                      setPendingKeys(null);
                      setSkipped([]);
                    }}
                    className="p-0.5 rounded text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer"
                  >
                    <X className="w-3.5 h-3.5" />
                  </button>
                </span>
              </li>
            ))}
          </ul>
        )}
        {skipped.length > 0 && (
          <div className="rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 space-y-1">
            <p className="text-xs font-medium flex items-center gap-1.5">
              <AlertTriangle className="w-3.5 h-3.5 text-amber-500" />
              Some files were skipped
            </p>
            <ul className="text-xs text-muted-foreground space-y-0.5">
              {skipped.map((s) => (
                <li key={s.name}>
                  <span className="font-medium text-foreground/80">{s.name}</span>: {s.reason}
                </li>
              ))}
            </ul>
          </div>
        )}
      </Card>

      {/* Repository */}
      <Card className="p-5 space-y-3">
        <div className="flex items-center gap-2">
          <FolderOpen className="w-4 h-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Repository</h2>
          <span className="text-xs text-muted-foreground">README and docs are read for voice</span>
        </div>
        <Input
          value={repoUrl}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setRepoUrl(e.target.value)}
          placeholder="https://github.com/acme/website"
          className="text-sm font-mono"
        />
      </Card>

      {/* Optional details */}
      <Card className="p-5 space-y-3">
        <h2 className="text-sm font-semibold">Details (optional)</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <Label htmlFor="context-scan-profile-name">Profile name</Label>
            <Input
              id="context-scan-profile-name"
              value={profileName}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setProfileName(e.target.value)}
              placeholder="e.g. Acme Voice"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="context-scan-domain">Subject domain</Label>
            <Input
              id="context-scan-domain"
              value={domain}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setDomain(e.target.value)}
              placeholder="e.g. developer tools"
            />
          </div>
        </div>
      </Card>

      {error && (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {awaitingSkipConfirm
            ? "The skipped files will not be read. Start anyway, or adjust the files first."
            : hasSource
              ? "The scan drafts a voice profile and candidate terms. Nothing is saved until you approve."
              : "Add at least one source to start."}
        </p>
        <Button
          data-testid={TEST_IDS.contextScan.start}
          onClick={handleStart}
          disabled={!hasSource || phase !== "idle"}
        >
          {phase === "uploading" ? (
            <>
              <Loader2 className="w-4 h-4 mr-1.5 animate-spin" /> Uploading…
            </>
          ) : phase === "starting" ? (
            <>
              <Loader2 className="w-4 h-4 mr-1.5 animate-spin" /> Starting…
            </>
          ) : awaitingSkipConfirm ? (
            "Start anyway"
          ) : (
            "Scan my brand"
          )}
        </Button>
      </div>
    </div>
  );
}
