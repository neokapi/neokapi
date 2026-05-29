#!/usr/bin/env python3
"""Benchmark the small-ML checker models kapi would run in-process.

Records, per model: the download size a user pays (the ONNX export + its
tokenizer), cold-start (session load) time, per-inference latency on a typical
task, and peak resident memory. The numbers come from onnxruntime — the same
runtime the Go plugin embeds — so they transfer to the kapi-check plugin.

Writes a JSON report consumed by the docs ML-benchmark dashboard. Run:

    python3 scripts/ml-benchmark.py --out web/docs/src/pages/ml-benchmark/_benchmark.json
"""
from __future__ import annotations

import argparse
import json
import platform
import statistics
import time

import numpy as np
import onnxruntime as ort
import psutil
from huggingface_hub import HfApi, hf_hub_download
from tokenizers import Tokenizer

# Candidate checker models. inference=True ones are actually run; the rest are
# sized only (their inference shape differs and the size is the headline pain).
MODELS = [
    {
        "key": "e5-small",
        "repo": "intfloat/multilingual-e5-small",
        "role": "Voice / style similarity (sentence embeddings)",
        "license": "MIT",
        "onnx": "onnx/model.onnx",
        "tokenizer_repo": "intfloat/multilingual-e5-small",
        "inference": True,
        "task": "embed a sentence",
    },
    {
        "key": "e5-small-O4",
        "repo": "intfloat/multilingual-e5-small",
        "role": "Voice / style similarity — fp16-optimized (O4) variant",
        "license": "MIT",
        "onnx": "onnx/model_O4.onnx",
        "tokenizer_repo": "intfloat/multilingual-e5-small",
        "inference": True,
        "task": "embed a sentence",
    },
    {
        "key": "e5-small-int8",
        "repo": "intfloat/multilingual-e5-small",
        "role": "Voice / style similarity — int8-quantized variant",
        "license": "MIT",
        "onnx": "onnx/model_qint8_avx512_vnni.onnx",
        "tokenizer_repo": "intfloat/multilingual-e5-small",
        "inference": True,
        "task": "embed a sentence",
    },
    {
        "key": "gliner-multi",
        "repo": "onnx-community/gliner_multi-v2.1",
        "role": "Do-not-translate / entity spotting (zero-shot NER)",
        "license": "Apache-2.0",
        "onnx": "onnx/model.onnx",
        "tokenizer_repo": "onnx-community/gliner_multi-v2.1",
        "inference": False,
        "task": "detect entities",
    },
    {
        "key": "gliner-multi-int8",
        "repo": "onnx-community/gliner_multi-v2.1",
        "role": "GLiNER — int8-quantized variant (download/footprint mitigation)",
        "license": "Apache-2.0",
        "onnx": "onnx/model_int8.onnx",
        "tokenizer_repo": "onnx-community/gliner_multi-v2.1",
        "inference": False,
        "task": "detect entities",
    },
    {
        "key": "formality",
        "repo": "s-nlp/mdistilbert-base-formality-ranker",
        "role": "Register / formality classification",
        "license": "academic (s-nlp)",
        "onnx": None,  # ships PyTorch only; needs an ONNX export step (optimum)
        "tokenizer_repo": "s-nlp/mdistilbert-base-formality-ranker",
        "inference": False,
        "task": "classify formality",
    },
    {
        "key": "sat-3l-sm",
        "repo": "segment-any-text/sat-3l-sm",
        "role": "Reference: the segmenter kapi already ships (kapi-sat)",
        "license": "MIT",
        "onnx": "model.onnx",
        "tokenizer_repo": "facebookAI/xlm-roberta-base",
        "inference": False,
        "task": "segment text",
    },
]

SAMPLES = [
    "You have {count} items in your Acme Cloud cart.",
    "Willkommen bei Acme Cloud — bitte melden Sie sich an, um fortzufahren.",
    "Veuillez confirmer votre adresse e-mail pour activer votre compte.",
    "请在结账前确认您的收货地址和付款方式。",
    "Drag the file here, or click to browse your computer for a document.",
]


def repo_download_bytes(api: HfApi, repo: str, files: list[str]) -> int:
    """Sum the byte size of the named files in a repo (no full download)."""
    info = api.model_info(repo, files_metadata=True)
    by_name = {s.rfilename: (s.size or 0) for s in info.siblings}
    total = 0
    for f in files:
        # Tokenizer may be tokenizer.json or sentencepiece; count whichever exists.
        if f == "<tokenizer>":
            for cand in ("tokenizer.json", "sentencepiece.bpe.model", "spm.model"):
                if by_name.get(cand):
                    total += by_name[cand]
                    break
        else:
            total += by_name.get(f, 0)
    return total


def rss_mb() -> float:
    return psutil.Process().memory_info().rss / (1024 * 1024)


def bench_embed(onnx_path: str, tok_path: str, runs: int = 30) -> dict:
    """Load an embedding model and time per-sentence inference + peak memory."""
    tok = Tokenizer.from_file(tok_path)
    base_rss = rss_mb()

    t0 = time.perf_counter()
    so = ort.SessionOptions()
    sess = ort.InferenceSession(onnx_path, so, providers=["CPUExecutionProvider"])
    load_ms = (time.perf_counter() - t0) * 1000
    loaded_rss = rss_mb()

    input_names = {i.name for i in sess.get_inputs()}

    def run_one(text: str) -> float:
        enc = tok.encode("query: " + text)
        ids = np.array([enc.ids], dtype=np.int64)
        mask = np.array([enc.attention_mask], dtype=np.int64)
        feeds = {"input_ids": ids, "attention_mask": mask}
        if "token_type_ids" in input_names:
            feeds["token_type_ids"] = np.zeros_like(ids)
        t = time.perf_counter()
        sess.run(None, feeds)
        return (time.perf_counter() - t) * 1000

    run_one(SAMPLES[0])  # warm up
    lat = []
    peak = loaded_rss
    for i in range(runs):
        lat.append(run_one(SAMPLES[i % len(SAMPLES)]))
        peak = max(peak, rss_mb())

    lat.sort()
    return {
        "load_ms": round(load_ms, 1),
        "infer_ms_mean": round(statistics.mean(lat), 2),
        "infer_ms_p50": round(lat[len(lat) // 2], 2),
        "infer_ms_p90": round(lat[int(len(lat) * 0.9)], 2),
        "session_rss_mb": round(loaded_rss - base_rss, 1),
        "peak_rss_mb": round(peak - base_rss, 1),
        "runs": runs,
        "threads": so.intra_op_num_threads or psutil.cpu_count(),
    }


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    api = HfApi()
    runtime_note = "onnxruntime CPU; the Go plugin bundles libonnxruntime (~18 MB) per platform"
    results = []
    for m in MODELS:
        files = []
        if m["onnx"]:
            files.append(m["onnx"])
        files.append("<tokenizer>")
        try:
            dl = repo_download_bytes(api, m["repo"], files)
        except Exception as e:  # network / repo shape
            dl = 0
            print(f"  ! size lookup failed for {m['key']}: {e}")
        entry = {
            "key": m["key"],
            "repo": m["repo"],
            "role": m["role"],
            "license": m["license"],
            "task": m["task"],
            "download_bytes": dl,
            "download_mb": round(dl / (1024 * 1024), 1) if dl else None,
            "onnx_available": bool(m["onnx"]),
        }
        if m["inference"] and m["onnx"]:
            print(f"  · downloading + benchmarking {m['key']} …")
            t0 = time.perf_counter()
            onnx_path = hf_hub_download(m["repo"], m["onnx"])
            tok_path = hf_hub_download(m["tokenizer_repo"], "tokenizer.json")
            entry["download_s"] = round(time.perf_counter() - t0, 1)
            entry.update(bench_embed(onnx_path, tok_path))
        results.append(entry)
        print(f"  ✓ {m['key']}: {entry.get('download_mb')} MB")

    report = {
        "generated_note": "Run scripts/ml-benchmark.py to regenerate.",
        "platform": f"{platform.system()} {platform.machine()}",
        "python": platform.python_version(),
        "onnxruntime": ort.__version__,
        "runtime_note": runtime_note,
        "samples": SAMPLES,
        "models": results,
    }
    with open(args.out, "w") as f:
        json.dump(report, f, indent=2)
    print(f"\nwrote {args.out}")


if __name__ == "__main__":
    main()
