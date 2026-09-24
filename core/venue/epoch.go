package venue

// The content-model epoch prevents an older producer from replacing stored
// blocks with a less complete representation of the same source.
//
// A push replaces block structure, including runs, segmentation and overlays.
// Producers below the stored epoch are rejected unless the user requests a
// downgrade with `kapi push --force`.
//
// Block properties use a separate mechanism: BlockPropertyKeys declares the keys
// a producer supplies, and the receiver preserves undeclared properties.
// Adding a property therefore does not require an epoch change.
//
// # Updating the epoch
//
// Increment the epoch when changes to runs, segmentation, overlays, names or
// identity improve fidelity that an older producer cannot reproduce. Presentation,
// storage and transport changes do not require an increment.
//
// After a stream receives the new epoch, older producers must upgrade or force
// a downgrade. Record the compatibility reason for each increment below.
//
// Epoch 1 introduces this check. Producers that omit the epoch are treated as
// epoch 0 and cannot replace content received from an epoch-1 producer.
const ContentModelEpoch = 1

// ContentModelEpochProperty is the stream property recording the highest epoch
// a stream's content has received.
const ContentModelEpochProperty = "content_model_epoch"
