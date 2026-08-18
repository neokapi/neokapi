package venue

// The content-model epoch: what stops an older producer from overwriting
// content a richer one wrote.
//
// A push replaces the blocks it carries. That is right when both sides read the
// same file the same way, and wrong when they do not: a kapi whose reader
// segments differently, resolves fewer runs, or does not produce overlays sends
// structurally poorer blocks for the same source, and storing them silently
// costs the project fidelity nobody asked to give up. Two runners on different
// versions then take turns — one enriching, the other flattening.
//
// Properties do not need this. A push declares the keys its readers emit
// (BlockPropertyKeys) and the far side keeps what the push said nothing about,
// so a field a reader learns to record arrives with no version involved at all.
// That is the mechanism to prefer: it is automatic, and it never refuses work.
//
// Structure cannot be handled that way — there is no merging half a
// segmentation — so the model states its generation instead, and a push from an
// older generation is refused rather than applied. `kapi push --force` is the
// deliberate downgrade, for the case where flattening is what you meant.
//
// # Bumping this
//
// Bump when the content a reader produces gains fidelity an older kapi cannot
// reproduce from the same file: a change to the run model, to segmentation, to
// overlays, to how blocks are named or identified. Do NOT bump for a new block
// property — that is what the declaration is for — nor for a change that only
// affects how content is presented, stored, or transported.
//
// A bump is a compatibility break by design: every producer below the new epoch
// stops being able to push to a stream that has received it, and is told to
// upgrade. That is the point, so bump deliberately and note why below.
//
// 1 — the first stated epoch. Producers older than this mechanism state
// nothing, which reads as epoch 0, so the first push from a stating producer
// closes a stream to them.
const ContentModelEpoch = 1

// ContentModelEpochProperty is the stream property recording the highest epoch
// a stream's content has received.
const ContentModelEpochProperty = "content_model_epoch"
