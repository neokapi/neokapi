"""Offline tests using a real local kapi binary; never call a model."""

import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from prepare_scoped_context import digest, isolated_environment, load_fixture, prepare, resolved_input, review_schedule


HERE = Path(__file__).resolve().parent
FIXTURE = HERE / "scoped-context"
BINARY = Path(os.environ.get("KAPI_SCOPED_TEST_BINARY", HERE.parents[1] / "bin" / "kapi"))


class ScopedPreparationUnitTest(unittest.TestCase):
    def test_isolation_overrides_user_project_and_plugin_locations(self):
        with tempfile.TemporaryDirectory() as temporary:
            with patch.dict(os.environ, {"KAPI_NO_PROJECT": "", "KAPI_PLUGINS_DIR_ONLY": "0", "KAPI_CONFIG_DIR": "/not-the-fixture"}):
                env = isolated_environment(Path(temporary))
            self.assertEqual(env["KAPI_NO_PROJECT"], "1")
            self.assertEqual(env["KAPI_PLUGINS_DIR_ONLY"], "1")
            for variable in ("KAPI_CONFIG_DIR", "XDG_DATA_HOME", "XDG_CACHE_HOME", "KAPI_PLUGINS_DIR"):
                self.assertTrue(Path(env[variable]).is_relative_to(Path(temporary).resolve()))
                self.assertTrue(Path(env[variable]).is_dir())

    def test_subject_guidance_comes_only_from_resolver(self):
        corpus = load_fixture(FIXTURE)
        case = corpus["cases"][0]
        actual = {"point": {"path": case["path"], "channel": "support"}, "scope": "project", "notes": ["resolver note"],
                  "voice": {"name": "Resolved profile", "source": "resolved.yaml", "field": "profiles.resolved.voice", "guide": "Only this returned text."}}
        subject = resolved_input(case, corpus["candidates"][case["candidate"]], actual)
        self.assertEqual(subject["sources"], [{"id": "resolved-voice", "text": actual["voice"]["guide"]}])
        self.assertNotIn("requirements", subject)
        self.assertEqual(subject["variables"]["resolved_context"]["point"], actual["point"])
        self.assertNotIn("expected_style", json.dumps(subject))


@unittest.skipUnless(BINARY.is_file(), "build bin/kapi or set KAPI_SCOPED_TEST_BINARY to run real resolver checks")
class ScopedPreparationRealBinaryTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="kapi-scoped-test-")
        cls.output = Path(cls.temporary.name) / "prepared"
        cls.summary = prepare(FIXTURE, BINARY, cls.output)
        cls.resolutions = json.loads((cls.output / "resolution.json").read_text())["cases"]
        cls.inputs = [json.loads(line) for line in (cls.output / "subject" / "inputs.jsonl").read_text().splitlines()]
        cls.labels = json.loads((cls.output / "labels.json").read_text())

    @classmethod
    def tearDownClass(cls):
        cls.temporary.cleanup()

    def test_same_candidates_receive_different_actual_channel_context(self):
        self.assertEqual(self.summary["model_calls"], 0)
        self.assertEqual(len(self.inputs), 6)
        by_variant = {}
        for subject in self.inputs:
            label = self.labels[subject["id"]]
            by_variant.setdefault(label["candidate"], []).append(subject)
            context = json.loads((self.output / "resolved" / label["case"] / "context.json").read_text())
            self.assertEqual(subject["sources"][0]["text"], context["voice"]["guide"])
            self.assertEqual(subject["variables"]["resolved_context"]["point"], context["point"])
            self.assertNotIn("beacon/status-celebration", subject["sources"][0]["text"])
            self.assertNotIn("requirements", subject)
        for pair in by_variant.values():
            self.assertEqual(len(pair), 2)
            self.assertEqual(pair[0]["candidate"], pair[1]["candidate"])
            self.assertNotEqual(pair[0]["sources"], pair[1]["sources"])
            self.assertEqual({case["variables"]["resolved_context"]["point"]["channel"] for case in pair}, {"support", "status"})
        for field in ("reader_task", "audience", "surface"):
            self.assertEqual(len({case[field] for case in self.inputs}), 1)
        manifest = json.loads((self.output / "subject" / "manifest.json").read_text())
        self.assertEqual(manifest["study"], "scoped-context-style")
        self.assertEqual(len(manifest["sessions"]), 6)
        self.assertEqual({session["protocol"] for session in manifest["sessions"]}, {"style"})
        self.assertEqual({path.name for path in (self.output / "subject").iterdir()}, {"inputs.jsonl", "manifest.json", "instruction.txt"})

    def test_check_coverage_and_raw_hashes_are_recorded(self):
        for result in self.resolutions:
            self.assertEqual(result["point"].get("profile", ""), result["check_context"]["voice"].get("profile", ""))
            self.assertEqual(result["point"].get("channel", ""), result["check_context"]["voice"].get("channel", ""))
            directory = self.output / "resolved" / result["case"]
            for operation in ("context", "check"):
                self.assertEqual(digest((directory / f"{operation}.json").read_bytes()), result[operation]["stdout_sha256"])
                self.assertEqual(digest((directory / f"{operation}.stderr.txt").read_bytes()), result[operation]["stderr_sha256"])
            if self.labels[result["id"]]["model_eligible"]:
                statuses = {analyzer["id"]: analyzer["status"] for analyzer in result["coverage"]}
                self.assertEqual(statuses["voice.guidance"], "unsupported")
                self.assertEqual(statuses["voice.llm"], "not_requested")
                self.assertEqual(result["check"]["exit_code"], 0)
        with self.assertRaises(FileExistsError):
            prepare(FIXTURE, BINARY, self.output)

    def test_calibration_schedule_keeps_paired_inputs_and_both_mismatch_directions(self):
        before = json.dumps(self.inputs)
        sessions = review_schedule(self.inputs, calibration_comparison=True)
        self.assertEqual(len(sessions), 6)
        self.assertEqual(sum(s["protocol"] == "style" for s in sessions), 2)
        cases_by_id = {case["id"]: self.labels[case["id"]]["case"] for case in self.inputs}
        controls = {cases_by_id[s["case_id"]] for s in sessions if s["protocol"] == "style"}
        calibrated = {cases_by_id[s["case_id"]] for s in sessions if s["protocol"] == "style-calibrated"}
        self.assertEqual(controls, {"harbor-support-paraphrase", "harbor-status-neutral"})
        self.assertEqual(calibrated - controls, {"harbor-support-neutral", "harbor-status-original"})
        self.assertEqual(json.dumps(self.inputs), before)

    def test_other_product_and_missing_voice_are_offline_controls(self):
        controls = json.loads((self.output / "offline-controls.json").read_text())
        self.assertEqual(len(controls), 2)
        unbound = next(case for case in controls if self.labels[case["id"]]["case"] == "unbound-original")
        context = unbound["variables"]["resolved_context"]
        self.assertNotIn("voice", context)
        self.assertTrue(context["point"]["default"])
        self.assertEqual(unbound["sources"][0]["id"], "resolved-context-notes")
        beacon = next(case for case in controls if self.labels[case["id"]]["case"] == "beacon-distraction")
        self.assertEqual(beacon["variables"]["resolved_context"]["point"]["profile"], "beacon")
        self.assertIn("beacon/status-celebration", beacon["sources"][0]["text"])
        scheduled = {case["id"] for case in self.inputs}
        self.assertTrue(all(case["id"] not in scheduled for case in controls))


if __name__ == "__main__":
    unittest.main()
