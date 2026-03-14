package bridge

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
)

// BridgeRoundTripper performs a complete read → process → write cycle through
// a single Java bridge instance using bidirectional streaming. Multiple
// round-trippers can share the same bridge via concurrent gRPC streams,
// enabling N files to be processed through one JVM.
//
// Usage:
//
//	rt := bridge.NewBridgeRoundTripper(pool, cfg, filterClass)
//	result, err := rt.Execute(ctx, cfg, processFn)
type BridgeRoundTripper struct {
	pool        *BridgePool
	cfg         BridgeConfig
	filterClass string
}

// NewBridgeRoundTripper creates a round-tripper that uses the pool's shared
// access mode to allow concurrent file processing through one JVM.
func NewBridgeRoundTripper(pool *BridgePool, cfg BridgeConfig, filterClass string) *BridgeRoundTripper {
	return &BridgeRoundTripper{
		pool:        pool,
		cfg:         cfg,
		filterClass: filterClass,
	}
}

// RoundTripConfig configures a complete read→process→write cycle.
type RoundTripConfig struct {
	// Input document.
	Content      []byte // Inline content bytes (sent via gRPC)
	InputPath    string // Input file path (Java reads from disk)
	URI          string
	SourceLocale string
	TargetLocale string
	Encoding     string
	MimeType     string
	FilterParams map[string]any

	// Output.
	OutputPath   string
	OutputLocale string
}

// Execute performs the round-trip using the pool's shared access mode. Multiple
// concurrent Execute calls share the same JVM bridge, each getting its own
// gRPC bidirectional stream. The processFn receives parts read from the document
// and should return processed parts for writing.
func (rt *BridgeRoundTripper) Execute(ctx context.Context, cfg RoundTripConfig,
	processFn func(parts <-chan *model.Part) <-chan *model.Part) (*RoundTripResult, error) {

	b, err := rt.pool.AcquireShared(rt.cfg)
	if err != nil {
		return nil, fmt.Errorf("acquiring shared bridge for roundtrip: %w", err)
	}
	defer rt.pool.ReleaseShared(b)

	sourcePath := cfg.InputPath
	if sourcePath != "" && !filepath.IsAbs(sourcePath) {
		if abs, err := filepath.Abs(sourcePath); err == nil {
			sourcePath = abs
		}
	}

	result, err := b.RoundTrip(ctx, RoundTripParams{
		FilterClass:  rt.filterClass,
		URI:          cfg.URI,
		SourceLocale: cfg.SourceLocale,
		TargetLocale: cfg.TargetLocale,
		Encoding:     cfg.Encoding,
		MimeType:     cfg.MimeType,
		FilterParams: cfg.FilterParams,
		Content:      cfg.Content,
		SourcePath:   sourcePath,
		OutputPath:   cfg.OutputPath,
		OutputLocale: cfg.OutputLocale,
	}, processFn)
	if err != nil {
		return nil, fmt.Errorf("roundtrip: %w", err)
	}
	return result, nil
}
