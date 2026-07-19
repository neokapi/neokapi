package billing

// The landing pricing page must never hand-duplicate the plan facts that live in
// this package. plans.json is the committed bridge: `go generate ./...` writes
// the Go-sourced facts (ids, names, credits, limits, per-seat/self-serve flags)
// into the landing build, and plans_gen_test.go fails the build if the committed
// file drifts from what plans.go would produce. To change a number: edit
// plans.go, run `go generate ./...` in the bowrain module, commit the refreshed
// JSON. This follows the repo's committed-JSON-not-runtime-export convention
// (like providers/ai/models.json) — never a runtime `export` subcommand.
//
//go:generate go run ./internal/genplans -out ../web/landing/src/generated/plans.json
