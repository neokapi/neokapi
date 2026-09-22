package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// What an import leaves on the record, and what it skips on the next run.
//
// Reading a checkout's context files puts them in force: from then on the
// store answers with what they held, on every surface. So an import writes a
// line per file into the project's context history, with the file's name and
// the digest of the bytes that were read, and `kapi context log` shows it
// beside every other operation.
//
// The digests are also what a second run skips on. They live in the workspace
// beside the rows they gate, keyed by the checkout that read them, so two
// clones of one project each read their own files and neither decides the
// other's have been read.

// importScribe writes an import's operations into the project's context log.
type importScribe struct {
	ops   *contextOpsSession
	actor contextop.Actor
	note  string
}

// importScribe opens the log an import records into, and settles who is
// recording before anything is read.
//
// A person runs an import. The context policy says so for the confirm each
// file is recorded as, and asking here means an import an agent started leaves
// the store as it found it rather than stopping part way through.
func (a *App) importScribe(ctx context.Context, recipePath string) (*importScribe, error) {
	ops, err := a.contextOps(ctx, recipePath)
	if err != nil {
		return nil, err
	}
	actor, note, err := ops.actorFor(ctx, contextop.Actor{}, "")
	if err != nil {
		return nil, err
	}
	if actor.Kind != contextop.ActorPerson {
		return nil, fmt.Errorf("%s may not import context: reading a checkout's context files puts them in force for everyone working in this project, which a person decides: ask the person working here to run `kapi context import`: %w",
			actor.String(), contextop.ErrRefused)
	}
	return &importScribe{ops: ops, actor: actor, note: note}, nil
}

// read records one context file the import put into the store, naming what the
// file holds and the bytes it held.
func (s *importScribe) read(ctx context.Context, src contextSource, n int, digest string) error {
	return s.record(ctx, src.rel, digest, importSubjectText(src, n))
}

// readRecord records the decision record the import read. A record is a
// directory of shards rather than one file, so it is recorded as the directory
// with the number of decisions it carried.
func (s *importScribe) readRecord(ctx context.Context, rel string, n int) error {
	return s.record(ctx, rel, "", fmt.Sprintf("decision record %s, %s", rel, pluralUnit(n, "decision", "decisions")))
}

// record appends one confirm naming a source the import read.
//
// The evidence carries the source's project-relative path and, for a single
// file, the SHA-256 of the bytes that were read, so a reader of the log can
// tell which version of a file is in force.
func (s *importScribe) record(ctx context.Context, rel, digest, subject string) error {
	evidence := []contextop.Evidence{{Path: rel}}
	if digest != "" {
		evidence[0].Quote = "sha256:" + digest
	}
	_, err := s.ops.ledger.Append(ctx, s.ops.stamp(contextop.Record{
		Actor:    s.actor,
		Kind:     contextop.KindConfirm,
		Subject:  contextop.Subject{Kind: contextop.SubjectNote, Text: subject},
		Evidence: evidence,
		Note:     s.note,
	}, evidence))
	if err != nil {
		return teachRefusal(err)
	}
	return nil
}

// importSubjectText says what one source carried, in the words the log prints.
func importSubjectText(src contextSource, n int) string {
	switch src.kind {
	case sourceKindTerms:
		return fmt.Sprintf("terms bundle %s, %s", src.rel, pluralUnit(n, "concept", "concepts"))
	case sourceKindMemory:
		return fmt.Sprintf("content-memory bundle %s, %s", src.rel, pluralUnit(n, "entry", "entries"))
	case sourceKindVoice:
		return "voice profile " + src.rel
	}
	return src.rel
}

// importStamps maps a context file to the digest it held when a checkout read
// it.
type importStamps map[importStampID]string

// importStampID addresses one stamp: the checkout that read the file, and the
// file inside it.
type importStampID struct {
	checkout string
	path     string
}

// importStampKey addresses the stamp for one file in one checkout.
func importStampKey(checkout, rel string) importStampID {
	return importStampID{checkout: checkout, path: rel}
}

// normalizedCheckout spells a checkout root the way every stamp keyed by it is
// spelled, so a tree reached through a symlink stamps the same rows as the tree
// itself.
func normalizedCheckout(root string) string { return NormalizeCheckoutPath(root) }

// loadImportStamps reads what this checkout has already read.
func (a *App) loadImportStamps(ctx context.Context, checkout string) (importStamps, error) {
	stamps := importStamps{}
	ws, err := a.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	if ws == nil {
		return stamps, nil
	}
	held, err := ws.ContextImports(ctx, checkout)
	if err != nil {
		return nil, err
	}
	for _, s := range held {
		stamps[importStampKey(s.Checkout, s.Path)] = s.Digest
	}
	return stamps, nil
}

// saveImportStamps writes the stamps back, so the next run of the same import
// reads only what has moved.
func (a *App) saveImportStamps(ctx context.Context, stamps importStamps) error {
	if len(stamps) == 0 {
		return nil
	}
	ws, err := a.Workspace(ctx)
	if err != nil {
		return err
	}
	if ws == nil {
		return nil
	}
	rows := make([]workspace.ContextImportStamp, 0, len(stamps))
	for id, digest := range stamps {
		rows = append(rows, workspace.ContextImportStamp{
			Checkout: id.checkout,
			Path:     id.path,
			Digest:   digest,
		})
	}
	return ws.NoteContextImports(ctx, rows...)
}

// fileDigest is the SHA-256 of a file's bytes, as lower-case hex.
func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read context source: %w", err)
	}
	defer func() { _ = f.Close() }()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", fmt.Errorf("read context source %s: %w", path, err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
