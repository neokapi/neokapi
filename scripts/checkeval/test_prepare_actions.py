"""Offline preparation checks; these do not establish semantic label accuracy."""

import json
import tempfile
import unittest
from pathlib import Path

from prepare_actions import digest, load_corpus, prepare


CORPUS = Path(__file__).with_name("action-coverage.json")


class ActionPreparationTest(unittest.TestCase):
    def test_bounded_schedule_keeps_labels_and_controls_outside_subject(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "prepared"
            summary = prepare(CORPUS, output)
            self.assertEqual(summary["model_calls"], 0)
            subject = output / "subject"
            self.assertEqual({p.name for p in subject.iterdir()}, {"manifest.json", "inputs.jsonl", "instruction.txt"})
            cases = [json.loads(line) for line in (subject / "inputs.jsonl").read_text().splitlines()]
            labels = json.loads((output / "labels.json").read_text())
            manifest = json.loads((subject / "manifest.json").read_text())
            self.assertEqual(len(cases), 3)
            self.assertEqual(len(labels), 9)
            self.assertEqual(sum(label["scheduled"] for label in labels.values()), 3)
            self.assertEqual(len(manifest["sessions"]), 6)
            self.assertEqual(manifest["study"], "action-coverage-comparison")
            self.assertEqual(manifest["billing"], "subscription-only")
            self.assertEqual(manifest["agents"], [{"host": "claude", "model": "claude-sonnet-5", "effort": "high"}])
            self.assertEqual([labels[case["id"]]["instruction_form"] for case in cases], ["explicit", "indirect", "omitted"])
            for index, case in enumerate(cases):
                self.assertEqual(set(case), {"id", "reader_task", "audience", "surface", "destination", "variables", "sources", "requirements", "candidate"})
                self.assertEqual(len(case["id"]), 16)
                self.assertTrue(labels[case["id"]]["scheduled"])
                for source in case["sources"]:
                    self.assertEqual(set(source), {"id", "text"})
                for requirement in case["requirements"]:
                    self.assertEqual(set(requirement), {"id", "description", "source_ids"})
                pair = manifest["sessions"][index * 2:index * 2 + 2]
                self.assertEqual([session["case_id"] for session in pair], [case["id"], case["id"]])
                expected = ["ordinary", "requirements"] if index % 2 == 0 else ["requirements", "ordinary"]
                self.assertEqual([session["protocol"] for session in pair], expected)
            provenance = json.loads((output / "provenance.json").read_text())
            self.assertEqual(provenance["corpus_sha256"], digest(CORPUS.read_bytes()))
            self.assertEqual(provenance["corpus"], json.loads(CORPUS.read_text()))
            self.assertIn("Unrun development control", (output / "review.html").read_text())
            with self.assertRaises(FileExistsError):
                prepare(CORPUS, output)

    def test_prepare_is_reproducible_before_any_subject_review(self):
        with tempfile.TemporaryDirectory() as temporary:
            first, second = Path(temporary) / "first", Path(temporary) / "second"
            prepare(CORPUS, first)
            prepare(CORPUS, second)
            for file in first.rglob("*"):
                if file.is_file():
                    self.assertEqual(file.read_bytes(), (second / file.relative_to(first)).read_bytes())

    def test_invalid_source_and_schedule_rejected(self):
        for mutation in ("unknown-source", "duplicate-form", "unbalanced-selection", "production-label"):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temporary:
                corpus = json.loads(CORPUS.read_text())
                family = corpus["families"][0]
                if mutation == "unknown-source":
                    family["requirements"][0]["source_ids"] = ["invented"]
                elif mutation == "duplicate-form":
                    family["variants"][1]["instruction_form"] = "explicit"
                elif mutation == "unbalanced-selection":
                    family["scheduled_variant"] = family["id"] + "-omitted"
                else:
                    family["variants"][0]["expected"]["label_status"] = "human_verified"
                path = Path(temporary) / "invalid.json"
                path.write_text(json.dumps(corpus))
                with self.assertRaises(ValueError):
                    load_corpus(path)


if __name__ == "__main__":
    unittest.main()
