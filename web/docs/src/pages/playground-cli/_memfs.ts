// An in-memory filesystem implementing the subset of the Node `fs` API that
// Go's js/wasm runtime calls (see syscall/fs_js.go). The browser has no
// filesystem, so we install this as `globalThis.fs` before booting the wasm
// kapi CLI; its os.* calls (open/read/write/stat/mkdir/rename/readdir/...)
// then operate on this tree. The same tree backs the "files" panel — uploads
// write into it, downloads read from it.
//
// Kept to erasable TypeScript syntax so it runs under both the browser bundler
// and `node --experimental-strip-types` (used by the test harness).

type NodeKind = "file" | "dir";

interface FSNode {
  kind: NodeKind;
  mtimeMs: number;
  // file
  content?: Uint8Array;
  // dir
  children?: Map<string, FSNode>;
}

interface FD {
  node: FSNode;
  path: string;
  pos: number;
  append: boolean;
}

interface MemFSOptions {
  onStdout?: (chunk: Uint8Array) => void;
  onStderr?: (chunk: Uint8Array) => void;
}

// Linux-style open flags. The exact values are arbitrary as long as we both
// publish them (fs.constants, read by Go at init) and interpret them here.
const O = {
  O_RDONLY: 0,
  O_WRONLY: 1,
  O_RDWR: 2,
  O_CREAT: 0o100,
  O_EXCL: 0o200,
  O_TRUNC: 0o1000,
  O_APPEND: 0o2000,
  O_DIRECTORY: 0o200000,
};

const S_IFDIR = 0o040000;
const S_IFREG = 0o100000;

function err(code: string, message?: string): Error & { code: string } {
  const e = new Error(message || code) as Error & { code: string };
  e.code = code;
  return e;
}

function dirNode(): FSNode {
  return { kind: "dir", children: new Map(), mtimeMs: Date.now() };
}
function fileNode(content?: Uint8Array): FSNode {
  return { kind: "file", content: content || new Uint8Array(0), mtimeMs: Date.now() };
}

// Normalize an absolute path into components, resolving "." and "..".
function splitPath(abs: string): string[] {
  const parts: string[] = [];
  for (const seg of abs.split("/")) {
    if (seg === "" || seg === ".") continue;
    if (seg === "..") parts.pop();
    else parts.push(seg);
  }
  return parts;
}

export interface MemVolume {
  // UI helpers (operate with absolute paths)
  writeFile(path: string, data: Uint8Array): void;
  readFile(path: string): Uint8Array;
  readdir(path: string): string[];
  mkdirp(path: string): void;
  remove(path: string): void;
  exists(path: string): boolean;
  isDir(path: string): boolean;
  cwd(): string;
}

export interface MemFS {
  fs: any;
  process: any;
  vol: MemVolume;
}

export function createMemFS(opts: MemFSOptions = {}): MemFS {
  const root = dirNode();
  let cwd = "/";
  const fds = new Map<number, FD>();
  let nextFd = 3; // 0,1,2 reserved for stdio

  function resolve(p: string): string[] {
    const abs = p.startsWith("/") ? p : cwd + "/" + p;
    return splitPath(abs);
  }
  function joinAbs(parts: string[]): string {
    return "/" + parts.join("/");
  }

  function lookup(parts: string[]): FSNode | null {
    let cur: FSNode = root;
    for (const name of parts) {
      if (cur.kind !== "dir" || !cur.children) return null;
      const next = cur.children.get(name);
      if (!next) return null;
      cur = next;
    }
    return cur;
  }
  function lookupParent(parts: string[]): { parent: FSNode; name: string } {
    if (parts.length === 0) throw err("EINVAL", "root has no parent");
    const parent = lookup(parts.slice(0, -1));
    if (!parent) throw err("ENOENT");
    if (parent.kind !== "dir") throw err("ENOTDIR");
    return { parent, name: parts[parts.length - 1] };
  }

  function statsFor(node: FSNode): any {
    const isDir = node.kind === "dir";
    const mode = (isDir ? S_IFDIR : S_IFREG) | (isDir ? 0o755 : 0o644);
    const size = isDir ? 0 : node.content!.length;
    const t = node.mtimeMs;
    return {
      dev: 1,
      ino: 1,
      mode,
      nlink: 1,
      uid: 0,
      gid: 0,
      rdev: 0,
      size,
      blksize: 4096,
      blocks: Math.ceil(size / 512),
      atimeMs: t,
      mtimeMs: t,
      ctimeMs: t,
      birthtimeMs: t,
      isDirectory: () => isDir,
      isFile: () => !isDir,
      isSymbolicLink: () => false,
      isBlockDevice: () => false,
      isCharacterDevice: () => false,
      isFIFO: () => false,
      isSocket: () => false,
    };
  }

  // ---------------- Node-style async fs API (what Go calls) ----------------

  const fs: any = {
    constants: O,

    open(p: string, flags: number, _mode: number, cb: Function) {
      try {
        const parts = resolve(p);
        let node = lookup(parts);
        const wantWrite = (flags & O.O_WRONLY) !== 0 || (flags & O.O_RDWR) !== 0;
        if (!node) {
          if ((flags & O.O_CREAT) === 0) return cb(err("ENOENT"));
          const { parent, name } = lookupParent(parts);
          node = fileNode();
          parent.children!.set(name, node);
          parent.mtimeMs = Date.now();
        } else if ((flags & O.O_CREAT) !== 0 && (flags & O.O_EXCL) !== 0) {
          return cb(err("EEXIST"));
        } else if (node.kind === "dir" && wantWrite) {
          return cb(err("EISDIR"));
        }
        if (node.kind === "file" && (flags & O.O_TRUNC) !== 0) {
          node.content = new Uint8Array(0);
        }
        const append = (flags & O.O_APPEND) !== 0;
        const fd = nextFd++;
        fds.set(fd, { node, path: joinAbs(parts), pos: append && node.content ? node.content.length : 0, append });
        cb(null, fd);
      } catch (e) {
        cb(e);
      }
    },

    close(fd: number, cb: Function) {
      fds.delete(fd);
      cb(null);
    },

    read(fd: number, buffer: Uint8Array, offset: number, length: number, position: number | null, cb: Function) {
      const e = fds.get(fd);
      if (!e) return cb(err("EBADF"));
      if (e.node.kind === "dir") return cb(err("EISDIR"));
      const content = e.node.content!;
      const pos = position === null || position === undefined ? e.pos : position;
      const slice = content.subarray(pos, Math.min(pos + length, content.length));
      buffer.set(slice, offset);
      if (position === null || position === undefined) e.pos = pos + slice.length;
      cb(null, slice.length, buffer);
    },

    write(fd: number, buffer: Uint8Array, offset: number, length: number, position: number | null, cb: Function) {
      if (fd === 1 || fd === 2) {
        const chunk = buffer.subarray(offset, offset + length);
        (fd === 1 ? opts.onStdout : opts.onStderr)?.(chunk);
        return cb(null, length, buffer);
      }
      const e = fds.get(fd);
      if (!e) return cb(err("EBADF"));
      const data = buffer.subarray(offset, offset + length);
      const pos = position === null || position === undefined ? (e.append ? e.node.content!.length : e.pos) : position;
      const cur = e.node.content!;
      const end = pos + data.length;
      if (end > cur.length) {
        const grown = new Uint8Array(end);
        grown.set(cur, 0);
        grown.set(data, pos);
        e.node.content = grown;
      } else {
        cur.set(data, pos);
      }
      e.node.mtimeMs = Date.now();
      if (position === null || position === undefined) e.pos = end;
      cb(null, length, buffer);
    },

    fsync(_fd: number, cb: Function) {
      cb(null);
    },

    fstat(fd: number, cb: Function) {
      const e = fds.get(fd);
      if (!e) return cb(err("EBADF"));
      cb(null, statsFor(e.node));
    },

    stat(p: string, cb: Function) {
      const node = lookup(resolve(p));
      if (!node) return cb(err("ENOENT"));
      cb(null, statsFor(node));
    },

    lstat(p: string, cb: Function) {
      fs.stat(p, cb);
    },

    mkdir(p: string, _perm: number, cb: Function) {
      try {
        const parts = resolve(p);
        if (lookup(parts)) return cb(err("EEXIST"));
        const { parent, name } = lookupParent(parts);
        parent.children!.set(name, dirNode());
        parent.mtimeMs = Date.now();
        cb(null);
      } catch (e) {
        cb(e);
      }
    },

    rmdir(p: string, cb: Function) {
      try {
        const parts = resolve(p);
        const node = lookup(parts);
        if (!node) return cb(err("ENOENT"));
        if (node.kind !== "dir") return cb(err("ENOTDIR"));
        if (node.children!.size > 0) return cb(err("ENOTEMPTY"));
        const { parent, name } = lookupParent(parts);
        parent.children!.delete(name);
        cb(null);
      } catch (e) {
        cb(e);
      }
    },

    unlink(p: string, cb: Function) {
      try {
        const parts = resolve(p);
        const node = lookup(parts);
        if (!node) return cb(err("ENOENT"));
        if (node.kind === "dir") return cb(err("EISDIR"));
        const { parent, name } = lookupParent(parts);
        parent.children!.delete(name);
        cb(null);
      } catch (e) {
        cb(e);
      }
    },

    rename(from: string, to: string, cb: Function) {
      try {
        const fromParts = resolve(from);
        const node = lookup(fromParts);
        if (!node) return cb(err("ENOENT"));
        const src = lookupParent(fromParts);
        const dstParts = resolve(to);
        const dst = lookupParent(dstParts);
        src.parent.children!.delete(src.name);
        dst.parent.children!.set(dst.name, node);
        dst.parent.mtimeMs = Date.now();
        cb(null);
      } catch (e) {
        cb(e);
      }
    },

    readdir(p: string, cb: Function) {
      const node = lookup(resolve(p));
      if (!node) return cb(err("ENOENT"));
      if (node.kind !== "dir") return cb(err("ENOTDIR"));
      cb(null, Array.from(node.children!.keys()));
    },

    ftruncate(fd: number, length: number, cb: Function) {
      const e = fds.get(fd);
      if (!e) return cb(err("EBADF"));
      const cur = e.node.content!;
      const grown = new Uint8Array(length);
      grown.set(cur.subarray(0, Math.min(length, cur.length)), 0);
      e.node.content = grown;
      cb(null);
    },

    truncate(p: string, length: number, cb: Function) {
      const node = lookup(resolve(p));
      if (!node) return cb(err("ENOENT"));
      if (node.kind !== "file") return cb(err("EISDIR"));
      const grown = new Uint8Array(length);
      grown.set(node.content!.subarray(0, Math.min(length, node.content!.length)), 0);
      node.content = grown;
      cb(null);
    },

    // Permission/ownership/time calls — accepted as no-ops.
    chmod: (_p: string, _m: number, cb: Function) => cb(null),
    fchmod: (_fd: number, _m: number, cb: Function) => cb(null),
    chown: (_p: string, _u: number, _g: number, cb: Function) => cb(null),
    fchown: (_fd: number, _u: number, _g: number, cb: Function) => cb(null),
    lchown: (_p: string, _u: number, _g: number, cb: Function) => cb(null),
    utimes: (_p: string, _a: number, _m: number, cb: Function) => cb(null),
    futimes: (_fd: number, _a: number, _m: number, cb: Function) => cb(null),

    // Unsupported — report ENOSYS so Go surfaces a clean error.
    symlink: (_t: string, _p: string, cb: Function) => cb(err("ENOSYS")),
    link: (_a: string, _b: string, cb: Function) => cb(err("ENOSYS")),
    readlink: (_p: string, cb: Function) => cb(err("EINVAL")),

    // Synchronous write — Go's runtime uses this for stdout/stderr.
    writeSync(fd: number, buffer: Uint8Array): number {
      if (fd === 1) opts.onStdout?.(buffer);
      else if (fd === 2) opts.onStderr?.(buffer);
      return buffer.length;
    },
  };

  // ---------------- process shim (cwd/chdir + ids Go reads) ----------------

  const process: any = {
    getuid: () => 0,
    getgid: () => 0,
    geteuid: () => 0,
    getegid: () => 0,
    getgroups: () => [],
    pid: 1,
    ppid: 0,
    umask: () => 0,
    cwd: () => cwd,
    chdir: (dir: string) => {
      const parts = resolve(dir);
      const node = lookup(parts);
      if (!node) throw err("ENOENT");
      if (node.kind !== "dir") throw err("ENOTDIR");
      cwd = joinAbs(parts);
    },
  };

  // ---------------- UI-facing volume helpers ----------------

  const vol: MemVolume = {
    mkdirp(p: string) {
      const parts = resolve(p);
      let cur = root;
      for (const name of parts) {
        let next = cur.children!.get(name);
        if (!next) {
          next = dirNode();
          cur.children!.set(name, next);
        } else if (next.kind !== "dir") {
          throw err("ENOTDIR");
        }
        cur = next;
      }
    },
    writeFile(p: string, data: Uint8Array) {
      const parts = resolve(p);
      const { parent, name } = lookupParent(parts);
      const existing = parent.children!.get(name);
      if (existing && existing.kind === "file") {
        existing.content = data;
        existing.mtimeMs = Date.now();
      } else {
        parent.children!.set(name, fileNode(data));
      }
      parent.mtimeMs = Date.now();
    },
    readFile(p: string): Uint8Array {
      const node = lookup(resolve(p));
      if (!node) throw err("ENOENT");
      if (node.kind !== "file") throw err("EISDIR");
      return node.content!;
    },
    readdir(p: string): string[] {
      const node = lookup(resolve(p));
      if (!node) throw err("ENOENT");
      if (node.kind !== "dir") throw err("ENOTDIR");
      return Array.from(node.children!.keys()).sort();
    },
    remove(p: string) {
      const parts = resolve(p);
      const { parent, name } = lookupParent(parts);
      parent.children!.delete(name);
    },
    exists(p: string): boolean {
      return lookup(resolve(p)) !== null;
    },
    isDir(p: string): boolean {
      const n = lookup(resolve(p));
      return n !== null && n.kind === "dir";
    },
    cwd: () => cwd,
  };

  // Seed a working directory and chdir into it.
  vol.mkdirp("/project");
  cwd = "/project";

  return { fs, process, vol };
}
