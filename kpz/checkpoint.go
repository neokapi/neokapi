package kpz

// A checkpoint is a project's projections as of one operation in the
// workspace's log: the terms store, the content memory, the voice profiles,
// the decision ledger, the document adoptions and the block history, each a
// table of rows exactly as the store holds them, plus the rules the project
// widened to the workspace. core/projector writes one and reads it back, so a
// rebuild loads the latest checkpoint and replays only the operations after
// it, and a first pull from a remote starts from one with the widened rules
// in place.
//
// The rows travel as the store holds them rather than as the authored bundles
// a context package carries, because a rebuild has to land on the rows the
// writes left, timestamps and archived versions included. The member names the
// table it restores.
//
// A table too large to carry in the package travels in parts (Parts): blobs
// of the workspace that holds the checkpoint, each a run of the table's rows,
// so no file a checkpoint is made of grows with the project's history.

// ProjectionDir is the archive directory holding a checkpoint's tables.
const ProjectionDir = "projection/"

// TableDoc is one projection table of a checkpoint.
type TableDoc struct {
	// Table is the store table the rows belong to, and the member's name
	// under projection/.
	Table string
	// Data is the table's rows as JSON Lines, one object of column to value
	// per row, in the order the store holds them.
	Data []byte
}

// CheckpointMark says where a checkpoint stands.
type CheckpointMark struct {
	// Project is the workspace key of the project whose projections these are.
	Project string `json:"project"`
	// Through is the id of the last operation the tables include. A rebuild
	// from the checkpoint replays the operations after it.
	Through string `json:"through"`
	// Operations counts the operations the tables include.
	Operations int `json:"operations"`
	// Segments names the segment files of a shared context whose operations
	// the tables include, for a checkpoint kept on a remote (core/workspace).
	// A first pull starts from the checkpoint only when every other segment
	// holds operations after Through alone.
	Segments []string `json:"segments,omitempty"`
	// Parts names the blobs holding the rows of the tables too large to carry
	// in the package, in the order they load.
	Parts []CheckpointPart `json:"parts,omitempty"`
}

// CheckpointPart is one run of a checkpoint table's rows, kept in a blob.
type CheckpointPart struct {
	// Table is the store table the rows belong to.
	Table string `json:"table"`
	// Blob is the address of the blob holding the rows, as JSON Lines in the
	// form TableDoc.Data takes.
	Blob string `json:"blob"`
}
