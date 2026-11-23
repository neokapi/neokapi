package project

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/rootpath"
)

// StateNotice shows a person a one-time line about a checkout's state and
// reports whether it did. kapi records a notice as given only when it was
// shown. A line that reached nobody, in a version check, a `--json` run or
// output a program reads, is shown on the next command a person runs in the
// checkout. The default shows nothing and reports false. A surface that has a
// person to tell sets it, as the CLI does before an interactive project
// command.
var StateNotice = func(string) bool { return false }

// LegacyStateInTree are the places under `.kapi/` where earlier versions of
// kapi kept a checkout's state that cannot be rebuilt. They hold the redaction
// vault and its batches, the sync state with a claim token, and the saved
// filters. kapi reads none of them; `kapi state import` brings them into the checkout's
// state directory once a person asks.
var LegacyStateInTree = []string{
	VaultDirName,
	SyncDirName,
	LocalFiltersFilename,
	filepath.Join(WorkDirName, VaultDirName),
	filepath.Join(WorkDirName, CacheDirName, RedactionDirName),
	filepath.Join(WorkDirName, CacheDirName, "refs.json"),
	filepath.Join(WorkDirName, CacheDirName, "sync-cache.json"),
	filepath.Join(CacheDirName, RedactionDirName),
}

// LegacyStateIn lists the entries of LegacyStateInTree present in the layout's
// `.kapi/`, relative to it.
func LegacyStateIn(l Layout) []string {
	var out []string
	for _, rel := range LegacyStateInTree {
		if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, rel)); err == nil {
			out = append(out, rel)
		}
	}
	return out
}

// The names under which a checkout's record keeps the one-time notices given.
const (
	toldWorkInTree  = "work-in-tree"
	toldStateInTree = "state-in-tree"
)

// NoticeState gives the one-time lines about the checkout's state: state
// recorded for another folder at the checkout's location, and state an
// earlier kapi left inside the checkout. Each line goes through StateNotice,
// and only a line that was shown is recorded as given. The record is the
// checkout's state directory under the data root, created when the first line
// is shown, because that a person was told is then state kapi keeps for the
// checkout. A checkout with nothing to tell, or a line nobody was shown,
// leaves the data root as it was. Nothing is written in the checkout.
func NoticeState(l Layout) {
	if l.LocalDir != "" || l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return
	}
	if _, err := rootpath.Stat(l.Root, l.RecipePath); err != nil {
		return
	}
	if !dataroot.CheckoutExists(l.Root) && dataroot.RecordedElsewhereAt(l.Root) {
		// Creating the state directory is what retires this line: once it
		// exists, the state recorded for the other directory is no longer
		// this checkout's to be told about.
		if !StateNotice(earlierAtPathMessage(l)) {
			return
		}
		if _, _, err := dataroot.EnsureCheckoutDir(l.Root); err != nil {
			return
		}
	}
	noticeStateInTree(l)
}

// noticeStateInTree tells the person, once per checkout, about state an
// earlier kapi left in the checkout: a work directory kapi no longer uses, and
// state it cannot rebuild, which a person imports.
func noticeStateInTree(l Layout) {
	if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, WorkDirName)); err == nil {
		tellOnce(l, toldWorkInTree, fmt.Sprintf("kapi: %s/%s in this checkout is no longer used; kapi keeps the checkout's state outside it, and you can delete it.", StateDirName, WorkDirName))
	}
	legacy := LegacyStateIn(l)
	if len(legacy) == 0 {
		return
	}
	names := make([]string, len(legacy))
	for i, rel := range legacy {
		names[i] = filepath.ToSlash(filepath.Join(StateDirName, rel))
	}
	tellOnce(l, toldStateInTree, fmt.Sprintf("kapi: %s in this checkout hold state kapi no longer reads; to use it here, run `kapi state import -p %s`.",
		strings.Join(names, ", "), l.Root))
}

// tellOnce shows msg through StateNotice unless the checkout's record says it
// was given, and records it as given once it was shown. The checkout's state
// directory is created for the record only then.
func tellOnce(l Layout, name, msg string) {
	if rec, err := dataroot.ReadRecord(dataroot.CheckoutDir(l.Root)); err == nil && rec.Told[name] {
		return
	}
	if !StateNotice(msg) {
		return
	}
	dir, _, err := dataroot.EnsureCheckoutDir(l.Root)
	if err != nil {
		return
	}
	_, _ = dataroot.MarkTold(dir, name)
}

// noticeEarlierAtPath tells the person that state recorded for another
// directory at this checkout's location was kept, not used. It runs when the
// checkout's state directory is created, which is the one time kapi finds the
// earlier state beside a new one.
func noticeEarlierAtPath(l Layout, earlier []dataroot.Checkout) {
	if len(earlier) == 0 {
		return
	}
	StateNotice(earlierAtPathMessage(l))
}

// earlierAtPathMessage is the line about state recorded for another directory
// at the checkout's location.
func earlierAtPathMessage(l Layout) string {
	return fmt.Sprintf("kapi: state recorded for an earlier checkout at %s belongs to another directory, so this checkout starts with none; if it is this checkout's, run `kapi state relink --from %s`.",
		l.Root, l.Root)
}
