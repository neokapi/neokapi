"""Offline invariants for the independent-draft context-transfer protocol."""
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("prepare", Path(__file__).with_name("prepare_context_transfer.py"))
PREP = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PREP)


class PreparationTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "workspaces/learn").mkdir(parents=True)
        (self.root / "sources").mkdir()
        (self.root / "sources/guide.md").write_text("Reference guide.")
        (self.root / "sources/index.json").write_text(json.dumps({"sources": [
            {"id": "guide", "file": "guide.md", "sha256": PREP.digest(b"Reference guide.")}]}))
        (self.root / "facts.md").write_text("Fictional product facts only.\n")
        self.learned = {"title": "Guide", "summary": "Learned summary", "non_transferable": ["Source APIs"],
                        "uncertainties": ["Examples are narrow."], "rules": [
                            {"id": "options", "statement": "Keep optional actions optional.", "basis": "explicit",
                             "surfaces": ["tutorial", "explanation"],
                             "sources": [{"id": "guide", "section": "Options"}]}]}
        (self.root / "workspaces/learn/context.json").write_text(json.dumps(self.learned))
        PREP.write(self.root / "preparation.json", {
            "source_index_sha256": PREP.digest((self.root / "sources/index.json").read_bytes()),
            "facts_sha256": PREP.digest((self.root / "facts.md").read_bytes())})
        self.record_stage("learn", ["context.json"])
        fixture = self.root / "fixture"
        fixture.mkdir()
        (fixture / "draft.txt").write_text("Write from facts.md.")
        fixture_patch = patch.object(PREP, "FIXTURE", fixture)
        fixture_patch.start()
        self.addCleanup(fixture_patch.stop)

    def record_stage(self, identifier, documents):
        stage = self.root / "ledger/stages" / identifier
        stage.mkdir(parents=True, exist_ok=True)
        records = []
        for document in documents:
            data = (self.root / "workspaces" / identifier / document).read_bytes()
            output = stage / "outputs" / document
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_bytes(data)
            records.append({"path": document, "sha256": PREP.digest(data), "bytes": len(data)})
        for filename, value in {
            "frozen.json": {"fingerprint": "frozen-identity", "stage": {"id": identifier, "expected_outputs": documents}},
            "started.json": {"fingerprint": "frozen-identity", "prepared": {"launch": {"agent": {"model": "claude-sonnet-5"}}}},
            "result.json": {"fingerprint": "frozen-identity", "agent": {"status": "completed", "protocol_completed": True,
                "actual_model": "claude-sonnet-5", "requested_model": "claude-sonnet-5"}, "outputs": records},
        }.items():
            (stage / filename).write_text(json.dumps(value))

    def test_learning_precedes_fact_only_draft(self):
        with self.assertRaises(FileNotFoundError):
            PREP.prepare_draft(self.root)
        PREP.freeze_context(self.root)
        original = (self.root / "workspaces/learn/context.json").read_bytes()
        self.assertEqual(original, (self.root / "frozen/context.json").read_bytes())
        PREP.prepare_draft(self.root)
        files = [p.name for p in (self.root / "workspaces/draft").iterdir()]
        self.assertEqual(files, ["facts.md"])
        stage = json.loads((self.root / "stages/draft.json").read_text())
        self.assertEqual(stage["input_files"], ["facts.md"])

    def test_changed_frozen_context_blocks_draft(self):
        PREP.freeze_context(self.root)
        (self.root / "frozen/context.json").write_text("{}")
        with self.assertRaisesRegex(ValueError, "Retained evidence changed"):
            PREP.prepare_draft(self.root)
        self.assertFalse((self.root / "workspaces/draft").exists())

    def test_compiler_preserves_qualified_statement_and_scopes(self):
        profile = PREP.compile_profile(self.learned)
        constraints = profile["constraints"]
        self.assertEqual(len(constraints), 2)
        self.assertEqual({c["scope"]["channel"] for c in constraints}, {"tutorial", "explanation"})
        for constraint in constraints:
            self.assertEqual(constraint["statement"], "[explicit] Keep optional actions optional.")
            self.assertEqual(constraint["kind"], "guidance")
            self.assertEqual(constraint["source"], "guide: Options")
        self.assertIn("Examples are narrow.", profile["description"])
        self.assertIn("Source APIs", profile["description"])

    def test_unknown_source_is_not_frozen(self):
        self.learned["rules"][0]["sources"][0]["id"] = "invented"
        (self.root / "workspaces/learn/context.json").write_text(json.dumps(self.learned))
        self.record_stage("learn", ["context.json"])
        with self.assertRaisesRegex(ValueError, "Unknown"):
            PREP.freeze_context(self.root)
        self.assertFalse((self.root / "frozen").exists())

    def test_changed_workspace_or_retained_output_blocks_freeze(self):
        for path in ("workspaces/learn/context.json", "ledger/stages/learn/outputs/context.json"):
            with self.subTest(path=path):
                target = self.root / path
                original = target.read_bytes()
                target.write_bytes(original + b" ")
                with self.assertRaises(ValueError):
                    PREP.freeze_context(self.root)
                self.assertFalse((self.root / "frozen").exists())
                target.write_bytes(original)

    def test_invalid_host_or_identity_blocks_freeze(self):
        path = self.root / "ledger/stages/learn/result.json"
        original = path.read_bytes()
        mutations = [
            {"status": "route_violation"}, {"status": "agent_failed"},
            {"protocol_completed": False}, {"actual_model": "different-model"}, {"rate_limited": True},
        ]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                value = json.loads(original)
                value["agent"].update(mutation)
                path.write_text(json.dumps(value))
                with self.assertRaisesRegex(ValueError, "eligible"):
                    PREP.freeze_context(self.root)
                self.assertFalse((self.root / "frozen").exists())
        value = json.loads(original)
        value["fingerprint"] = "different"
        path.write_text(json.dumps(value))
        with self.assertRaisesRegex(ValueError, "identity"):
            PREP.freeze_context(self.root)

    def test_changed_guide_sources_and_brief_block_staging(self):
        PREP.freeze_context(self.root)
        for path in ("frozen/learned-guide.md", "sources/guide.md", "sources/index.json", "facts.md"):
            with self.subTest(path=path):
                target = self.root / path
                original = target.read_bytes()
                target.write_bytes(original + b"changed")
                with self.assertRaisesRegex(ValueError, "Retained evidence changed"):
                    PREP.prepare_draft(self.root)
                with self.assertRaisesRegex(ValueError, "Retained evidence changed"):
                    PREP.prepare_adaptations(self.root, Path("unused-kapi"))
                self.assertFalse((self.root / "workspaces/draft").exists())
                target.write_bytes(original)

    def test_changed_draft_is_rejected_before_adaptation_outputs_exist(self):
        PREP.freeze_context(self.root)
        PREP.prepare_draft(self.root)
        for document in PREP.DOCUMENTS:
            path = self.root / "workspaces/draft" / document
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("Independent draft.")
        self.record_stage("draft", list(PREP.DOCUMENTS))
        (self.root / "workspaces/draft/README.md").write_text("Convenient injected defect.")
        with self.assertRaisesRegex(ValueError, "Workspace output differs"):
            PREP.prepare_adaptations(self.root, Path("unused-kapi"))
        self.assertFalse((self.root / "original").exists())
        self.assertFalse((self.root / "workspaces/references").exists())


if __name__ == "__main__":
    unittest.main()
