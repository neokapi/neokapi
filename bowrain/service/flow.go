package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
)

// ErrCapacityExhausted is returned when a flow cannot be admitted because the
// server-level in-flight-bytes budget is saturated and the acquire wait
// elapsed. Callers map it to a 429/503 (HTTP) or ResourceExhausted (gRPC)
// load-shed response rather than a 500.
var ErrCapacityExhausted = errors.New("server flow capacity exhausted; retry later")

// Environment variables tuning the server-level flow admission budget.
const (
	// flowMaxInflightEnv overrides the total in-flight-bytes budget shared
	// across all concurrent flow executions on this process. Unset/zero/invalid
	// → safeio.DefaultMaxInflightBytes; negative → admission disabled (no cap).
	flowMaxInflightEnv = "BOWRAIN_FLOW_MAX_INFLIGHT_BYTES"

	// flowAdmissionWaitEnv overrides how long a flow queues for budget before it
	// is shed with ErrCapacityExhausted (a Go duration string, e.g. "3s").
	// Unset/invalid → defaultAdmissionWait; "0" disables shedding (queue until
	// budget frees or the caller's context is done).
	flowAdmissionWaitEnv = "BOWRAIN_FLOW_ADMISSION_WAIT"

	// defaultAdmissionWait bounds how long a saturated request waits for budget
	// before shedding. Short enough to shed load promptly, long enough to ride
	// out brief contention without spurious 503s.
	defaultAdmissionWait = 5 * time.Second
)

// FlowService manages flow execution with optional store integration.
type FlowService struct {
	store     store.ContentStore
	formatReg *registry.FormatRegistry
	toolReg   *registry.ToolRegistry

	// admission caps total in-flight bytes across all concurrent flow runs on
	// this process (a server-level, cross-request resource cap). A nil admission
	// means no cap. See safeio.Admission.
	admission *safeio.Admission

	// admissionWait bounds how long AcquireCapacity queues for budget before
	// shedding with ErrCapacityExhausted. <= 0 means block until available.
	admissionWait time.Duration

	// tracker captures flow_run_completed analytics events. Optional; nil
	// disables capture (see Services.SetEventTracker).
	tracker EventTracker
}

// NewFlowService creates a new FlowService with a server-level admission budget
// configured from the environment (see flowMaxInflightEnv / flowAdmissionWaitEnv).
func NewFlowService(s store.ContentStore, formatReg *registry.FormatRegistry, toolReg *registry.ToolRegistry) *FlowService {
	return &FlowService{
		store:         s,
		formatReg:     formatReg,
		toolReg:       toolReg,
		admission:     flowAdmissionFromEnv(),
		admissionWait: flowAdmissionWaitFromEnv(),
	}
}

// flowAdmissionFromEnv builds the in-flight-bytes admission from the
// environment, defaulting to safeio.DefaultMaxInflightBytes.
func flowAdmissionFromEnv() *safeio.Admission {
	v := os.Getenv(flowMaxInflightEnv)
	if v == "" {
		return safeio.NewAdmission(safeio.DefaultMaxInflightBytes)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n == 0 {
		return safeio.NewAdmission(safeio.DefaultMaxInflightBytes)
	}
	return safeio.NewAdmission(n) // negative → nil (no cap)
}

// flowAdmissionWaitFromEnv reads the shed-wait duration from the environment.
func flowAdmissionWaitFromEnv() time.Duration {
	v := os.Getenv(flowAdmissionWaitEnv)
	if v == "" {
		return defaultAdmissionWait
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultAdmissionWait
	}
	return d // may be 0 (block until available)
}

// AcquireCapacity reserves weight bytes of the server-level in-flight budget for
// a unit of document processing, returning a release function that MUST be
// called exactly once when the work completes (calling it more than once is a
// safe no-op).
//
// On a nil admission (cap disabled) it returns a no-op release immediately. When
// the budget is saturated it queues up to admissionWait, then:
//   - if the caller's context is already done, returns that context error;
//   - otherwise returns ErrCapacityExhausted so the caller can shed load.
func (s *FlowService) AcquireCapacity(ctx context.Context, weight int64) (release func(), err error) {
	if s.admission == nil {
		return func() {}, nil
	}
	acqCtx := ctx
	if s.admissionWait > 0 {
		var cancel context.CancelFunc
		acqCtx, cancel = context.WithTimeout(ctx, s.admissionWait)
		defer cancel()
	}
	release, err = s.admission.Acquire(acqCtx, weight)
	if err != nil {
		// Distinguish caller cancellation/deadline from admission saturation: a
		// done parent context is the caller's own cancel/timeout; anything else
		// is the acquire wait elapsing while the budget stayed full.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrCapacityExhausted
	}
	return release, nil
}

// ExecuteFlow runs a flow definition with optional store-backed persistence.
// When projectID is non-empty and a content store is configured, output blocks
// are persisted to the store after successful execution.
//
// Before executing, it reserves the estimated in-flight size of the batch from
// the server-level admission budget so that concurrent flow runs cannot exhaust
// memory; on saturation it returns ErrCapacityExhausted.
func (s *FlowService) ExecuteFlow(ctx context.Context, f *flow.Flow, items []*flow.Item, projectID string) error {
	if f == nil {
		return errors.New("flow definition is required")
	}
	if len(items) == 0 {
		return errors.New("at least one flow item is required")
	}

	release, err := s.AcquireCapacity(ctx, flowItemsWeight(items))
	if err != nil {
		return err
	}
	defer release()

	opts := []flow.ExecutorOption{
		flow.WithFailFast(true),
	}

	start := time.Now()
	executor := flow.NewExecutor(opts...)
	if err := executor.Execute(ctx, f, items); err != nil {
		s.trackFlowRun(f.Name, projectID, len(items), time.Since(start), "failed")
		return fmt.Errorf("execute flow: %w", err)
	}

	// Persist output blocks to the content store if project-scoped.
	if projectID != "" && s.store != nil {
		for _, item := range items {
			if len(item.OutputBlocks) > 0 {
				if err := s.store.StoreBlocks(ctx, projectID, "main", item.OutputBlocks); err != nil {
					s.trackFlowRun(f.Name, projectID, len(items), time.Since(start), "persist_failed")
					return fmt.Errorf("persist flow output blocks: %w", err)
				}
			}
		}
	}

	s.trackFlowRun(f.Name, projectID, len(items), time.Since(start), "completed")
	return nil
}

// trackFlowRun captures a flow_run_completed analytics event after a flow
// execution finishes (fire-and-forget, nil-safe; never carries content).
func (s *FlowService) trackFlowRun(flowName, projectID string, parts int, d time.Duration, outcome string) {
	if s.tracker == nil {
		return
	}
	props := analytics.Props("", projectID)
	props["flow"] = flowName
	props["duration_bucket"] = analytics.DurationBucket(d)
	props["outcome"] = outcome
	props["part_count"] = parts
	track(s.tracker, projectID, analytics.EventFlowRunCompleted, props)
}

// flowItemsWeight estimates the in-flight byte weight of a batch: the sum of
// each item's on-disk input size (safeio.FileWeight, which returns
// UnknownFileWeight for in-memory/stdin inputs). Never returns <= 0 so a batch
// always draws some budget.
func flowItemsWeight(items []*flow.Item) int64 {
	var total int64
	for _, it := range items {
		if it == nil || it.Input == nil {
			continue
		}
		total += safeio.FileWeight(it.Input.URI)
	}
	if total <= 0 {
		total = safeio.UnknownFileWeight
	}
	return total
}
