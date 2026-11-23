"""Read a pnpm lockfile the way pnpm does: as YAML.

A pnpm 12 lockfile holds two YAML documents, one for the package manager
itself and one for the workspace. Lock parses the last one and refuses a
lockfile the parser and a line count disagree about (a duplicate key, which
the parser merges silently), so a check built on it sees the entries pnpm
sees.
"""

import re

import yaml

DEP_SECTIONS = ("dependencies", "devDependencies", "optionalDependencies")


class LockError(Exception):
    pass


def version_key(v):
    """A sort key for a semver version; a release sorts after its prereleases."""
    core, _, pre = v.partition("-")
    nums = tuple(int(p) if p.isdigit() else -1 for p in core.split("."))
    pre_key = tuple((0, int(p), "") if p.isdigit() else (1, 0, p) for p in pre.split(".")) if pre else ()
    return (nums, 0 if pre else 1, pre_key)


def split_key(key):
    """'@scope/name@1.2.3(peer@1)' -> ('@scope/name', '1.2.3')."""
    bare = key.split("(", 1)[0]
    at = bare.rfind("@")
    if at <= 0:
        raise LockError(f"cannot read the package key {key!r}")
    return bare[:at], bare[at + 1:]


def top_level_keys(doc_text, section):
    """The keys indented two spaces under `section:`, counted line by line."""
    keys, inside = [], False
    for line in doc_text.split("\n"):
        if re.match(r"^\S", line):
            inside = line.rstrip() == f"{section}:"
            continue
        if inside and re.match(r"^  [^\s#]", line):
            keys.append(line)
    return keys


class Lock:
    """The workspace document of a pnpm lockfile."""

    def __init__(self, text, label):
        self.label = label
        docs = text.split("\n---\n")
        doc_text = docs[-1]
        # The documents before the workspace's describe the package manager
        # itself (pnpm and its platform binaries); their resolutions are held
        # to the same form.
        self.manager_entries = {}
        for i, other in enumerate(docs[:-1]):
            try:
                mdoc = yaml.load(other, Loader=getattr(yaml, "CSafeLoader", yaml.SafeLoader)) or {}
            except yaml.YAMLError as e:
                raise LockError(f"{label}: document {i + 1} is not valid YAML: {e}") from e
            mpk = (mdoc.get("packages") or {}) if isinstance(mdoc, dict) else {}
            if len(top_level_keys(other, "packages")) != len(mpk):
                raise LockError(f"{label}: document {i + 1} packages disagree between the parser and the lines")
            for key, body in mpk.items():
                self.manager_entries[split_key(key)] = (body or {}).get("resolution")
        try:
            doc = yaml.load(doc_text, Loader=getattr(yaml, "CSafeLoader", yaml.SafeLoader))
        except yaml.YAMLError as e:
            raise LockError(f"{label}: not valid YAML: {e}") from e
        if not isinstance(doc, dict) or not str(doc.get("lockfileVersion", "")):
            raise LockError(f"{label}: no lockfileVersion")
        self.packages = doc.get("packages") or {}
        self.snapshots = doc.get("snapshots") or {}
        self.importers = doc.get("importers") or {}
        for section, parsed in (("packages", self.packages), ("snapshots", self.snapshots),
                                ("importers", self.importers)):
            if not isinstance(parsed, dict):
                raise LockError(f"{label}: {section} is not a mapping")
            counted = len(top_level_keys(doc_text, section))
            if counted != len(parsed):
                raise LockError(f"{label}: {section} has {counted} keys by line and {len(parsed)} parsed")
        self.entries = {}
        for key, body in self.packages.items():
            name, ver = split_key(key)
            if (name, ver) in self.entries:
                raise LockError(f"{label}: {name}@{ver} is listed twice")
            self.entries[(name, ver)] = (body or {}).get("resolution")
        self.edges = self._edges()

    def _edges(self):
        """{(dependent, package name): version} for every registry dependency."""
        edges = {}

        def add(dependent, name, ref):
            ref = str(ref)
            if ref.startswith(("link:", "file:", "workspace:")):
                return
            base = ref.split("(", 1)[0]
            if not base[:1].isdigit():
                # An alias ('@scope/real@1.0.0').
                name, base = split_key(base)
            edges[(dependent, name)] = base

        for path, imp in self.importers.items():
            for section in DEP_SECTIONS:
                for name, spec in ((imp or {}).get(section) or {}).items():
                    add(f"importer {path}", name, spec["version"] if isinstance(spec, dict) else spec)
        for key, snap in self.snapshots.items():
            for section in ("dependencies", "optionalDependencies"):
                for name, ref in ((snap or {}).get(section) or {}).items():
                    add(key, name, ref)
        return edges

    def versions(self, name):
        return [v for (n, v) in self.entries if n == name]


def has_integrity(res):
    return isinstance(res, dict) and str(res.get("integrity", "")).startswith("sha512-")


def integrity_problems(lock):
    """Every dependency of an importer or a package resolves to a packages
    entry with an integrity hash."""
    problems = []
    for (dependent, name), ver in sorted(lock.edges.items()):
        res = lock.entries.get((name, ver))
        if res is None:
            problems.append(f"{dependent} -> {name}@{ver}: no packages entry")
        elif not has_integrity(res):
            problems.append(f"{dependent} -> {name}@{ver}: no integrity hash")
    return problems


def form_problems(lock, local_dir=None):
    """Every resolution is an integrity hash and nothing else: a package from
    the configured registry. pnpm writes a tarball or a git commit only for a
    package from somewhere else. A `file:` dependency resolves to a directory;
    local_dir(path) says whether that directory is one this repository holds,
    and without it no directory is accepted."""
    problems = []
    for (name, ver), res in sorted(lock.entries.items()) + sorted(lock.manager_entries.items()):
        if has_integrity(res) and set(res) == {"integrity"}:
            continue
        if (isinstance(res, dict) and set(res) == {"type", "directory"} and res["type"] == "directory"
                and ver.startswith("file:") and local_dir and local_dir(str(res["directory"]))):
            continue
        problems.append(f"{name}@{ver}: resolution {res} is not an integrity hash alone")
    return problems
