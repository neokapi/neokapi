"""Fixture and evidence-assertion tests; the CLI integration is run.py itself."""

import importlib.util
from pathlib import Path
import tempfile
import unittest
import xml.etree.ElementTree as ET
import zipfile


spec = importlib.util.spec_from_file_location("semantic_edit_run", Path(__file__).with_name("run.py"))
demo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(demo)


class EvidenceTests(unittest.TestCase):
    def test_fixture_docx_is_deterministic_and_has_native_headings_and_numbering(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            first, second = root / "a.docx", root / "b.docx"
            demo.make_docx(first)
            demo.make_docx(second)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            with zipfile.ZipFile(first) as archive:
                for name in archive.namelist():
                    if name.endswith((".xml", ".rels")):
                        ET.fromstring(archive.read(name))
                doc = ET.fromstring(archive.read("word/document.xml"))
                ns = {"w": demo.W}
                headings = [p for p in doc.findall(".//w:p", ns) if p.find("w:pPr/w:outlineLvl", ns) is not None]
                self.assertEqual(["".join(p.itertext()) for p in headings], ["Pageglass", "Local preview", demo.TITLE, "Things to remember", "Retention"])
                self.assertEqual(len(doc.findall(".//w:numPr", ns)), 2)
                self.assertIn(b'numId="1"', archive.read("word/numbering.xml"))
                self.assertIn("custom/preserved.bin", archive.namelist())

    def test_plan_replay_uses_original_byte_offsets_with_length_changes(self):
        with tempfile.TemporaryDirectory() as temp:
            file = Path(temp) / "fixture.md"
            source = "prefix é first middle second suffix".encode()
            file.write_bytes(source)
            patches = []
            for before, replacement in [("first", "a much longer paragraph"), ("second", "x")]:
                start = source.index(before.encode())
                patches.append({"start": start, "end": start + len(before), "before": before, "replacement": replacement})
            plan = {"snapshot": demo.digest(source), "patches": patches}
            self.assertEqual(demo.replay_plan(file, plan)[""], "prefix é a much longer paragraph middle x suffix".encode())
            plan["patches"][1]["before"] = "wrong!"
            with self.assertRaisesRegex(AssertionError, "before text differs"):
                demo.replay_plan(file, plan)

    def test_preservation_detects_unrelated_edits(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            demo.create_project(root / "project")
            for extension in ["md", "html", "docx"]:
                with self.subTest(format=extension):
                    before = root / "project" / "documents" / ("sharing." + extension)
                    after = root / ("changed." + extension)
                    if extension == "docx":
                        with zipfile.ZipFile(before) as old, zipfile.ZipFile(after, "w") as new:
                            for name in old.namelist():
                                content = old.read(name)
                                if name == "custom/preserved.bin":
                                    content += b"unexpected modification"
                                new.writestr(name, content)
                    else:
                        after.write_bytes(before.read_bytes().replace(demo.AFTER.encode(), b"Changed retention policy"))
                    with self.assertRaises(AssertionError):
                        demo.preserve(before, after)

    def test_scope_and_finding_coverage_cannot_be_confused_with_pass(self):
        report = {"schema": "kapi.check/v1", "pass": True, "findings": [], "execution": {"analyzers": [{"id": "voice.rules", "status": "not_requested"}, {"id": "voice.guidance", "status": "unsupported"}]}}
        with self.assertRaises(AssertionError):
            demo.assert_check(report, 0)
        report["execution"]["analyzers"][0]["status"] = "passed"
        demo.assert_check(report, 0)

    def test_native_range_rejects_different_hashes_and_outside_findings(self):
        heading = {"id": "h", "content_hash": "heading-hash"}
        body = {"id": "p", "name": "sharing/p", "content_hash": "paragraph-hash"}
        document = {"content_format": "markdown", "sections": [{"id": "h", "title": demo.TITLE, "level": 2, "range": {"heading": heading, "body": [body]}}]}
        ordinary = [{**heading, "role": "heading", "level": 2, "text": demo.TITLE}, {"id": "p", "content_hash": "paragraph-hash", "text": "Utilize the link ID."}]
        check = {"findings": [{"location": {"block": "sharing/p", "snippet": "Utilize"}}]}
        demo.assert_range(document, ordinary, check)
        body["content_hash"] = "stale-hash"
        with self.assertRaisesRegex(AssertionError, "content hash"):
            demo.assert_range(document, ordinary, check)
        body["content_hash"] = "paragraph-hash"
        check["findings"][0]["location"]["block"] = "unrelated/p"
        with self.assertRaisesRegex(AssertionError, "one block"):
            demo.assert_range(document, ordinary, check)


if __name__ == "__main__":
    unittest.main()
