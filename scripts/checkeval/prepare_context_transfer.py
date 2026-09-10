#!/usr/bin/env python3
"""Prepare isolated, staged context-transfer inputs without model calls."""

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import urllib.request
from pathlib import Path


FIXTURE = Path(__file__).with_name("context-transfer")
DOCUMENTS = {
    "README.md": "overview",
    "docs/getting-started.md": "tutorial",
    "docs/preview-links.md": "explanation",
    "docs/troubleshooting.md": "troubleshooting",
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("x", encoding="utf-8") as file:
        json.dump(value, file, indent=2, ensure_ascii=False)
        file.write("\n")


def stage(root, identifier, prompt, outputs, condition="baseline", binary=None):
    workspace = root / "workspaces" / identifier
    prompt_path = root / "prompts" / f"{identifier}.txt"
    prompt_path.parent.mkdir(exist_ok=True)
    with prompt_path.open("x", encoding="utf-8") as file:
        file.write(prompt)
    manifest = {"id": identifier, "workspace": str(workspace), "prompt_file": str(prompt_path),
                "condition": condition, "timeout_seconds": 600, "max_turns": 40,
                "input_files": sorted(str(p.relative_to(workspace)) for p in workspace.rglob("*") if p.is_file()),
                "expected_outputs": outputs}
    if binary:
        manifest["kapi_bin"] = str(binary.resolve(strict=True))
    write(root / "stages" / f"{identifier}.json", manifest)
    return manifest


def prepare(root):
    root.mkdir(parents=True, exist_ok=False)
    source_dir = root / "sources"
    source_dir.mkdir()
    source_manifest = json.loads((FIXTURE / "sources.json").read_text())
    records = []
    licenses = {}
    for source in source_manifest["sources"]:
        raw_url = f"https://raw.githubusercontent.com/{source['repository']}/{source['revision']}/{source['path']}"
        data = urllib.request.urlopen(raw_url, timeout=60).read()
        filename = source["id"] + Path(source["path"]).suffix
        (source_dir / filename).write_bytes(data)
        records.append({**source, "file": filename, "sha256": digest(data), "raw_url": raw_url})
        key = source["repository"]
        if key not in licenses:
            url = f"https://raw.githubusercontent.com/{key}/{source['revision']}/LICENSE"
            license_data = urllib.request.urlopen(url, timeout=60).read()
            filename = key.split("/")[-1] + "-LICENSE.txt"
            (source_dir / filename).write_bytes(license_data)
            licenses[key] = {"file": filename, "sha256": digest(license_data), "url": url}
    write(source_dir / "index.json", {"sources": records, "licenses": licenses})
    workspace = root / "workspaces" / "learn"
    workspace.mkdir(parents=True)
    shutil.copytree(source_dir, workspace / "sources")
    shutil.copyfile(FIXTURE / "facts.md", root / "facts.md")
    stage(root, "learn", (FIXTURE / "learn.txt").read_text(), ["context.json"])
    write(root / "preparation.json", {"source_index_sha256": digest((source_dir / "index.json").read_bytes()),
          "facts_sha256": digest((root / "facts.md").read_bytes()), "model_calls": 0})


def verified_file(root, relative, expected):
    path = Path(relative)
    if path.is_absolute() or ".." in path.parts:
        raise ValueError("Evidence path must stay within its directory")
    resolved = (root / path).resolve(strict=True)
    if not resolved.is_relative_to(root.resolve()):
        raise ValueError("Evidence path escapes its directory")
    data = resolved.read_bytes()
    if digest(data) != expected:
        raise ValueError("Retained evidence changed: " + str(relative))
    return data


def verify_preparation(root):
    preparation = json.loads((root / "preparation.json").read_text())
    verified_file(root, "facts.md", preparation["facts_sha256"])
    index = json.loads(verified_file(root / "sources", "index.json", preparation["source_index_sha256"]))
    for record in [*index["sources"], *index.get("licenses", {}).values()]:
        verified_file(root / "sources", record["file"], record["sha256"])
    return preparation


def retained_output(root, identifier, document):
    stage_dir = root / "ledger/stages" / identifier
    frozen = json.loads((stage_dir / "frozen.json").read_text())
    started = json.loads((stage_dir / "started.json").read_text())
    result = json.loads((stage_dir / "result.json").read_text())
    if not frozen.get("fingerprint") or any(record.get("fingerprint") != frozen["fingerprint"] for record in (started, result)):
        raise ValueError("Stage evidence identity differs: " + identifier)
    agent = result["agent"]
    expected_model = started["prepared"]["launch"]["agent"]["model"]
    if (result.get("error") or agent.get("error") or agent.get("status") != "completed"
            or agent.get("protocol_completed") is not True or agent.get("rate_limited")
            or not expected_model or agent.get("actual_model") != expected_model
            or agent.get("requested_model") != expected_model):
        raise ValueError("Stage has no eligible completed host result: " + identifier)
    if frozen["stage"]["id"] != identifier or document not in frozen["stage"]["expected_outputs"]:
        raise ValueError("Output was not declared for stage: " + document)
    records = [record for record in result["outputs"] if record["path"] == document]
    if len(records) != 1 or records[0].get("error") or not records[0].get("sha256"):
        raise ValueError("Stage has no verified retained output: " + document)
    record = records[0]
    data = verified_file(stage_dir / "outputs", document, record["sha256"])
    if len(data) != record.get("bytes", 0):
        raise ValueError("Retained output size differs: " + document)
    workspace = root / "workspaces" / identifier
    if (workspace / document).read_bytes() != data:
        raise ValueError("Workspace output differs from retained output: " + document)
    return data


def verify_frozen(root):
    preparation = verify_preparation(root)
    identity = json.loads((root / "frozen/identity.json").read_text())
    if identity["source_index_sha256"] != preparation["source_index_sha256"]:
        raise ValueError("Frozen source identity differs")
    context = verified_file(root / "frozen", "context.json", identity["context_sha256"])
    verified_file(root / "frozen", "learned-guide.md", identity["guide_sha256"])
    if retained_output(root, "learn", "context.json") != context:
        raise ValueError("Frozen context differs from retained learner output")
    return context


def freeze_context(root):
    verify_preparation(root)
    data = retained_output(root, "learn", "context.json")
    learned = json.loads(data)
    known = {s["id"] for s in json.loads((root / "sources/index.json").read_text())["sources"]}
    if set(learned) != {"title", "summary", "rules", "non_transferable", "uncertainties"}:
        raise ValueError("Unexpected learned context shape; retain raw output for inspection")
    seen = set()
    for rule in learned["rules"]:
        if rule["id"] in seen or not rule["statement"].strip() or rule["basis"] not in {"explicit", "inferred"}:
            raise ValueError("Invalid or duplicate learned rule")
        seen.add(rule["id"])
        if not rule["surfaces"] or not set(rule["surfaces"]) <= set(DOCUMENTS.values()):
            raise ValueError("Unknown surface in learned context")
        if not rule["sources"] or any(s["id"] not in known or not s["section"].strip() for s in rule["sources"]):
            raise ValueError("Unknown or unlocated learned source")
    frozen = root / "frozen"
    frozen.mkdir(exist_ok=False)
    (frozen / "context.json").write_bytes(data)
    guide = f"# {learned['title']}\n\n{learned['summary']}\n"
    for rule in learned["rules"]:
        sources = "; ".join(s["id"] + ": " + s["section"] for s in rule["sources"])
        guide += f"\n## {rule['id']}\n\n{rule['statement']}\n\nBasis: {rule['basis']}. Surfaces: {', '.join(rule['surfaces'])}. Sources: {sources}.\n"
    for key in ("non_transferable", "uncertainties"):
        guide += "\n## " + key.replace("_", " ").capitalize() + "\n\n"
        guide += "\n".join("- " + item for item in learned[key]) + "\n"
    (frozen / "learned-guide.md").write_text(guide)
    write(frozen / "identity.json", {"context_sha256": digest(data), "guide_sha256": digest(guide.encode()),
          "source_index_sha256": digest((root / "sources/index.json").read_bytes())})


def prepare_draft(root):
    # Learning must be frozen before this writer is launched or staged.
    verify_frozen(root)
    workspace = root / "workspaces" / "draft"
    workspace.mkdir(parents=True, exist_ok=False)
    shutil.copyfile(root / "facts.md", workspace / "facts.md")
    stage(root, "draft", (FIXTURE / "draft.txt").read_text(), list(DOCUMENTS))


def isolated_env(root):
    isolation = root / "resolver-state"
    (isolation / "plugins").mkdir(parents=True, exist_ok=True)
    return {**os.environ, "KAPI_NO_PROJECT": "1", "KAPI_CONFIG_DIR": str(isolation / "config"),
            "XDG_DATA_HOME": str(isolation / "data"), "XDG_CACHE_HOME": str(isolation / "cache"),
            "KAPI_PLUGINS_DIR_ONLY": "1", "KAPI_PLUGINS_DIR": str(isolation / "plugins")}


def compile_profile(learned):
    # Statements are copied without rewriting or adding semantic assertions.
    description = learned["summary"]
    for key in ("non_transferable", "uncertainties"):
        description += "\n\n" + key.replace("_", " ").capitalize() + ":\n"
        description += "\n".join("- " + item for item in learned[key])
    constraints = []
    for rule in learned["rules"]:
        for surface in rule["surfaces"]:
            constraints.append({"id": rule["id"] + "/" + surface, "version": 1,
                "kind": "guidance", "scope": {"channel": surface},
                "source": "; ".join(s["id"] + ": " + s["section"] for s in rule["sources"]),
                "statement": f"[{rule['basis']}] {rule['statement']}"})
    return {"name": learned["title"], "description": description, "constraints": constraints,
            "channels": {surface: {} for surface in DOCUMENTS.values()}}


def prepare_adaptations(root, binary):
    verify_frozen(root)
    draft = root / "workspaces/draft"
    if (draft / "facts.md").read_bytes() != (root / "facts.md").read_bytes():
        raise ValueError("Initial writer changed the factual brief")
    retained = {document: retained_output(root, "draft", document) for document in DOCUMENTS}
    original = root / "original"
    original.mkdir(exist_ok=False)
    originals = {}
    for document, data in retained.items():
        if not data.strip():
            raise ValueError("Empty initial document: " + document)
        target = original / document
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
        originals[document] = digest(data)
    write(original / "identity.json", originals)
    for identifier in ("references", "guidance", "kapi"):
        workspace = root / "workspaces" / identifier
        workspace.mkdir(exist_ok=False)
        shutil.copyfile(root / "facts.md", workspace / "facts.md")
        for document in DOCUMENTS:
            target = workspace / document
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(original / document, target)
    shutil.copytree(root / "sources", root / "workspaces/references/sources")
    kapi = root / "workspaces/kapi"
    learned = json.loads((root / "frozen/context.json").read_text())
    write(kapi / ".kapi/learned.yaml", compile_profile(learned))
    recipe = {"version": "v1", "name": "pageglass-context-transfer",
              "defaults": {"source_language": "en"},
              "profiles": {"pageglass": {"channels": list(DOCUMENTS.values()), "voice": ".kapi/learned.yaml"}},
              "collections": [{"name": surface, "channel": "pageglass/" + surface,
                "coordinates": {"product": "pageglass", "audience": "developer"},
                "content": [{"path": document}]} for document, surface in DOCUMENTS.items()]}
    write(kapi / "kapi.yaml", recipe)
    resolved = root / "resolved"
    resolved.mkdir()
    env = isolated_env(root)
    resolver_records = []
    for document, surface in DOCUMENTS.items():
        command = [str(binary), "-p", str(kapi / "kapi.yaml"), "context", document, "--json"]
        result = subprocess.run(command, cwd=kapi, env=env, capture_output=True, check=True)
        context = json.loads(result.stdout)
        if not context.get("voice", {}).get("guide"):
            raise ValueError("No actual resolved writing guide: " + document)
        filename = surface + ".json"
        (resolved / filename).write_bytes(result.stdout)
        resolver_records.append({"document": document, "surface": surface, "file": filename,
                                 "sha256": digest(result.stdout), "stderr": result.stderr.decode()})
    write(resolved / "index.json", {"documents": resolver_records,
        "meaning": "These exact kapi context responses are supplied as ordinary files in the guidance arm."})
    shutil.copytree(resolved, root / "workspaces/guidance/context")
    # Both compact-guidance arms can inspect the unchanged learned context and its qualifications.
    for identifier in ("guidance", "kapi"):
        shutil.copyfile(root / "frozen/learned-guide.md", root / "workspaces" / identifier / "learned-guide.md")
    suffixes = {
        "references": "Read sources/index.json and the retained source pages. Infer applicable writing guidance directly from these references. Treat source-project examples as examples, not Pageglass facts. Use ordinary reading and editing tools.\n",
        "guidance": "Read learned-guide.md and context/index.json, then the exact resolved context JSON for each document. These are plain reference files; use ordinary reading and editing tools to apply the relevant guidance. Explicit principles and inferred tendencies are labelled.\n",
        "kapi": "Use the installed kapi skill. Read learned-guide.md for provenance and qualifications, and retrieve kapi context for each document before editing. Use kapi formats to inspect Markdown editing capabilities, then inspect, apply --diff, apply, and check on saved edits. You may use ordinary authoring tools when structural reorganisation cannot be expressed through the supported edit route; document any such fallback in notes.md. Do not execute fictional pageglass commands. Semantic guidance is advisory and no semantic-check provider is configured: report unsupported coverage honestly and review wording yourself against the context. Do not change the profile or recipe.\n",
    }
    for identifier in suffixes:
        stage(root, identifier, (FIXTURE / "adapt.txt").read_text() + "\n" + suffixes[identifier],
              [*DOCUMENTS, "notes.md"], "skill" if identifier == "kapi" else "baseline",
              binary if identifier == "kapi" else None)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["prepare", "freeze-context", "draft", "adapt"])
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--kapi-bin", type=Path)
    args = parser.parse_args()
    root = args.out.resolve()
    if args.action == "adapt":
        if not args.kapi_bin:
            parser.error("adapt requires --kapi-bin")
        prepare_adaptations(root, args.kapi_bin.resolve(strict=True))
    else:
        {"prepare": prepare, "freeze-context": freeze_context, "draft": prepare_draft}[args.action](root)
    print(json.dumps({"action": args.action, "directory": str(root), "model_calls": 0}))
