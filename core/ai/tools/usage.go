package tools

import (
	"sync"

	"github.com/neokapi/neokapi/providers/ai"
)

// UsageReporter is implemented by AI tools that track token usage.
// The platform layer can type-assert tool.Tool to UsageReporter to
// read accumulated usage without importing specific tool types.
type UsageReporter interface {
	TotalUsage() aiprovider.TokenUsage
	ResetUsage()
}

// usageAccumulator is a thread-safe token usage counter embedded in AI tools.
type usageAccumulator struct {
	mu    sync.Mutex
	usage aiprovider.TokenUsage
}

func (a *usageAccumulator) addUsage(u aiprovider.TokenUsage) {
	a.mu.Lock()
	a.usage = a.usage.Add(u)
	a.mu.Unlock()
}

// TotalUsage returns accumulated token usage since last reset.
func (a *usageAccumulator) TotalUsage() aiprovider.TokenUsage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.usage
}

// ResetUsage clears the accumulated usage counter.
func (a *usageAccumulator) ResetUsage() {
	a.mu.Lock()
	a.usage = aiprovider.TokenUsage{}
	a.mu.Unlock()
}
