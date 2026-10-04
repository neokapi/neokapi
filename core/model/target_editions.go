package model

// EachTargetEdition yields every target the block holds with the key it is
// filed under, for use as a range function: each edition other than the one
// the block was read in, a target in the source language included, and a
// target filed under the zero key, which TargetEdition("") reads and
// EachEdition never yields. The setters file every target under its canonical
// key. The order is unspecified.
func (b *Block) EachTargetEdition(yield func(EditionKey, Edition) bool) {
	for k, t := range b.Targets {
		if t == nil {
			continue
		}
		if !yield(k, Edition{Runs: t.Runs, Status: Status(t.Status), Origin: t.Origin, Score: t.Score}) {
			return
		}
	}
}
