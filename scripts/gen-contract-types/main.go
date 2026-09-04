// gen-contract-types generates the shared TypeScript contract types and the
// content-model JSON Schema from the framework's Go sources of truth. It
// mirrors the codegen+drift-gate pattern of scripts/gen-refs; see the
// `outputs` variable below for the artifact list.
//
// The TS contract types had drifted across six hand-maintained copies (issue
// #817). The atoms that map 1:1 onto Go (Side, IOPort, ToolMeta, ParameterGroup,
// LayoutHints, OptionItem, FileFilter, PathAnnotation, FormatMeta) are emitted
// here by reflecting over the actual structs, so the committed TS can never
// silently diverge from Go. The content-model types (AD-034) are emitted the
// same way — from the neokapi.content.v1 proto descriptors and the projection
// structs — so no frontend package hand-mirrors the model. The superset
// "envelope" types that the UI layers extend beyond Go (ComponentSchema,
// PropertySchema, ConditionExpr) have no 1:1 Go struct and are hand-authored
// in packages/contract-types/src/manual.ts.
//
// Run from the repo root:
//
//	go run ./scripts/gen-contract-types            # write all outputs
//	go run ./scripts/gen-contract-types -check     # drift gate (no write)
package main

import (
	"flag"
	"fmt"
	"os"
)

// output is one generated artifact: its repo-relative path and generator.
type output struct {
	path string
	gen  func() (string, error)
}

// outputs enumerates every generated artifact this tool owns:
//
//   - contract.gen.ts — the IO-contract atoms reflected from Go (issue #817).
//   - content.gen.ts — the content-model types (AD-034): neokapi.content.v1
//     wire shapes as they appear in canonical protojson, plus the model.Run
//     JSON and core/editor ContentTree projection shapes (see content.go).
//   - content.schema.json — a JSON Schema for the canonical content-model
//     JSON, generated from the proto descriptors for non-proto consumers
//     (see schema.go). The protojson golden files under
//     core/proto/content/v1/testdata remain the binding contract.
//   - review.gen.ts — the review model (S-07) every review client reads,
//     reflected from core/review and the structs it carries (see review.go).
var outputs = []output{
	{"packages/contract-types/src/contract.gen.ts", emit},
	{"packages/contract-types/src/content.gen.ts", emitContent},
	{"packages/contract-types/src/review.gen.ts", emitReview},
	{"core/proto/content/v1/content.schema.json", emitSchema},
}

func main() {
	check := flag.Bool("check", false, "drift gate: regenerate in memory and fail if a committed file is stale (does not write)")
	flag.Parse()

	stale := false
	for _, o := range outputs {
		generated, err := o.gen()
		if err != nil {
			fmt.Fprintf(os.Stderr, "gen-contract-types: %s: %v\n", o.path, err)
			os.Exit(1)
		}

		if *check {
			committed, err := os.ReadFile(o.path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "gen-contract-types -check: cannot read %s: %v\n", o.path, err)
				os.Exit(1)
			}
			if string(committed) != generated {
				fmt.Fprintf(os.Stderr, "gen-contract-types -check: %s is stale vs the Go source of truth; run `make generate-contract-types` and commit the result\n", o.path)
				stale = true
				continue
			}
			fmt.Printf("fresh: %s\n", o.path)
			continue
		}

		if err := os.WriteFile(o.path, []byte(generated), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gen-contract-types: write %s: %v\n", o.path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", o.path)
	}
	if stale {
		os.Exit(1)
	}
}
