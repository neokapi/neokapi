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

// The names of the one-time lines about a checkout's state. The checkout's
// record keeps each in its told set once a person was shown it.
const (
	// NoticeEarlierAtPath: state recorded for another directory at the
	// checkout's location, which `kapi state relink` takes.
	NoticeEarlierAtPath = "earlier-at-path"
	// NoticeWorkInTree: a `.kapi/work` in the checkout, which kapi no longer
	// uses.
	NoticeWorkInTree = "work-in-tree"
	// NoticeStateInTree: state an earlier kapi left in the checkout, which
	// `kapi state import` takes.
	NoticeStateInTree = "state-in-tree"
)

// PendingStateNotice is one line kapi owes a person about a checkout's state.
type PendingStateNotice struct {
	Name    string
	Message string
}

// PendingStateNotices lists the one-time lines about the checkout's state that
// no person has been shown yet, in the order a surface shows them. It reads
// and writes nothing: a surface shows each line and then records it with
// RecordStateNoticeGiven.
func PendingStateNotices(l Layout) []PendingStateNotice {
	if l.LocalDir != "" || l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return nil
	}
	if _, err := rootpath.Stat(l.Root, l.RecipePath); err != nil {
		return nil
	}
	var told map[string]bool
	if rec, err := dataroot.ReadRecord(dataroot.CheckoutDir(l.Root)); err == nil {
		told = rec.Told
	}
	var out []PendingStateNotice
	if !told[NoticeEarlierAtPath] && dataroot.RecordedElsewhereAt(l.Root) {
		out = append(out, PendingStateNotice{Name: NoticeEarlierAtPath, Message: earlierAtPathMessage(l)})
	}
	if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, WorkDirName)); err == nil && !told[NoticeWorkInTree] {
		out = append(out, PendingStateNotice{
			Name:    NoticeWorkInTree,
			Message: fmt.Sprintf("kapi: %s/%s in this checkout is no longer used; kapi keeps the checkout's state outside it, and you can delete it.", StateDirName, WorkDirName),
		})
	}
	if legacy := LegacyStateIn(l); len(legacy) > 0 && !told[NoticeStateInTree] {
		names := make([]string, len(legacy))
		for i, rel := range legacy {
			names[i] = filepath.ToSlash(filepath.Join(StateDirName, rel))
		}
		out = append(out, PendingStateNotice{
			Name:    NoticeStateInTree,
			Message: fmt.Sprintf("kapi: %s in this checkout hold state kapi no longer reads; to use it here, run `kapi state import -p %s`.", strings.Join(names, ", "), l.Root),
		})
	}
	return out
}

// RecordStateNoticeGiven records that a person was shown the line name about
// the checkout's state. The checkout's state directory and its record are
// created for it, because that the person was told is then state kapi keeps
// for the checkout.
func RecordStateNoticeGiven(l Layout, name string) error {
	dir, _, err := dataroot.EnsureCheckoutDir(l.Root)
	if err != nil {
		return err
	}
	recordCheckoutProject(l, dir)
	_, err = dataroot.MarkTold(dir, name)
	return err
}

// earlierAtPathMessage is the line about state recorded for another directory
// at the checkout's location. It offers the relink only for this project's
// state.
func earlierAtPathMessage(l Layout) string {
	if other := OtherProjectAt(l); other != "" {
		return fmt.Sprintf("kapi: state recorded for an earlier checkout at %s belongs to %s, another project, so this checkout starts with none and kapi keeps that state where it is.", l.Root, other)
	}
	return fmt.Sprintf("kapi: state recorded for an earlier checkout at %s belongs to another directory, so this checkout starts with none; if it is this checkout's, run `kapi state relink --from %s`.", l.Root, l.Root)
}

// NoticeState gives the one-time lines about the checkout's state
// (PendingStateNotices). Each line goes through StateNotice, and only a line
// that was shown is recorded as given. A checkout with nothing to tell, or a
// line nobody was shown, leaves the data root as it was. Nothing is written in
// the checkout.
func NoticeState(l Layout) {
	for _, n := range PendingStateNotices(l) {
		if StateNotice(n.Message) {
			_ = RecordStateNoticeGiven(l, n.Name)
		}
	}
}
