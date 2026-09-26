package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// Measure 3, settling.
//
// A suggestion becomes an established rule by evidence the log holds, so two
// machines holding the same operations must reach the same answer whatever
// order their logs were merged in. The measure scripts a small story across
// four machines (eval_product.go), settles each machine's log, then merges the
// four logs into a fresh workspace in every order, settling after each merge.
// It is met when every order establishes exactly the expected rules and every
// rule ends at the same status. It makes no model call and runs in CI as a Go
// test.

// EvalSettle is Measure 3's result.
type EvalSettle struct {
	// Orders is how many merge orders were run.
	Orders int `json:"orders"`
	// Expected are the rules the scenario must establish, by the form each
	// avoids; Established are the rules the first order established.
	Expected    []string `json:"expected"`
	Established []string `json:"established"`
	// Outcome is each rule's status after the first order.
	Outcome map[string]string `json:"outcome"`
	// Differing names the orders whose outcome differed from the first.
	Differing []string `json:"differing,omitempty"`
	Verdict   string   `json:"verdict"`
}

// Met reports whether the measure's bar was reached.
func (s EvalSettle) Met() bool {
	return len(s.Differing) == 0 && slices.Equal(s.Established, s.Expected)
}

// measureEvalSettle runs Measure 3 in a scratch directory of its own.
func measureEvalSettle(ctx context.Context) (EvalSettle, error) {
	dir, err := os.MkdirTemp("", "kapi-eval-settle-")
	if err != nil {
		return EvalSettle{}, err
	}
	defer os.RemoveAll(dir)

	open := func(name string) (*workspace.Workspace, *contextop.Ledger, error) {
		ws, oerr := workspace.OpenLocal(ctx, filepath.Join(dir, name))
		if oerr != nil {
			return nil, nil, oerr
		}
		return ws, contextop.NewLedger(ws, contextop.PersonDecides), nil
	}

	origin, originLedger, err := open("origin")
	if err != nil {
		return EvalSettle{}, err
	}
	defer origin.Close()
	ids, err := evalSettleOrigin(ctx, originLedger)
	if err != nil {
		return EvalSettle{}, fmt.Errorf("record the origin: %w", err)
	}

	machines := make([]*workspace.Workspace, len(evalSettleMachines))
	for i, m := range evalSettleMachines {
		ws, ledger, oerr := open(fmt.Sprintf("machine-%d", i))
		if oerr != nil {
			return EvalSettle{}, oerr
		}
		defer ws.Close()
		if err := evalCopyLog(ctx, origin, ws); err != nil {
			return EvalSettle{}, err
		}
		if err := m.Record(ctx, ledger, ids); err != nil {
			return EvalSettle{}, fmt.Errorf("record the %s machine: %w", m.Name, err)
		}
		if _, err := evalSettleOutcome(ctx, ledger); err != nil {
			return EvalSettle{}, fmt.Errorf("settle the %s machine: %w", m.Name, err)
		}
		machines[i] = ws
	}

	out := EvalSettle{Expected: slices.Sorted(slices.Values(evalSettleExpected))}
	for n, order := range evalPermutations(len(machines)) {
		ws, ledger, oerr := open(fmt.Sprintf("order-%d", n))
		if oerr != nil {
			return EvalSettle{}, oerr
		}
		var outcome map[string]string
		for _, i := range order {
			if err := evalCopyLog(ctx, machines[i], ws); err != nil {
				ws.Close()
				return EvalSettle{}, err
			}
			if outcome, err = evalSettleOutcome(ctx, ledger); err != nil {
				ws.Close()
				return EvalSettle{}, err
			}
		}
		ws.Close()
		out.Orders++
		if out.Outcome == nil {
			out.Outcome = outcome
			for _, term := range slices.Sorted(maps.Keys(outcome)) {
				if outcome[term] == evalSettleEstablished {
					out.Established = append(out.Established, term)
				}
			}
			continue
		}
		if !maps.Equal(out.Outcome, outcome) {
			out.Differing = append(out.Differing, evalOrderName(order))
		}
	}

	switch {
	case out.Met():
		out.Verdict = fmt.Sprintf("met: %d of %d rules established, the expected ones and no other, identically in all %d merge orders",
			len(out.Established), len(evalSettleRules), out.Orders)
	case len(out.Differing) > 0:
		out.Verdict = fmt.Sprintf("not met: %d of %d merge orders reached a different outcome", len(out.Differing), out.Orders)
	default:
		out.Verdict = fmt.Sprintf("not met: established %s, expected %s",
			strings.Join(out.Established, ", "), strings.Join(out.Expected, ", "))
	}
	return out, nil
}

// evalCopyLog merges one workspace's operation log into another, by union.
func evalCopyLog(ctx context.Context, from, to *workspace.Workspace) error {
	ops, err := from.Ops(ctx, 0, 0)
	if err != nil {
		return err
	}
	_, err = to.Record(ctx, ops...)
	return err
}

// evalPermutations lists every order of n machines, in lexicographic order.
func evalPermutations(n int) [][]int {
	var out [][]int
	var walk func(prefix []int, rest []int)
	walk = func(prefix, rest []int) {
		if len(rest) == 0 {
			out = append(out, slices.Clone(prefix))
			return
		}
		for i := range rest {
			next := append(slices.Clone(rest[:i]), rest[i+1:]...)
			walk(append(prefix, rest[i]), next)
		}
	}
	rest := make([]int, n)
	for i := range rest {
		rest[i] = i
	}
	walk(nil, rest)
	return out
}

// evalOrderName names a merge order by its machines.
func evalOrderName(order []int) string {
	names := make([]string, len(order))
	for i, m := range order {
		names[i] = evalSettleMachines[m].Name
	}
	return strings.Join(names, " > ")
}

// renderEvalSettle renders Measure 3's section of the report.
func renderEvalSettle(out *strings.Builder, s *EvalSettle) {
	out.WriteString("## Measure 3: settling\n\n")
	if s == nil {
		out.WriteString("Settling was not measured.\n\n")
		return
	}
	fmt.Fprintf(out, "Scripted logs from %d machines, merged in %d orders. %s.\n\n", len(evalSettleMachines), s.Orders, s.Verdict)
	out.WriteString("| Rule | Status |\n|---|---|\n")
	for _, term := range slices.Sorted(maps.Keys(s.Outcome)) {
		fmt.Fprintf(out, "| %s | %s |\n", term, s.Outcome[term])
	}
	for _, order := range s.Differing {
		fmt.Fprintf(out, "\nDiffered: %s\n", order)
	}
	out.WriteString("\n")
}
