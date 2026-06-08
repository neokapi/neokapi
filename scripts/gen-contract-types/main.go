// gen-contract-types generates the shared TypeScript IO-contract atom types
// (packages/contract-types/src/contract.gen.ts) from the framework's Go source
// of truth. It mirrors the codegen+drift-gate pattern of scripts/gen-refs.
//
// The TS contract types had drifted across six hand-maintained copies (issue
// #817). The atoms that map 1:1 onto Go (Side, IOPort, ToolMeta, ParameterGroup,
// LayoutHints, OptionItem, FileFilter, PathAnnotation, FormatMeta) are emitted
// here by reflecting over the actual structs, so the committed TS can never
// silently diverge from Go. The superset "envelope" types that the UI layers
// extend beyond Go (ComponentSchema, PropertySchema, ConditionExpr) have no 1:1
// Go struct and are hand-authored in packages/contract-types/src/manual.ts.
//
// Run from the repo root:
//
//	go run ./scripts/gen-contract-types            # write contract.gen.ts
//	go run ./scripts/gen-contract-types -check     # drift gate (no write)
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		out   = flag.String("out", "packages/contract-types/src/contract.gen.ts", "output TS file")
		check = flag.Bool("check", false, "drift gate: regenerate in memory and fail if the committed file is stale (does not write)")
	)
	flag.Parse()

	generated, err := emit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-contract-types: %v\n", err)
		os.Exit(1)
	}

	if *check {
		committed, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gen-contract-types -check: cannot read %s: %v\n", *out, err)
			os.Exit(1)
		}
		if string(committed) != generated {
			fmt.Fprintf(os.Stderr, "gen-contract-types -check: %s is stale vs core/schema; run `make generate-contract-types` and commit the result\n", *out)
			os.Exit(1)
		}
		fmt.Printf("contract types are fresh (%s)\n", *out)
		return
	}

	if err := os.WriteFile(*out, []byte(generated), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gen-contract-types: write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", *out)
}
