"""Offline evidence-preparation checks; no judgments of model output."""

import json
import shutil
import tempfile
import unittest
from pathlib import Path

import prepare_actions
import prepare_documents
from prepare_evidence import TARGETS, prepare, prepared_bundle, read


HERE = Path(__file__).parent
ACTIONS = HERE / "action-coverage.json"
DOCUMENTS = HERE / "contextual-documents"
REPO = HERE.resolve().parents[1]


class EvidencePreparationTest(unittest.TestCase):
    def test_selected_cases_preserve_frozen_evidence_and_labels(self):
        with tempfile.TemporaryDirectory() as temporary:
            temporary = Path(temporary)
            actions, documents, output = temporary / "actions", temporary / "documents", temporary / "result"
            prepare_actions.prepare(ACTIONS, actions)
            prepare_documents.prepare(DOCUMENTS, documents, REPO, requirements_comparison=True)
            originals = {"actions": prepared_bundle(actions), "documents": prepared_bundle(documents)}
            summary = prepare(ACTIONS, DOCUMENTS, output, REPO)
            self.assertEqual(summary["model_calls"], 0)
            selected = prepared_bundle(output)
            self.assertEqual(len(selected["inputs"]), 3)
            self.assertEqual(len(selected["labels"]), 3)
            for corpus, variant, reason in TARGETS:
                original = originals[corpus]
                case_id = next(key for key, label in original["labels"].items() if label["variant"] == variant)
                self.assertEqual(selected["inputs"][case_id], original["inputs"][case_id])
                expected_label = {**original["labels"][case_id], "selection_reason": reason}
                self.assertEqual(selected["labels"][case_id], expected_label)
            for corpus in originals:
                self.assertEqual(selected["provenance"]["source_preparations"][corpus], originals[corpus]["provenance"])
            self.assertEqual((output / "subject" / "instruction.txt").read_text(), prepare_actions.INSTRUCTION)
            self.assertEqual({p.name for p in (output / "subject").iterdir()}, {"inputs.jsonl", "manifest.json", "instruction.txt"})
            for case in selected["inputs"].values():
                self.assertEqual(set(case), {"id", "reader_task", "audience", "surface", "destination", "variables", "sources", "requirements", "candidate"})
            manifest = read(output / "subject" / "manifest.json")
            self.assertEqual(manifest["study"], "contextual-evidence-comparison")
            self.assertEqual(manifest["billing"], "subscription-only")
            self.assertEqual(manifest["agents"], [{"host": "claude", "model": "claude-sonnet-5", "effort": "high"}])
            self.assertEqual(len(manifest["sessions"]), 6)
            for index, case_id in enumerate(selected["inputs"]):
                pair = manifest["sessions"][index * 2:index * 2 + 2]
                self.assertEqual([session["case_id"] for session in pair], [case_id, case_id])
                protocols = ["requirements", "anchored"] if index % 2 == 0 else ["anchored", "requirements"]
                self.assertEqual([session["protocol"] for session in pair], protocols)
            self.assertIn("targeted development regression", (output / "review.html").read_text())
            with self.assertRaises(FileExistsError):
                prepare(ACTIONS, DOCUMENTS, output, REPO)

    def test_repository_source_hash_verification_is_retained(self):
        with tempfile.TemporaryDirectory() as temporary:
            temporary = Path(temporary)
            repo = temporary / "repo"
            paths = set()
            for corpus in DOCUMENTS.glob("*.json"):
                for family in read(corpus)["families"]:
                    paths.update(source["path"] for source in family["sources"])
            for path in paths:
                target = repo / path
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(REPO / path, target)
            changed = repo / sorted(paths)[0]
            changed.write_text(changed.read_text() + "\nSource changed after fixture freeze.\n")
            output = temporary / "result"
            with self.assertRaisesRegex(ValueError, "Source blob changed"):
                prepare(ACTIONS, DOCUMENTS, output, repo)
            self.assertFalse(output.exists())

    def test_reproducible_preparation(self):
        with tempfile.TemporaryDirectory() as temporary:
            first, second = Path(temporary) / "first", Path(temporary) / "second"
            prepare(ACTIONS, DOCUMENTS, first, REPO)
            prepare(ACTIONS, DOCUMENTS, second, REPO)
            for path in first.rglob("*"):
                if path.is_file():
                    self.assertEqual(path.read_bytes(), (second / path.relative_to(first)).read_bytes())


if __name__ == "__main__":
    unittest.main()
