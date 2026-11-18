// Package termcandidate finds a project's key language and files it for
// review: candidate terms drawn from a sample of every collection, counted
// over the whole project, and stored as proposed concepts in the workspace
// terms store.
//
// A candidate is a concept whose source-language term has status proposed.
// terms.RuleForConcept skips proposed terms, so a candidate governs nothing
// until a person keeps it. Each candidate carries its reach (the blocks that
// use it, per collection) as concept properties, so the context deck can rank
// candidates without recounting, and it is scoped to the coordinates every
// block that uses it shares.
package termcandidate
