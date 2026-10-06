// A project's context shared through a folder the page was given: the
// remote kapiSyncContext reads and writes (kapi/cmd/kapi-wasm-cli/
// contextremote.go), over a directory handle from the File System Access API
// (`showDirectoryPicker()`), or any directory handle, the origin private file
// system's included.
//
// The folder keeps the layout a `file` context backend keeps on disk
// (core/workspace/fileremote.go): log/<writer>/<first-op-id>.jsonl, blobs/
// and checkpoints/, one file per object, created once and never replaced. A
// machine whose recipe declares `context: {backend: file, path: <folder>}`
// and a page given the same folder share one context.
//
// Kept to erasable TypeScript syntax so it runs under Node's type stripping.

/** The part of a FileSystemFileHandle the remote uses. */
export interface FolderFile {
  readonly kind: "file";
  readonly name: string;
  getFile(): Promise<{ arrayBuffer(): Promise<ArrayBuffer> }>;
  createWritable(): Promise<{ write(data: Uint8Array): Promise<void>; close(): Promise<void> }>;
  /** Rename the file in its directory; absent where the browser lacks it. */
  move?(name: string): Promise<void>;
}

/** The part of a FileSystemDirectoryHandle the remote uses. */
export interface FolderHandle {
  readonly kind: "directory";
  readonly name: string;
  getDirectoryHandle(name: string, options?: { create?: boolean }): Promise<FolderHandle>;
  getFileHandle(name: string, options?: { create?: boolean }): Promise<FolderFile>;
  removeEntry(name: string): Promise<void>;
  values(): AsyncIterable<FolderHandle | FolderFile>;
}

/** One object a push writes. */
export interface RemoteObject {
  name: string;
  data: Uint8Array;
}

/** The remote kapiSyncContext takes (the page contract in contextremote.go). */
export interface ContextRemote {
  location: string;
  list(dir: string): Promise<string[]>;
  get(name: string): Promise<Uint8Array | null>;
  put(objects: RemoteObject[]): Promise<void>;
}

const notFound = (e: unknown) =>
  e instanceof Error && (e.name === "NotFoundError" || e.name === "TypeMismatchError");

/** The directory at the slash-separated path, or null when it is not there. */
async function dirAt(
  root: FolderHandle,
  parts: string[],
  create: boolean,
): Promise<FolderHandle | null> {
  let dir = root;
  for (const part of parts) {
    try {
      dir = await dir.getDirectoryHandle(part, { create });
    } catch (e) {
      if (!create && notFound(e)) return null;
      throw e;
    }
  }
  return dir;
}

/** Every file below dir, as names prefixed with `prefix`. */
async function walk(dir: FolderHandle, prefix: string, out: string[]): Promise<void> {
  for await (const entry of dir.values()) {
    if (entry.kind === "directory") await walk(entry, `${prefix}${entry.name}/`, out);
    // A write in progress is not an object.
    else if (!entry.name.startsWith(".")) out.push(`${prefix}${entry.name}`);
  }
}

async function readFile(file: FolderFile): Promise<Uint8Array> {
  return new Uint8Array(await (await file.getFile()).arrayBuffer());
}

const sameBytes = (a: Uint8Array, b: Uint8Array) =>
  a.length === b.length && a.every((v, i) => v === b[i]);

/** An error the engine reads as "the remote already holds that object with other bytes". */
function objectExists(name: string): Error {
  const e = new Error(`${name} is already in the folder with other bytes`);
  e.name = "ObjectExists";
  return e;
}

/** Split a layout name into its directory parts and its file name. */
function split(name: string): { parts: string[]; base: string } {
  const parts = name.split("/");
  const base = parts.pop() as string;
  return { parts, base };
}

/** Read the file at name, null when it is not there. */
async function readAt(root: FolderHandle, name: string): Promise<Uint8Array | null> {
  const { parts, base } = split(name);
  const dir = await dirAt(root, parts, false);
  if (!dir) return null;
  try {
    return await readFile(await dir.getFileHandle(base));
  } catch (e) {
    if (notFound(e)) return null;
    throw e;
  }
}

/**
 * The remote over a folder. `location` names it in messages and keys what the
 * engine knows about it; it defaults to the folder's name.
 */
export function folderRemote(root: FolderHandle, location = root.name): ContextRemote {
  return {
    location,
    async list(dir: string): Promise<string[]> {
      const parts = dir.replace(/\/$/, "").split("/").filter(Boolean);
      const at = await dirAt(root, parts, false);
      if (!at) return [];
      const out: string[] = [];
      await walk(at, `${parts.join("/")}/`, out);
      return out.sort();
    },
    get: (name: string) => readAt(root, name),
    async put(objects: RemoteObject[]): Promise<void> {
      for (const obj of objects) {
        const held = await readAt(root, obj.name);
        if (held) {
          if (!sameBytes(held, obj.data)) throw objectExists(obj.name);
          continue;
        }
        const { parts, base } = split(obj.name);
        const dir = (await dirAt(root, parts, true)) as FolderHandle;
        // Written under a name the layout ignores and moved into place where
        // the browser can, so a reader of the folder never meets half an object.
        const temp = `.tmp-${base}-${Math.random().toString(36).slice(2)}`;
        let file = await dir.getFileHandle(temp, { create: true });
        const movable = typeof file.move === "function";
        if (!movable) {
          // The writable still lands its bytes in one step when it closes.
          await dir.removeEntry(temp);
          file = await dir.getFileHandle(base, { create: true });
        }
        const writable = await file.createWritable();
        await writable.write(obj.data);
        await writable.close();
        if (movable) await (file.move as (name: string) => Promise<void>).call(file, base);
      }
    },
  };
}
