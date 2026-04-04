// Package flow orchestrates the execution of tool pipelines. A [Flow] is a
// configured sequence of Tools that Parts stream through via buffered channels.
// [Builder] provides a fluent API for constructing flows, and
// [Executor] runs them with concurrent goroutines, error propagation,
// and context cancellation.
package flow
