package projector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/kpz"
)

// LocalKinds are the operation kinds that stay on the machine that recorded
// them when a project's context is shared (workspace.Sync): the project's
// registration, its checkpoints, and the rules a person widened to the whole
// workspace, which belong to this machine's workspace rather than the project.
var LocalKinds = []string{workspace.OpRegisterProject, workspace.OpForgetProject, KindCheckpoint, KindRules}

// Syncer is the projector as a shared context's workspace.Applier: it applies
// what a pull merged, and reads and writes the checkpoints a remote carries.
func (p *Projector) Syncer() workspace.Applier { return syncer{p} }

type syncer struct{ p *Projector }

// Apply catches the stores up with the log, or rebuilds them from it.
func (s syncer) Apply(ctx context.Context, rebuild bool) error {
	if !rebuild {
		return s.p.CatchUp(ctx)
	}
	_, err := s.p.Rebuild(ctx)
	return err
}

// Checkpoint writes the project's projections as a checkpoint file for a
// remote. It records nothing in the log, and it leaves out the rules the
// project widened to the workspace, which stay on this machine.
func (s syncer) Checkpoint(ctx context.Context, segments []string) ([]byte, string, error) {
	p := s.p
	if p.log == nil {
		return nil, "", errors.New("projector: this store has no log to checkpoint")
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	if err := p.catchUpLocked(ctx, nil); err != nil {
		return nil, "", err
	}
	ops, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key})
	if err != nil {
		return nil, "", err
	}
	mark := kpz.CheckpointMark{Project: string(p.key), Segments: slices.Clone(segments)}
	for _, op := range ops {
		if projects(op.Kind) && op.Kind != KindRules {
			mark.Operations++
			mark.Through = max(mark.Through, op.ID)
		}
	}
	if mark.Operations == 0 {
		return nil, "", nil
	}
	tables, err := p.dumpTables(ctx)
	if err != nil {
		return nil, "", err
	}
	tables = slices.DeleteFunc(tables, func(t kpz.TableDoc) bool { return t.Table == rulesTable })
	pkg := &kpz.Package{
		Kind:       kpz.KindCheckpoint,
		Created:    time.Now().UTC().Format(time.RFC3339),
		Tables:     tables,
		Checkpoint: &mark,
	}
	data, err := pkg.Marshal()
	if err != nil {
		return nil, "", fmt.Errorf("projector: write checkpoint: %w", err)
	}
	return data, mark.Through, nil
}

// CheckpointMark reads where a checkpoint file stands.
func (s syncer) CheckpointMark(data []byte) (string, []string, error) {
	pkg, err := s.p.readCheckpoint(data)
	if err != nil {
		return "", nil, err
	}
	return pkg.Checkpoint.Through, pkg.Checkpoint.Segments, nil
}

// InstallCheckpoint keeps a checkpoint file read from a remote and records
// it as standing at every operation the log holds for the project now, so the
// next rebuild loads it and replays only what arrives after.
func (s syncer) InstallCheckpoint(ctx context.Context, data []byte) error {
	p := s.p
	pkg, err := p.readCheckpoint(data)
	if err != nil {
		return err
	}
	if p.log == nil {
		return errors.New("projector: this store has no log to record a checkpoint in")
	}
	ops, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key})
	if err != nil {
		return err
	}
	var seq int64
	for _, op := range ops {
		seq = max(seq, op.Seq)
	}
	address, err := p.log.PutBlob(ctx, data)
	if err != nil {
		return fmt.Errorf("projector: store checkpoint: %w", err)
	}
	body, err := json.Marshal(checkpointPayload{
		Blob: address, Through: pkg.Checkpoint.Through, Seq: seq, Operations: pkg.Checkpoint.Operations,
	})
	if err != nil {
		return err
	}
	_, err = p.log.Record(ctx, workspace.Op{Project: p.key, Kind: KindCheckpoint, Payload: body})
	return err
}

// readCheckpoint parses a checkpoint file and checks it is this project's.
func (p *Projector) readCheckpoint(data []byte) (*kpz.Package, error) {
	pkg, err := kpz.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("projector: read checkpoint: %w", err)
	}
	if pkg.Kind != kpz.KindCheckpoint || pkg.Checkpoint == nil {
		return nil, fmt.Errorf("projector: a %s package is not a checkpoint", pkg.Kind)
	}
	if pkg.Checkpoint.Project != string(p.key) {
		return nil, fmt.Errorf("projector: the checkpoint belongs to project %q, not %q", pkg.Checkpoint.Project, p.key)
	}
	if !workspace.ValidOpID(pkg.Checkpoint.Through) {
		return nil, fmt.Errorf("projector: the checkpoint stands at %q, which is not an operation id", pkg.Checkpoint.Through)
	}
	return pkg, nil
}
