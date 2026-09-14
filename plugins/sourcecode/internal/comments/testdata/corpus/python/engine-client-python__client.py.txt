#!/usr/bin/env python3
"""EngineService smoke-test client (Python).

Starts `kapi engine serve`, then drives the full contract against it:

  1. Extract  — stream messages.json in chunks, collect the Part stream
  2. Merge    — identity round-trip: the merged bytes must equal the source
                byte-for-byte
  3. Process  — pseudo-translate the parts (target locale qps)
  4. Merge    — write the pseudo-translated document
  5. Parity   — run the same pipeline through the kapi CLI
                (`kapi pseudo-translate`) and assert the engine output is
                byte-identical

gRPC stubs are generated at run time with grpc_tools from the .proto files
in the repo (core/proto/engine/v1 + core/proto/content/v1) — the .proto
files are the contract.

Environment:
  KAPI_BIN   path to the kapi binary (default: "kapi" on PATH)
  REPO_ROOT  repo root holding core/proto/ (default: ../../ from this file)
"""

import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = Path(os.environ.get("REPO_ROOT", HERE.parent.parent))
KAPI_BIN = os.environ.get("KAPI_BIN", "kapi")
FIXTURE = HERE / "messages.json"


def generate_stubs(out_dir: Path):
    """Generate Python gRPC stubs from the checked-in .proto files."""
    from grpc_tools import protoc

    args = [
        "protoc",
        f"-I{REPO_ROOT}",
        f"--python_out={out_dir}",
        f"--grpc_python_out={out_dir}",
        str(REPO_ROOT / "core/proto/content/v1/content.proto"),
        str(REPO_ROOT / "core/proto/engine/v1/engine.proto"),
    ]
    if protoc.main(args) != 0:
        raise SystemExit("protoc stub generation failed")
    sys.path.insert(0, str(out_dir))


def start_server(env):
    """Spawn `kapi engine serve` and parse its one-line JSON handshake."""
    proc = subprocess.Popen(
        [KAPI_BIN, "engine", "serve"],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        env=env,
    )
    line = proc.stdout.readline().decode()
    handshake = json.loads(line)
    return proc, handshake["socket"]


def chunks(data: bytes, size: int = 64 * 1024):
    for i in range(0, len(data), size):
        yield data[i : i + size]


def main() -> int:
    gen_dir = Path(tempfile.mkdtemp(prefix="kapi-engine-stubs-"))
    workdir = Path(tempfile.mkdtemp(prefix="kapi-engine-py-"))
    try:
        generate_stubs(gen_dir)
        import grpc  # noqa: E402  (after stub generation)
        from core.proto.engine.v1 import engine_pb2, engine_pb2_grpc
        from core.proto.content.v1 import content_pb2

        env = dict(os.environ)
        proc, socket_path = start_server(env)
        try:
            channel = grpc.insecure_channel(f"unix://{socket_path}")
            grpc.channel_ready_future(channel).result(timeout=15)
            client = engine_pb2_grpc.EngineServiceStub(channel)

            source = FIXTURE.read_bytes()

            # 1. Extract: header, then document chunks.
            def extract_requests():
                yield engine_pb2.ExtractRequest(
                    header=engine_pb2.ExtractHeader(
                        name="messages.json", source_locale="en"
                    )
                )
                for c in chunks(source):
                    yield engine_pb2.ExtractRequest(
                        chunk=engine_pb2.DocumentChunk(data=c)
                    )

            parts = []
            for resp in client.Extract(extract_requests()):
                if resp.HasField("parts"):
                    parts.extend(resp.parts.parts)
            assert parts, "extract returned no parts"

            # 2. Identity merge: byte-exact round trip.
            def merge_requests(merged_parts, target_locale=""):
                yield engine_pb2.MergeRequest(
                    header=engine_pb2.MergeHeader(
                        name="messages.json",
                        original=content_pb2.ContentRef(inline=source),
                        target_locale=target_locale,
                    )
                )
                yield engine_pb2.MergeRequest(
                    parts=engine_pb2.PartBatch(parts=merged_parts)
                )

            identity = b"".join(
                r.chunk.data for r in client.Merge(merge_requests(parts))
                if r.HasField("chunk")
            )
            assert identity == source, (
                "identity extract→merge is not byte-exact:\n"
                f"--- source ---\n{source!r}\n--- merged ---\n{identity!r}"
            )
            print("ok: identity round-trip is byte-exact")

            # 3. Process: pseudo-translate to qps.
            def process_requests():
                yield engine_pb2.ProcessRequest(
                    header=engine_pb2.ProcessHeader(
                        tools=[engine_pb2.ToolSpec(tool="pseudo-translate")],
                        source_locale="en",
                        target_locale="qps",
                    )
                )
                yield engine_pb2.ProcessRequest(
                    parts=engine_pb2.PartBatch(parts=parts)
                )

            processed = []
            for resp in client.Process(process_requests()):
                if resp.HasField("parts"):
                    processed.extend(resp.parts.parts)
            assert processed, "process returned no parts"

            # 4. Merge the pseudo-translated parts.
            engine_out = b"".join(
                r.chunk.data
                for r in client.Merge(merge_requests(processed, "qps"))
                if r.HasField("chunk")
            )
            assert engine_out != source, "pseudo-translate produced no change"
            print("ok: pseudo-translate + merge produced a localized document")

            # 5. CLI parity: the same pipeline through the kapi CLI must be
            #    byte-identical.
            fixture_copy = workdir / "messages.json"
            fixture_copy.write_bytes(source)
            cli_out = workdir / "cli-out.json"
            subprocess.run(
                [
                    KAPI_BIN,
                    "pseudo-translate",
                    str(fixture_copy),
                    "-o",
                    str(cli_out),
                    "--target-lang",
                    "qps",
                ],
                check=True,
                env=env,
                stdout=subprocess.DEVNULL,
            )
            cli_bytes = cli_out.read_bytes()
            assert engine_out == cli_bytes, (
                "engine output differs from CLI output:\n"
                f"--- engine ---\n{engine_out.decode()}\n"
                f"--- cli ---\n{cli_bytes.decode()}"
            )
            print("ok: engine output is byte-identical to the CLI output")
            print("PASS engine-client-python")
            return 0
        finally:
            proc.terminate()
            proc.wait(timeout=10)
    finally:
        shutil.rmtree(gen_dir, ignore_errors=True)
        shutil.rmtree(workdir, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
