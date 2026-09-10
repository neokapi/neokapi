"""Offline bundle integrity checks using synthetic retained runner artifacts."""

import json
import tempfile
import unittest
from pathlib import Path

from bundle_context_transfer import assemble, digest
from report_context_transfer import load_index


def fixture(root):
    def store(relative, value):
        raw = value.encode() if isinstance(value, str) else (json.dumps(value) + "\n").encode()
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(raw)
        return digest(raw)
    source_sha = store("sources/style.md", "# Writing guide\nUse clear verbs.\n")
    license_sha = store("sources/LICENSE.txt", "Synthetic license record.\n")
    source_index = {"sources": [{"id": "style", "title": "Writing guide", "file": "style.md", "sha256": source_sha,
                                  "url": "https://example.com/style"}], "licenses": {"example/docs": {"file": "LICENSE.txt", "sha256": license_sha, "url": "https://example.com/license"}}}
    index_sha = store("sources/index.json", source_index)
    facts_sha = store("facts.md", "# Facts\nA fictional preview service.\n")
    store("preparation.json", {"source_index_sha256": index_sha, "facts_sha256": facts_sha})
    assessment = root.parent / "assessment.json"
    assessment.write_text(json.dumps({"method": "Synthetic integrity check", "summary": "No semantic judgment.", "notes": []}))
    def stage(name, missing=False):
        prefix = "ledger/stages/" + name
        prompt_sha = store(prefix + "/prompt.txt", "Write the guide.")
        store(prefix + "/inputs/facts.md", "# Facts\nA fictional preview service.\n")
        frozen = {"fingerprint": name + "-identity", "stage": {"id": name, "expected_outputs": ["README.md", "notes.md"]},
                  "input_sha256": {"facts.md": facts_sha}, "prompt_sha256": prompt_sha}
        store(prefix + "/frozen.json", frozen)
        store(prefix + "/started.json", {"fingerprint": frozen["fingerprint"], "started_at": "2026-09-10T21:00:00Z"})
        outputs = [{"path": "README.md", "error": "not found"}] if missing else [
            {"path": "README.md", "sha256": store(prefix + "/outputs/README.md", "# Retained guide\n") }]
        outputs.append({"path": "notes.md", "sha256": store(prefix + "/outputs/notes.md", "# Notes\nPreserved facts.\n")})
        store(prefix + "/result.json", {"fingerprint": frozen["fingerprint"], "agent": {"actual_model": "test-model", "duration_ms": 12,
                                                       "tools": ["Read"], "status": "interrupted" if missing else "completed"},
                                                "finished_at": "2026-09-10T21:00:01Z", "outputs": outputs})
    stage("draft")
    stage("references", missing=True)
    store("workspaces/references/README.md", "# Mutable workspace output must not fill a missing artifact.\n")
    return assessment


class ContextTransferBundleTest(unittest.TestCase):
    def test_missing_outputs_stay_missing_and_evidence_remains_accessible(self):
        with tempfile.TemporaryDirectory() as temporary:
            root, output = Path(temporary) / "run", Path(temporary) / "bundle"
            assessment = fixture(root)
            result = assemble(root, assessment, output)
            index, _, _ = load_index(output / "index.json")
            self.assertEqual(result["model_calls"], 0)
            reference_arm = next(arm for arm in index["adaptations"] if arm["id"] == "references")
            self.assertEqual(reference_arm["documents"], {})
            self.assertEqual(reference_arm["observation"]["status"], "interrupted")
            self.assertEqual(reference_arm["observation"]["model"], "test-model")
            self.assertIn("README.md: not found", reference_arm["notes"])
            self.assertTrue(any(item["id"].startswith("license-") for item in index["references"]))
            self.assertTrue(any(item["id"] == "references-notes" for item in index["references"]))
            self.assertTrue(any(item["id"] == "prompt-draft" for item in index["references"]))
            audit = json.loads((output / "audit.json").read_text())
            self.assertEqual(audit["stages"]["draft"]["started_at"], "2026-09-10T21:00:00Z")
            self.assertIn("facts.md", audit["stages"]["draft"]["input_sha256"])
            with self.assertRaises(FileExistsError):
                assemble(root, assessment, output)

    def test_tampered_sources_or_immutable_outputs_are_rejected(self):
        for path in ("sources/style.md", "sources/LICENSE.txt", "ledger/stages/draft/outputs/README.md", "ledger/stages/draft/inputs/facts.md"):
            with self.subTest(path=path), tempfile.TemporaryDirectory() as temporary:
                root, output = Path(temporary) / "run", Path(temporary) / "bundle"
                assessment = fixture(root)
                (root / path).write_text("tampered")
                with self.assertRaisesRegex(ValueError, "hash differs"):
                    assemble(root, assessment, output)

    def test_checkpoints_require_corresponding_immutable_outputs(self):
        for checkpoint in ("original", "learned"):
            with self.subTest(checkpoint=checkpoint), tempfile.TemporaryDirectory() as temporary:
                root, output = Path(temporary) / "run", Path(temporary) / "bundle"
                assessment = fixture(root)
                if checkpoint == "original":
                    directory = root / "original"
                    directory.mkdir()
                    missing = "docs/troubleshooting.md"
                    (directory / "docs").mkdir()
                    (directory / missing).write_text("Not retained by the draft stage.")
                    (directory / "identity.json").write_text(json.dumps({missing: digest((directory / missing).read_bytes())}))
                else:
                    directory = root / "frozen"
                    directory.mkdir()
                    (directory / "context.json").write_text("{}")
                    (directory / "learned-guide.md").write_text("# Unproven learned guide")
                    preparation = json.loads((root / "preparation.json").read_text())
                    (directory / "identity.json").write_text(json.dumps({"source_index_sha256": preparation["source_index_sha256"],
                        "context_sha256": digest((directory / "context.json").read_bytes()),
                        "guide_sha256": digest((directory / "learned-guide.md").read_bytes())}))
                with self.assertRaisesRegex(ValueError, "differs from immutable"):
                    assemble(root, assessment, output)

    def test_plain_context_must_match_actual_resolver_bytes(self):
        with tempfile.TemporaryDirectory() as temporary:
            root, output = Path(temporary) / "run", Path(temporary) / "bundle"
            assessment = fixture(root)
            actual = json.dumps({"point": {"ref": "pageglass/overview"}, "voice": {"guide": "# Actual guide"}}).encode()
            (root / "resolved").mkdir()
            (root / "resolved/overview.json").write_bytes(actual)
            (root / "resolved/index.json").write_text(json.dumps({"documents": [{"document": "README.md", "surface": "overview", "file": "overview.json", "sha256": digest(actual)}]}))
            stage = root / "ledger/stages/guidance"
            (stage / "inputs/context").mkdir(parents=True)
            changed = b'{"voice":{"guide":"Different context"}}'
            (stage / "inputs/context/overview.json").write_bytes(changed)
            (stage / "prompt.txt").write_text("Apply context.")
            (stage / "frozen.json").write_text(json.dumps({"fingerprint": "guidance-id", "stage": {"id": "guidance", "expected_outputs": []},
                                                        "input_sha256": {"context/overview.json": digest(changed)}, "prompt_sha256": digest(b"Apply context.")}))
            with self.assertRaisesRegex(ValueError, "Plain guidance input differs"):
                assemble(root, assessment, output)


if __name__ == "__main__":
    unittest.main()
