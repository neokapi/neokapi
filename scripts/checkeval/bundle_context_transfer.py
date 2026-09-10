#!/usr/bin/env python3
"""Bundle retained context-transfer evidence without reading mutable arm outputs.

No model, subprocess or network calls. Rendering is a separate operation.
"""

import argparse
import json
from pathlib import Path

from report_context_transfer import DOCUMENTS, digest


TITLES = {"README.md": "Overview", "docs/getting-started.md": "Getting started",
          "docs/preview-links.md": "Preview links", "docs/troubleshooting.md": "Troubleshooting"}
ARMS = {"references": "Reference documents", "guidance": "Same guidance in plain files", "kapi": "Same guidance through kapi"}


class Bundle:
    def __init__(self, root, output):
        self.root, self.output = root.resolve(strict=True), output.resolve()
        self.output.mkdir(parents=True, exist_ok=False)
        self.artifacts = []

    def data(self, relative, expected=None):
        path = (self.root / relative).resolve(strict=True)
        if Path(relative).is_absolute() or not path.is_relative_to(self.root):
            raise ValueError("Source artifact escapes the run directory")
        data = path.read_bytes()
        if expected is not None and digest(data) != expected:
            raise ValueError("Source artifact hash differs: " + str(relative))
        return data

    def read(self, relative, expected=None):
        return json.loads(self.data(relative, expected))

    def put(self, relative, data):
        path = (self.output / relative).resolve()
        if Path(relative).is_absolute() or not path.is_relative_to(self.output):
            raise ValueError("Bundle artifact escapes its output directory")
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("xb") as file:
            file.write(data)
        artifact = {"path": str(relative), "sha256": digest(data)}
        self.artifacts.append(artifact)
        return artifact

    def copy(self, relative, expected=None):
        return self.put(relative, self.data(relative, expected))

    def exists(self, relative):
        return (self.root / relative).exists()


def stage(bundle, identifier, expected_stage_id=None):
    if expected_stage_id is not None and (identifier, expected_stage_id) != ("prior-auth-failure", "learn"):
        raise ValueError("Only the retained prior authentication failure may keep its original stage ID")
    directory = f"ledger/stages/{identifier}"
    if not bundle.exists(directory + "/frozen.json"):
        if bundle.exists(directory + "/started.json") or bundle.exists(directory + "/result.json"):
            raise ValueError("Stage records lack frozen input identity")
        prompt = bundle.copy(f"prompts/{identifier}.txt") if bundle.exists(f"prompts/{identifier}.txt") else None
        return {"observation": {"status": "not_started"}, "outputs": {}, "notes": [], "audit": {}, "prompt": prompt}
    frozen = bundle.read(directory + "/frozen.json")
    if frozen["stage"]["id"] != (expected_stage_id or identifier):
        raise ValueError("Frozen stage belongs to another stage")
    bundle.copy(directory + "/frozen.json")
    for path, sha in frozen["input_sha256"].items():
        bundle.copy(directory + "/inputs/" + path, sha)
    prompt = bundle.copy(directory + "/prompt.txt", frozen["prompt_sha256"])
    started = bundle.read(directory + "/started.json") if bundle.exists(directory + "/started.json") else {}
    result = bundle.read(directory + "/result.json") if bundle.exists(directory + "/result.json") else {}
    for record in (started, result):
        if record and record["fingerprint"] != frozen["fingerprint"]:
            raise ValueError("Stage result/start fingerprint differs from frozen inputs")
    if result and not started:
        raise ValueError("Stage result lacks a start reservation")
    agent = result.get("agent", {})
    observation = {"status": agent.get("status", "started_no_result" if started else "not_started")}
    for source, target in (("actual_model", "model"), ("duration_ms", "duration_ms"), ("tools", "tools")):
        if source in agent and (source != "actual_model" or agent[source]):
            observation[target] = agent[source]
    outputs, notes, seen = {}, [], set()
    for artifact in result.get("outputs", []):
        path = artifact["path"]
        if path in seen or path not in frozen["stage"]["expected_outputs"]:
            raise ValueError("Repeated or undeclared output artifact")
        seen.add(path)
        if artifact.get("error"):
            notes.append(path + ": " + artifact["error"])
        else:
            outputs[path] = bundle.copy(directory + "/outputs/" + path, artifact["sha256"])
    if result.get("error"):
        notes.append(result["error"])
    missing = set(frozen["stage"]["expected_outputs"]) - set(outputs)
    notes.extend("No immutable output retained: " + name for name in sorted(missing))
    audit = {"stage_id": frozen["stage"]["id"], "ledger_id": identifier, "fingerprint": frozen["fingerprint"], "input_sha256": frozen["input_sha256"],
             "prompt_sha256": frozen["prompt_sha256"], "runner_sha256": frozen.get("runner_sha256"),
             "skill_sha256": frozen.get("skill_sha256"), "kapi_sha256": frozen.get("kapi_sha256"),
             "started_at": started.get("started_at"), "finished_at": result.get("finished_at"),
             "requested_model": agent.get("requested_model"), "outputs": result.get("outputs", []), "issues": notes}
    return {"observation": observation, "outputs": outputs, "notes": notes, "audit": audit, "prompt": prompt}


def assemble(root, assessment, output):
    bundle = Bundle(root, output)
    preparation = bundle.read("preparation.json")
    sources = bundle.read("sources/index.json", preparation["source_index_sha256"])
    index = {"title": "Learning and applying documentation context", "description": "Planned comparison: independent Pageglass documentation adapted using full Astro references, compiled guidance as plain files, and that same guidance through kapi. Retained outputs below show which stages completed. This is a proof of concept, not a quality benchmark.",
             "brief": bundle.copy("facts.md", preparation["facts_sha256"]), "references": [], "guidance": [],
             "documents": [{"id": doc, "title": TITLES[doc]} for doc in TITLES], "adaptations": [], "stages": []}
    def reference(identifier, title, artifact, url=""):
        index["references"].append({"id": identifier, "title": title, "url": url, "artifact": artifact})
    reference("source-index", "Source attribution and pinned revisions", bundle.copy("sources/index.json", preparation["source_index_sha256"]))
    for source in sources["sources"]:
        reference(source["id"], source["title"], bundle.copy("sources/" + source["file"], source["sha256"]), source["url"])
    for repository, license in sources["licenses"].items():
        reference("license-" + repository, repository + " — retained license", bundle.copy("sources/" + license["file"], license["sha256"]), license["url"])
    stages = {name: stage(bundle, name) for name in ("learn", "draft", *ARMS)}
    if bundle.exists("ledger/stages/prior-auth-failure"):
        if not bundle.exists("ledger/stages/prior-auth-failure/frozen.json"):
            raise ValueError("Retained prior authentication failure lacks frozen identity")
        stages["prior-auth-failure"] = stage(bundle, "prior-auth-failure", expected_stage_id="learn")
        prior = stages["prior-auth-failure"]
        index["stages"].append({"id": "prior-auth-failure", "title": "Retained prior authentication failure", "observation": prior["observation"], "notes": prior["notes"]})
    recovery = bundle.read("recovery.json") if bundle.exists("recovery.json") else None
    if recovery is not None:
        reference("recovery", "Authentication recovery and retained attempt accounting", bundle.copy("recovery.json"))
    for name, record in stages.items():
        if record["prompt"]:
            reference("prompt-" + name, name + " — exact stage prompt", record["prompt"])
    if bundle.exists("frozen/identity.json"):
        identity = bundle.read("frozen/identity.json")
        if identity["source_index_sha256"] != preparation["source_index_sha256"]:
            raise ValueError("Learned guidance belongs to different source inputs")
        context = bundle.copy("frozen/context.json", identity["context_sha256"])
        learned_output = stages["learn"]["outputs"].get("context.json")
        if learned_output is None or learned_output["sha256"] != context["sha256"]:
            raise ValueError("Frozen learned context differs from immutable learn output")
        reference("learned-json", "Frozen learned context with provenance", context)
        index["guidance"].append({"title": "Frozen learned guidance", "artifact": bundle.copy("frozen/learned-guide.md", identity["guide_sha256"])})
    matched = []
    if bundle.exists("resolved/index.json"):
        resolved = bundle.read("resolved/index.json")
        reference("resolved-index", "Actual resolver response inventory", bundle.copy("resolved/index.json"))
        for record in resolved["documents"]:
            relative = "resolved/" + record["file"]
            actual = bundle.read(relative, record["sha256"])
            reference("context-" + record["surface"], "Raw kapi context: " + record["document"], bundle.copy(relative, record["sha256"]))
            guide = bundle.put("resolved/" + record["surface"] + ".md", actual["voice"]["guide"].encode())
            index["guidance"].append({"title": "Applied to " + record["document"], "artifact": guide})
            plain_hash = stages["guidance"]["audit"].get("input_sha256", {}).get("context/" + record["file"])
            if plain_hash is not None and plain_hash != record["sha256"]:
                raise ValueError("Plain guidance input differs from actual resolver output")
            matched.append({"document": record["document"], "resolved_sha256": record["sha256"],
                            "plain_input_sha256": plain_hash, "byte_identical": plain_hash == record["sha256"] if plain_hash is not None else None, "point": actual["point"]})
    original = {doc: item for doc, item in stages["draft"]["outputs"].items() if doc in DOCUMENTS}
    if bundle.exists("original/identity.json"):
        for doc, sha in bundle.read("original/identity.json").items():
            if doc not in original or original[doc]["sha256"] != sha:
                raise ValueError("Original checkpoint differs from immutable draft output")
            bundle.copy("original/" + doc, sha)
    index["original"] = {"title": "Original documentation", "documents": original, "observation": stages["draft"]["observation"], "notes": stages["draft"]["notes"]}
    for name, title in ARMS.items():
        record = stages[name]
        index["adaptations"].append({"id": name, "title": title, "documents": {doc: item for doc, item in record["outputs"].items() if doc in DOCUMENTS}, "observation": record["observation"], "notes": record["notes"]})
        if "notes.md" in record["outputs"]:
            reference(name + "-notes", title + " — full authoring notes", record["outputs"]["notes.md"])
    index["stages"].append({"id": "learn", "title": "Learn context from Astro references", "observation": stages["learn"]["observation"], "notes": stages["learn"]["notes"]})
    assessment_data = assessment.read_bytes()
    index["assessment"] = json.loads(assessment_data)
    reference("assessment", "Independent assessment record", bundle.put("assessment.json", assessment_data))
    audit = {"preparation": preparation, "recovery": recovery, "stages": {name: record["audit"] for name, record in stages.items()},
             "matched_guidance": matched, "artifacts": list(bundle.artifacts), "model_calls": 0,
             "meaning": "Artifact identity and observed execution only; no automatic quality score. Mutable adaptation workspace files are not used as outputs."}
    reference("audit", "Input hashes, timings and matched-guidance evidence", bundle.put("audit.json", (json.dumps(audit, indent=2) + "\n").encode()))
    bundle.put("index.json", (json.dumps(index, indent=2, ensure_ascii=False) + "\n").encode())
    return {"index": str(bundle.output / "index.json"), "artifacts": len(bundle.artifacts), "model_calls": 0}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", type=Path, required=True)
    parser.add_argument("--assessment", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(assemble(args.run, args.assessment, args.out), indent=2))
