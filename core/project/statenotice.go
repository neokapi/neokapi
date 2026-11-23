package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/rootpath"
)

// StateNotice receives the one-time lines kapi prints about a checkout's
// state. The host points it at its own error stream; the default writes to
// standard error.
var StateNotice = func(msg string) { fmt.Fprintln(os.Stderr, msg) }

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

// NoticeState gives the one-time lines about the checkout's state: state
// recorded for another folder at the checkout's location, and state an
// earlier kapi left inside the checkout. It creates the checkout's state
// directory under the data root, where kapi records that it told the person,
// and writes nothing in the checkout.
func NoticeState(l Layout) {
	if l.LocalDir != "" || l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return
	}
	if _, err := rootpath.Stat(l.Root, l.RecipePath); err != nil {
		return
	}
	// A read creates nothing: the checkout's state directory is created here
	// only when there is something to tell, and its record keeps that told.
	if !dataroot.CheckoutExists(l.Root) {
		_, inTreeWork := rootpath.Stat(l.Root, filepath.Join(l.StateDir, WorkDirName))
		if inTreeWork != nil && len(LegacyStateIn(l)) == 0 && !dataroot.RecordedElsewhereAt(l.Root) {
			return
		}
	}
	dir, earlier, err := dataroot.EnsureCheckoutDir(l.Root)
	if err != nil {
		return
	}
	noticeEarlierAtPath(l, earlier)
	noticeStateInTree(l, dir)
}

// noticeStateInTree tells the person, once per checkout, about state an
// earlier kapi left in the checkout: a work directory kapi no longer uses, and
// state it cannot rebuild, which a person imports.
func noticeStateInTree(l Layout, dir string) {
	if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, WorkDirName)); err == nil {
		if told, err := dataroot.MarkTold(dir, "work-in-tree"); err == nil && !told {
			StateNotice(fmt.Sprintf("kapi: %s/%s in this checkout is no longer used; kapi keeps the checkout's state outside it, and you can delete it.", StateDirName, WorkDirName))
		}
	}
	legacy := LegacyStateIn(l)
	if len(legacy) == 0 {
		return
	}
	if told, err := dataroot.MarkTold(dir, "state-in-tree"); err != nil || told {
		return
	}
	names := make([]string, len(legacy))
	for i, rel := range legacy {
		names[i] = filepath.ToSlash(filepath.Join(StateDirName, rel))
	}
	StateNotice(fmt.Sprintf("kapi: %s in this checkout hold state kapi no longer reads; to use it here, run `kapi state import -p %s`.",
		strings.Join(names, ", "), l.Root))
}

// noticeEarlierAtPath tells the person that state recorded for another
// directory at this checkout's location was kept, not used.
func noticeEarlierAtPath(l Layout, earlier []dataroot.Checkout) {
	if len(earlier) == 0 {
		return
	}
	StateNotice(fmt.Sprintf("kapi: state recorded for an earlier checkout at %s belongs to another directory, so this checkout starts with none; if it is this checkout's, run `kapi state relink --from %s`.",
		l.Root, l.Root))
}
