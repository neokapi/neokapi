package main

import (
	"reflect"
	"testing"
)

func TestTransformSortsAndAggregates(t *testing.T) {
	rows := []rawRow{
		{Kind: "format", ID: "okf_xml", Status: "pass", Mode: "head-to-head", DurationMS: 100},
		{Kind: "format", ID: "okf_html", Status: "fail", Mode: "head-to-head", Detail: "diverged on x", DurationMS: 200},
		{Kind: "step", ID: "word-count", Status: "skip", Mode: "bridge-only", Detail: "needs testdata"},
		{Kind: "format", ID: "okf_json", Status: "pass", Mode: "head-to-head"},
		{Kind: "step", ID: "char-count", Status: "pass", Mode: "head-to-head"},
	}

	got, tot := transform(rows)

	wantOrder := []string{"okf_html", "okf_json", "okf_xml", "char-count", "word-count"}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d rows, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got[i].ID != want {
			t.Errorf("row[%d].ID = %q, want %q", i, got[i].ID, want)
		}
	}

	wantTotals := map[string]*totals{
		"format": {Pass: 2, Fail: 1, Skip: 0, Error: 0, Total: 3},
		"step":   {Pass: 1, Fail: 0, Skip: 1, Error: 0, Total: 2},
	}
	if !reflect.DeepEqual(tot, wantTotals) {
		t.Errorf("totals mismatch:\n got: %#v\nwant: %#v", tot, wantTotals)
	}
}
