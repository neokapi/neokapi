"""The file surgery the KapiMart regeneration performs on its scratch tree.

Convergence writes a complete target file for every source file, filling the
units it has no answer for with the source text. That is right for a working
tree and wrong for a committed sample: a German file that is three-quarters
English reads as broken rather than as unfinished.

So the targets that ship are pruned to the keys that actually have an answer,
and the files that cannot be pruned that way are not shipped at all.
"""

import datetime
import hashlib
import json
import os
import shutil
import sys

# The window the sample's history sits in. A fixture that moves whenever it is
# regenerated produces a diff that says nothing, so every timestamp is derived
# from the unit it belongs to rather than read off a clock.
SPREAD_DAYS = 90


# ── formats ──────────────────────────────────────────────────────────────────
#
# Only key-addressed formats can be shipped partly translated, and each needs
# its own reader and writer to do it.


def _read_props(path):
    """Ordered (key, value) pairs, dropping comments and blanks."""
    out = []
    for line in open(path, encoding="utf-8").read().splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        out.append((key.strip(), value))
    return out


def _write_props(path, pairs):
    body = "".join(f"{k}={v}\n" for k, v in pairs)
    open(path, "w", encoding="utf-8").write(body)


def _flatten(obj, prefix=""):
    for key, value in obj.items():
        name = f"{prefix}.{key}" if prefix else key
        if isinstance(value, dict):
            yield from _flatten(value, name)
        else:
            yield name, value


def _prune_json(source, target):
    """The target's tree, keeping only leaves that differ from the source."""
    out = {}
    for key, value in source.items():
        if key not in target:
            continue
        if isinstance(value, dict):
            sub = _prune_json(value, target[key])
            if sub:
                out[key] = sub
        elif target[key] != value:
            out[key] = target[key]
    return out


def _load_shipped(spec_path):
    spec = json.load(open(spec_path, encoding="utf-8"))
    return spec["locales"], spec["catalogs"]


def _shipped_paths(locales, catalogs):
    for catalog in catalogs:
        for locale in locales:
            yield locale, catalog.replace("{locale}", locale)


def _source_of(rel, locale):
    return rel.replace(f"/{locale}/", "/en/")


# ── steps ────────────────────────────────────────────────────────────────────


def prune(work, spec_path):
    """Keep the shipped catalogs, stripped to the target language; drop the rest."""
    locales, catalogs = _load_shipped(spec_path)
    keep = {rel for _, rel in _shipped_paths(locales, catalogs)}

    for area in ("web", "src", "legal", "marketing"):
        for locale in locales:
            base = os.path.join(work, area, locale)
            if not os.path.isdir(base):
                continue
            for dirpath, _, files in os.walk(base):
                for name in files:
                    abs_path = os.path.join(dirpath, name)
                    rel = os.path.relpath(abs_path, work)
                    if rel not in keep:
                        os.remove(abs_path)

    for locale, rel in _shipped_paths(locales, catalogs):
        abs_path = os.path.join(work, rel)
        if not os.path.exists(abs_path):
            continue
        src_path = os.path.join(work, _source_of(rel, locale))

        if rel.endswith(".json"):
            source = json.load(open(src_path, encoding="utf-8"))
            target = json.load(open(abs_path, encoding="utf-8"))
            pruned = _prune_json(source, target)
            json.dump(pruned, open(abs_path, "w", encoding="utf-8"),
                      ensure_ascii=False, indent=2)
            open(abs_path, "a", encoding="utf-8").write("\n")
            kept, total = len(list(_flatten(pruned))), len(list(_flatten(source)))
        elif rel.endswith(".properties"):
            source = dict(_read_props(src_path))
            pairs = [(k, v) for k, v in _read_props(abs_path)
                     if k in source and v != source[k]]
            _write_props(abs_path, pairs)
            kept, total = len(pairs), len(source)
        else:
            raise SystemExit(f"prune: no partial writer for {rel}")

        print(f"    {rel}: {kept}/{total} keys")

    for area in ("web", "src", "legal", "marketing"):
        for locale in locales:
            base = os.path.join(work, area, locale)
            if os.path.isdir(base) and not any(os.scandir(base)):
                os.rmdir(base)


def _set_json_key(path, dotted, text):
    doc = json.load(open(path, encoding="utf-8"))
    node = doc
    parts = dotted.split(".")
    for part in parts[:-1]:
        node = node.setdefault(part, {})
    node[parts[-1]] = text
    json.dump(doc, open(path, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
    open(path, "a", encoding="utf-8").write("\n")


def _set_props_key(path, key, text):
    pairs = [(k, v) for k, v in _read_props(path) if k != key]
    pairs.append((key, text))
    pairs.sort(key=lambda kv: kv[0])
    _write_props(path, pairs)


def answer(work, spec_path):
    """Write answers a translator supplied into the committed targets.

    Convergence only reuses what the corpus already holds, so a string nobody
    has answered before has to arrive this way. It goes into the target file and
    through the rest of the loop unchanged, so it is absorbed and approved like
    any other answer rather than written into the store behind the loop's back.
    """
    spec = json.load(open(spec_path, encoding="utf-8"))
    answers = spec.get("answers") or [spec]
    written = 0

    for item in answers:
        key = item["key"]
        for locale, text in item["targets"].items():
            rel = item["file"].replace("{locale}", locale)
            abs_path = os.path.join(work, rel)
            if not os.path.exists(abs_path):
                raise SystemExit(f"answer: {rel} is not shipped, so it cannot carry {key}")
            if rel.endswith(".json"):
                _set_json_key(abs_path, key, text)
            elif rel.endswith(".properties"):
                _set_props_key(abs_path, key, text)
            else:
                raise SystemExit(f"answer: no writer for {rel}")
            written += 1

    print(f"    {written} answer(s) across {len(answers)} string(s)")


def approvals(review_path, out_path):
    """A review decision for every unit whose target says something new."""
    pending = json.load(open(review_path, encoding="utf-8"))["pending"]
    rows = [
        {"kind": "review", "file": u["file"], "id": u["key"],
         "locale": u["locale"], "status": "reviewed"}
        for u in pending
        if u.get("source") != u.get("target")
    ]
    with open(out_path, "w", encoding="utf-8") as fh:
        for row in rows:
            fh.write(json.dumps(row) + "\n")
    print(f"    {len(rows)} approval(s)")


def _approved_at(unit):
    """A stable instant for one unit, inside the window ending at the anchor.

    Spread rather than constant so the activity chart has a shape, derived from
    the unit rather than from a clock so the shape is the same on every machine,
    and keyed by unit so the decision that approved an answer and the memory
    entry that came out of it agree about when it happened.
    """
    anchor = datetime.datetime(2026, 2, 11, 8, 30, tzinfo=datetime.timezone.utc)
    digest = hashlib.sha256(unit.encode("utf-8")).digest()
    days = int.from_bytes(digest[:4], "big") % SPREAD_DAYS
    minutes = int.from_bytes(digest[4:6], "big") % (60 * 8)
    at = anchor - datetime.timedelta(days=days) + datetime.timedelta(minutes=minutes)
    return at.strftime("%Y-%m-%dT%H:%M:%SZ")


def settle(work):
    """Replace every generated timestamp with one derived from the unit."""
    bundle_path = os.path.join(work, ".kapi", "memory", "memory.json")
    bundle = json.load(open(bundle_path, encoding="utf-8"))
    for entry in bundle.get("entries", []):
        at = _approved_at(entry.get("unit") or entry.get("id", ""))
        for field in ("created", "updated"):
            if field in entry:
                entry[field] = at
        for origin in entry.get("origins", []):
            if "addedAt" in origin:
                origin["addedAt"] = at
    bundle.pop("created", None)
    json.dump(bundle, open(bundle_path, "w", encoding="utf-8"),
              ensure_ascii=False, indent=2, sort_keys=True)
    open(bundle_path, "a", encoding="utf-8").write("\n")

    state_dir = os.path.join(work, ".kapi", "state")
    for name in sorted(os.listdir(state_dir)):
        path = os.path.join(state_dir, name)
        rows = []
        for line in open(path, encoding="utf-8"):
            line = line.strip()
            if not line:
                continue
            row = json.loads(line)
            at = _approved_at(row.get("unit", ""))
            if "updated" in row:
                row["updated"] = at
            if isinstance(row.get("decision"), dict) and "at" in row["decision"]:
                row["decision"]["at"] = at
            rows.append(row)
        rows.sort(key=lambda r: (r.get("unit", ""), r.get("variant", "")))
        with open(path, "w", encoding="utf-8") as fh:
            for row in rows:
                fh.write(json.dumps(row, ensure_ascii=False, sort_keys=True) + "\n")


def install(work, sample, spec_path):
    """Copy the regenerated artifacts into the sample."""
    locales, catalogs = _load_shipped(spec_path)

    for area in ("web", "src", "legal", "marketing"):
        for locale in locales:
            stale = os.path.join(sample, area, locale)
            if os.path.isdir(stale):
                shutil.rmtree(stale)

    for _, rel in _shipped_paths(locales, catalogs):
        src = os.path.join(work, rel)
        if not os.path.exists(src):
            continue
        dst = os.path.join(sample, rel)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copyfile(src, dst)

    shutil.copyfile(os.path.join(work, ".kapi", "memory", "memory.json"),
                    os.path.join(sample, ".kapi", "memory", "memory.json"))

    state_dst = os.path.join(sample, ".kapi", "state")
    if os.path.isdir(state_dst):
        shutil.rmtree(state_dst)
    shutil.copytree(os.path.join(work, ".kapi", "state"), state_dst)


def summary(sample):
    bundle = json.load(open(os.path.join(sample, ".kapi", "memory", "memory.json"),
                            encoding="utf-8"))
    entries = bundle.get("entries", [])
    state_dir = os.path.join(sample, ".kapi", "state")
    rows = sum(1 for name in os.listdir(state_dir)
               for line in open(os.path.join(state_dir, name), encoding="utf-8")
               if line.strip())

    print(f"  memory entries      {len(entries)}")
    print(f"    carrying a unit   {sum(1 for e in entries if e.get('unit'))}")
    print(f"    carrying a point  {sum(1 for e in entries if e.get('point'))}")
    print(f"  unit-state rows     {rows}")


if __name__ == "__main__":
    verb = sys.argv[1] if len(sys.argv) > 1 else ""
    handlers = {
        "prune": (prune, 3), "answer": (answer, 3), "approvals": (approvals, 3),
        "settle": (settle, 2), "install": (install, 4), "summary": (summary, 2),
    }
    if verb not in handlers:
        print(f"usage: targets.py <{'|'.join(handlers)}> ...", file=sys.stderr)
        raise SystemExit(2)
    fn, argc = handlers[verb]
    if len(sys.argv) != argc + 1:
        print(f"targets.py {verb}: wrong number of arguments", file=sys.stderr)
        raise SystemExit(2)
    fn(*sys.argv[2:])
