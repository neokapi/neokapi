# Positioning lab measurements

Run from the repository root after building the integrated kapi binary:

```sh
make build
node samples/audience-context/run.mjs
node scripts/positioning-lab/benchmark.mjs
node scripts/positioning-lab/corpus-manifest.mjs
node scripts/positioning-lab/verify-scripts.mjs
```

Generated measurements and corpus manifests are written to the ignored `harness/out/positioning-review/` directory.

The benchmark uses only Node built-ins, copies the sample into a temporary directory, and starts the supplied binary with isolated project/config/data/cache/plugin state. It makes no provider calls. It does not invoke recording or narration.

`POSITIONING_KAPI_BIN` selects an explicitly built binary. `POSITIONING_BENCH_OUTPUT` changes the output path. `POSITIONING_BENCH_RUNS` changes repetitions per workload and transport (default 40). Record the exact command when overriding defaults. Source revision and binary hash are both retained; an arbitrary supplied binary is not automatically a build of the recorded checkout.

The workloads are small JSON, 300-block JSON, and a one-field change in the larger file. Each iteration invokes CLI and MCP in alternating order against the same input. CLI starts a fresh process; MCP uses one persistent session. Exclude each workload's first invocation from the warm summaries, while retaining its time and every raw sample. The OS page cache is not reset, so fresh-process results are not fully cold-machine results.

The evidence retains raw report objects by hash, input hashes, exit codes, internal phase timings when present, external parsed-report wall time and machine details. The MCP startup handshake is measured separately. Semantic parity includes the gate, summary, findings and analyzer inventory; object-key ordering is immaterial. The runner asserts the extracted block count and alternates a prohibited phrase and its removal in sparse edits. Failures retain raw attempts and available reports. Post-response MCP resident-memory snapshots are outside the latency window and do not measure peak memory. Reports include scope and coverage. Throughput is the reciprocal of sequential mean response time; it is not concurrent server capacity. The synchronous API returns a complete report, so there is no separate streaming first-finding result. Encoding cost is measured separately with the Go report JSON benchmark; external time minus internal time is not a serialization measurement.

Run correctness checks before timing. The authored benchmark text is a workload, not a labeled quality corpus. Quality comparisons require independently authored cases and blinded human review. Do not tune on held-out cases or publish a customer-value claim from this timing run.
