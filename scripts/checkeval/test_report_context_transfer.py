"""Integrity and safe standard-Markdown rendering checks; no model calls."""

import json
import os
import tempfile
import unittest
from pathlib import Path

from report_context_transfer import DOCUMENTS, digest, exact_diff, load_index, render


REPO = Path(os.environ.get("CONTEXT_TRANSFER_REPORT_REPO", Path(__file__).resolve().parents[2]))


def sample_index(directory):
    def artifact(name, text):
        path = directory / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
        return {"path": name, "sha256": digest(path.read_bytes())}
    body = "# Preview a project\n\nUse **Preview** to inspect a deployment.\n\n- Open the project.\n- Copy the preview link.\n\n```sh\nastro dev\n```\n\n| Setting | Value |\n| --- | --- |\n| Access | Team |\n\n[Getting started](docs/getting-started.md)\n"
    index = {"title": "Context transfer comparison", "description": "Synthetic rendering fixture; no model output.",
             "brief": artifact("brief.md", "# The brief\n\nMake the guide easy to use."),
             "references": [{"id": "astro", "title": "Reference source", "url": "https://example.com/docs/",
                             "artifact": artifact("references/source.mdx", "<Aside>Retained MDX source</Aside>\n")}],
             "guidance": [{"title": "Learned guidance", "artifact": artifact("guidance.md", "# Guidance\n\nUse concrete verbs.")}],
             "documents": [{"id": doc, "title": doc} for doc in sorted(DOCUMENTS)],
             "original": {"title": "Original", "documents": {}, "observation": {"status": "retained", "tools": []}},
             "adaptations": [], "assessment": {"method": "Synthetic test", "summary": "No semantic assessment.", "notes": []}}
    for doc in DOCUMENTS:
        index["original"]["documents"][doc] = artifact("original/" + doc, body)
    for name, title in [("references", "Reference docs"), ("guidance", "Compiled guidance in plain files"), ("kapi", "Compiled guidance through kapi")]:
        arm = {"id": name, "title": title, "documents": {}, "observation": {"status": "completed", "model": "synthetic", "duration_ms": 1250, "tools": ["Read"]}, "notes": []}
        for doc in DOCUMENTS:
            arm["documents"][doc] = artifact(name + "/" + doc, body.replace("inspect a deployment", "check the deployment"))
        index["adaptations"].append(arm)
    path = directory / "index.json"
    path.write_text(json.dumps(index), encoding="utf-8")
    return path, index


class ContextTransferIntegrityTest(unittest.TestCase):
    def test_hash_mismatch_rejected_in_any_artifact_role(self):
        for role in ("brief", "reference", "guidance", "document"):
            with self.subTest(role=role), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                path, index = sample_index(root)
                artifact = {"brief": index["brief"], "reference": index["references"][0]["artifact"],
                            "guidance": index["guidance"][0]["artifact"], "document": index["adaptations"][0]["documents"]["README.md"]}[role]
                (root / artifact["path"]).write_text("Changed after the recorded run.")
                with self.assertRaisesRegex(ValueError, "hash differs"):
                    load_index(path)

    def test_paths_cannot_escape_snapshot_root(self):
        for mode in ("relative", "absolute", "symlink"):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as temporary:
                parent = Path(temporary)
                root = parent / "snapshot"
                root.mkdir()
                path, index = sample_index(root)
                outside = parent / "outside.md"
                outside.write_text("outside")
                if mode == "symlink":
                    (root / "link.md").symlink_to(outside)
                target = "../outside.md" if mode == "relative" else str(outside) if mode == "absolute" else "link.md"
                index["brief"] = {"path": target, "sha256": digest(outside.read_bytes())}
                path.write_text(json.dumps(index))
                with self.assertRaises(ValueError):
                    load_index(path)

    def test_exact_diff_records_missing_final_newline(self):
        difference = exact_diff("first\nsecond\n", "first\nchanged", "README.md", "kapi")
        self.assertIn("-second\n+changed\n\\ No newline at end of file\n", difference)
        self.assertEqual(exact_diff("same", "same", "README.md", "kapi"), "")

    @unittest.skipUnless((REPO / "node_modules").is_dir(), "install repo JS dependencies or set CONTEXT_TRANSFER_REPORT_REPO")
    def test_standard_markdown_is_rendered_and_unsafe_content_is_inert(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            path, index = sample_index(root)
            artifact = index["adaptations"][0]["documents"]["README.md"]
            raw = (root / artifact["path"]).read_text()
            raw += '\n<script>window.bad=true</script>\n\n[bad](javascript:alert(1))\n\n![remote](https://example.com/tracker.png)\n'
            (root / artifact["path"]).write_text(raw)
            artifact["sha256"] = digest((root / artifact["path"]).read_bytes())
            index["adaptations"][2]["documents"].pop("docs/troubleshooting.md")
            index["adaptations"][2]["observation"]["status"] = "interrupted"
            path.write_text(json.dumps(index))
            output = root / "review.html"
            result = render(path, output, REPO)
            page = output.read_text()
            self.assertEqual(result["model_calls"], 0)
            self.assertIn("<h1>Preview a project</h1>", page)
            self.assertIn("<strong>Preview</strong>", page)
            self.assertIn("<table>", page)
            self.assertIn('<code class="language-sh">', page)
            self.assertNotIn("<script>window.bad=true</script>", page)
            self.assertNotIn('href="javascript:', page)
            self.assertNotIn('<img ', page)
            self.assertIn("No artifact retained for this document", page)
            self.assertIn("interrupted", page)
            self.assertIn("Full retained reference source", page)
            self.assertIn("&lt;Aside&gt;Retained MDX source&lt;/Aside&gt;", page)
            self.assertIn("Exact Markdown diff", page)
            self.assertIn("Verified artifact provenance", page)


if __name__ == "__main__":
    unittest.main()
