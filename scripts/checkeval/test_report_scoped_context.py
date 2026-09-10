"""Keep report evidence tied to its candidate without asserting style judgments."""

import copy
import json
import unittest

from report_scoped_context import digest, validate_style_identity


class StyleIdentityTest(unittest.TestCase):
    def setUp(self):
        self.case = {"id": "a", "candidate": "Café", "sources": [{"id": "resolved-voice", "text": "Be warm."}], "variables": {"resolved_context": {"point": {"channel": "support"}}}}
        candidate = [{"id": "c1", "text": "Café", "start": 0, "end": 5}]
        guide = [{"id": "g1", "text": "Be warm.", "start": 0, "end": 8}]
        snapshot = {"input": {**self.case, "candidate_spans": candidate, "guidance_spans": guide}}
        evidence = json.dumps(snapshot)
        self.style = {"request_id": "a", "request_fingerprint": digest(evidence.encode()), "evidence": evidence,
                      "candidate_spans": candidate, "guidance_spans": guide, "findings": [], "suggestions": []}

    def test_matching_snapshot_and_utf8_bytes_are_accepted(self):
        validate_style_identity(self.style, self.case)

    def test_another_destination_cannot_reuse_an_accepted_result(self):
        other = copy.deepcopy(self.case)
        other["variables"]["resolved_context"]["point"]["channel"] = "status"
        with self.assertRaisesRegex(ValueError, "different frozen input"):
            validate_style_identity(self.style, other)

    def test_changed_selected_passage_is_rejected(self):
        self.style["findings"] = [{"candidate_ids": ["c1"], "guidance_ids": ["g1"],
                                   "candidate_spans": [{"id": "c1", "text": "different", "start": 0, "end": 5}],
                                   "guidance_spans": self.style["guidance_spans"]}]
        with self.assertRaisesRegex(ValueError, "Selected style evidence differs"):
            validate_style_identity(self.style, self.case)

    def test_changed_snapshot_fingerprint_is_rejected(self):
        self.style["request_fingerprint"] = "wrong"
        with self.assertRaisesRegex(ValueError, "identity differs"):
            validate_style_identity(self.style, self.case)


if __name__ == "__main__":
    unittest.main()
