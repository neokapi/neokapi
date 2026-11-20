import hashlib
import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


root = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location(
    "audit_format", root / ".skills/refresh-format-maturity/scripts/audit-format.py"
)
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


def digest(checks):
    payload = json.dumps(checks, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    return hashlib.sha256(payload).hexdigest()


class ReceiptTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        scope = patch.object(audit, "REPO", self.root)
        scope.start()
        self.addCleanup(scope.stop)
        self.git("init", "-q")
        self.git("config", "user.name", "Audit")
        self.git("config", "user.email", "audit@example.invalid")
        self.git("config", "commit.gpgsign", "false")
        model = self.root / "core/model/block.go"
        model.parent.mkdir(parents=True)
        model.write_text("package model\n")
        self.git("add", "core")
        self.git("commit", "-qm", "Record source")
        checks = [{"test": "TestAcceptance/consumer", "outcome": "pass"}]
        self.receipt = {
            "check": "format-acceptance", "format": "xliff", "environment": "github-actions",
            "checks": checks, "exit": 0, "output_sha": digest(checks),
            "workflow": ".github/workflows/format-acceptance.yml", "run_id": 123, "run_attempt": 1,
            "head_sha": self.git("rev-parse", "HEAD"), "conclusion": "success", "test_outcome": "pass",
        }

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, text=True).strip()

    def result(self, *receipts, fmt="xliff", check="acceptance"):
        # Each call represents a new audit process after a possible checkout change.
        audit._tested_inputs_match.cache_clear()
        return audit._ledger_check({"runs": [{"evidence": list(receipts)}]}, fmt, check)

    def test_commands_resolve_the_exact_subject_and_checker(self):
        for command, check, subject, neighboring in [
            ("node scripts/format-ops/check-citations.mjs androidxml --json", "citations", "androidxml", "xml"),
            ("node scripts/format-ops/context-pack.mjs xliff2 --out result", "context-pack", "xliff2", "xliff"),
            ("structure-certification pdf", "structure", "pdf", "odf"),
        ]:
            with self.subTest(command=command):
                evidence = {"check": command, "exit": 0, "output_sha": "a" * 64}
                self.assertEqual("green", self.result(evidence, fmt=subject, check=check))
                self.assertEqual("unknown", self.result(evidence, fmt=neighboring, check=check))
                self.assertEqual("unknown", self.result({**evidence, "format": "xml"}, fmt=subject, check=check))
                self.assertEqual("unknown", self.result({**evidence, "check": "echo " + command}, fmt=subject, check=check))
                self.assertEqual("red", self.result(evidence, {**evidence, "exit": 1}, fmt=subject, check=check))

    def test_local_or_unscoped_acceptance_cannot_establish_ci(self):
        self.assertEqual("unknown", self.result({"check": "make format-acceptance", "format": "xliff", "environment": "local", "exit": 0}))
        for invalid in [
            {"format": "xliff2"}, {"workflow": "nightly.yml"}, {"run_id": 0}, {"output_sha": "missing"},
            {"environment": "local"}, {"environment": None}, {"environment": "other"}, {"checks": []}, {"checks": [{"test": "TestAcceptance", "outcome": "skip"}]},
            {"head_sha": "f" * 40}, {"head_sha": []}, {"conclusion": "cancelled"}, {"run_attempt": 0},
            {"run_id": "123"}, {"output_sha": "a" * 64},
        ]:
            with self.subTest(invalid=invalid):
                self.assertEqual("unknown", self.result({**self.receipt, **invalid}))

    def test_newer_skipped_partial_and_failed_results_supersede_success(self):
        for outcome, cases, conclusion, status, expected in [
            ("skip", ["skip"], "success", 0, "unknown"),
            ("partial", ["pass", "skip"], "success", 0, "unknown"),
            ("fail", ["pass", "fail"], "failure", 1, "red"),
            ("fail", [], "failure", 1, "red"),
        ]:
            with self.subTest(outcome=outcome):
                latest = {**self.receipt, "test_outcome": outcome, "conclusion": conclusion, "exit": status,
                          "checks": [{"test": "TestAcceptance/" + value, "outcome": value} for value in cases]}
                latest["output_sha"] = digest(latest["checks"])
                self.assertEqual(expected, self.result(self.receipt, latest))

    def test_receipts_survive_report_commits_and_expire_when_tested_inputs_change(self):
        self.assertEqual("green", self.result(self.receipt))
        (self.root / "dashboard.json").write_text("{}")
        self.git("add", "dashboard.json")
        self.git("commit", "-qm", "Record observation")
        self.assertEqual("green", self.result(self.receipt))
        for change in ("core/model/block.go", "core/format/new.go", "go.mod", ".github/workflows/format-acceptance.yml"):
            with self.subTest(change=change):
                candidate = self.root / change
                candidate.parent.mkdir(parents=True, exist_ok=True)
                previous = candidate.read_bytes() if candidate.exists() else None
                candidate.write_bytes(b"changed\n")
                self.assertEqual("unknown", self.result(self.receipt))
                if previous is None:
                    candidate.unlink()
                else:
                    candidate.write_bytes(previous)


if __name__ == "__main__":
    unittest.main()
